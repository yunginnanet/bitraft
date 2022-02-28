package server

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.tcp.direct/Mirrors/bitcask-mirror"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/finn"
	"github.com/tidwall/redcon"
)

const defaultTCPKeepAlive = time.Minute * 5

var (
	errWrongNumberOfArguments = errors.New("wrong number of arguments")
)

func trimPattern(glob string) string {
	escaped := false
	pattern := ""
parse:
	for _, char := range glob {
		switch char {
		case '\\':
			escaped = !escaped
			if !escaped {
				pattern = pattern + string(char)
			}
		case '*':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		case '?':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		case '[':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		default:
			pattern = pattern + string(char)
		}
	}
	return pattern
}

func ListenAndServe(addr, join, dir, logdir string, consistency, durability finn.Level) error {
	opts := finn.Options{
		Backend:     finn.FastLog,
		Consistency: consistency,
		Durability:  durability,
		ConnAccept:  AcceptConnection,
	}
	m, err := NewMachine(dir)
	if err != nil {
		return err
	}
	n, err := finn.Open(logdir, addr, join, m, &opts)
	if err != nil {
		return err
	}
	defer n.Close()

	select {
	// blocking, there's no way out
	}
}

// AcceptConnection handles an incoming TCP connection.
func AcceptConnection(conn redcon.Conn) bool {
	if tcp, ok := conn.NetConn().(*net.TCPConn); ok {
		if err := tcp.SetKeepAlive(true); err != nil {
			log.Warn().Err(err).Caller().Str("caller", tcp.RemoteAddr().String()).
				Msg("could not set keepalive")
		} else {
			err := tcp.SetKeepAlivePeriod(defaultTCPKeepAlive)
			if err != nil {
				log.Warn().Err(err).Caller().Str("caller", tcp.RemoteAddr().String()).
					Msg("could not set keepalive period")
			}
		}
	}
	return true
}

type cmdHandler func(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error)

// Machine is the FSM for the Raft consensus.
type Machine struct {
	mu     sync.RWMutex
	dir    string
	db     *bitcask.Bitcask
	dbPath string
	// TODO: what was "addr" for?
	//	addr      string
	closed    bool
	cmdMapper map[string]cmdHandler
}

// NewMachine constructs a Machine type for our finite state machine for Raft consensus that will power our database.
func NewMachine(dir string) (*Machine, error) {
	kvm := &Machine{
		dir: dir,
		// addr: addr,
	}
	kvm.cmdMapper = map[string]cmdHandler{
		"echo": kvm.cmdEcho, "set": kvm.cmdSet,
		"get": kvm.cmdGet, "del": kvm.cmdDel,
		"keys": kvm.cmdKeys, "flushdb": kvm.cmdFlushdb,
	}
	var err error
	kvm.dbPath = filepath.Join(dir, "node.db")
	kvm.db, err = bitcask.Open(kvm.dir)
	if err != nil {
		return nil, err
	}
	return kvm, nil
}

// Close shuts down our finite state machine and presumably, our database.
func (kvm *Machine) Close() (err error) {
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	err = kvm.db.Close()
	kvm.closed = true
	return
}

func newSubLog(conn redcon.Conn, cmd redcon.Command) *zerolog.Logger {
	slog := log.With().Logger()
	if conn != nil {
		slog = slog.With().Str("caller", conn.NetConn().RemoteAddr().String()).Logger()
	}
	if len(cmd.Raw) > 1 {
		slog = slog.With().Str("cmd", string(cmd.Raw)).Logger()
	}
	return &slog
}

// Command is a callback for incoming Redis commands.
func (kvm *Machine) Command(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	slog := newSubLog(conn, cmd)
	slog.Trace().Msg(string(cmd.Raw))

	strCmd := strings.ToLower(string(cmd.Args[0]))
	handler, ok := kvm.cmdMapper[strCmd]
	switch {
	case ok:
		return handler(m, conn, cmd)
	case strCmd == "shutdown":
		slog.Warn().Msg("shutting down")
		conn.WriteString("OK")
		err := conn.Close()
		if err != nil {
			slog.Debug().Err(err).Caller().Msg("failed to close connection")
		}
		os.Exit(0)
		return nil, nil
	default:
		// TODO: do we need to log here if we are returning the error type?
		slog.Warn().Msg("unknown command")
		return nil, finn.ErrUnknownCommand
	}
}

// Restore attempts to restore a database from rd, which implements an io.Reader.
// This is meant to restore data exported by the Snapshot function.
func (kvm *Machine) Restore(rd io.Reader) error {
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	var err error
	if err := kvm.db.Close(); err != nil {
		return err
	}
	if err := os.RemoveAll(kvm.dbPath); err != nil {
		return err
	}
	kvm.db = nil
	kvm.db, err = bitcask.Open(kvm.dir)
	if err != nil {
		return err
	}
	num := make([]byte, 8)
	gzr, err := gzip.NewReader(rd)
	if err != nil {
		return err
	}
	r := bufio.NewReader(gzr)
	for {
		if _, err := io.ReadFull(r, num); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		key := make([]byte, int(binary.LittleEndian.Uint64(num)))
		if _, err := io.ReadFull(r, key); err != nil {
			return err
		}
		if _, err := io.ReadFull(r, num); err != nil {
			return err
		}
		value := make([]byte, int(binary.LittleEndian.Uint64(num)))
		if _, err := io.ReadFull(r, value); err != nil {
			return err
		}
		if err := kvm.db.Put(key, value); err != nil {
			return err
		}
	}
	return gzr.Close()
}

// WriteRedisCommandsFromSnapshot will read a snapshot and write all the
// Redis SET commands needed to rebuild the entire database.
// The commands are written to wr.
func WriteRedisCommandsFromSnapshot(wr io.Writer, snapshotPath string) error {
	f, err := os.Open(snapshotPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var cmd []byte
	num := make([]byte, 8)
	var gzclosed bool
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		if !gzclosed {
			gzr.Close()
		}
	}()
	r := bufio.NewReader(gzr)
	for {
		if _, err := io.ReadFull(r, num); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		key := make([]byte, int(binary.LittleEndian.Uint64(num)))
		if _, err := io.ReadFull(r, key); err != nil {
			return err
		}
		if _, err := io.ReadFull(r, num); err != nil {
			return err
		}
		value := make([]byte, int(binary.LittleEndian.Uint64(num)))
		if _, err := io.ReadFull(r, value); err != nil {
			return err
		}
		if len(key) == 0 || key[0] != 'k' {
			// do not accept keys that do not start with 'k'
			continue
		}
		key = key[1:]
		cmd = cmd[:0]
		cmd = append(cmd, "*3\r\n$3\r\nSET\r\n$"...)
		cmd = strconv.AppendInt(cmd, int64(len(key)), 10)
		cmd = append(cmd, '\r', '\n')
		cmd = append(cmd, key...)
		cmd = append(cmd, '\r', '\n', '$')
		cmd = strconv.AppendInt(cmd, int64(len(value)), 10)
		cmd = append(cmd, '\r', '\n')
		cmd = append(cmd, value...)
		cmd = append(cmd, '\r', '\n')
		if _, err := wr.Write(cmd); err != nil {
			return err
		}
	}
	err = gzr.Close()
	gzclosed = true
	return err
}

// Snapshot writes a snapshot of the database to wr, which implements io.Writer.
func (kvm *Machine) Snapshot(wr io.Writer) error {
	kvm.mu.RLock()
	defer kvm.mu.RUnlock()
	gzw := gzip.NewWriter(wr)

	err := kvm.db.Fold(func(key []byte) error {
		var buf []byte
		value, err := kvm.db.Get(key)
		if err != nil {
			return err
		}

		num := make([]byte, 8)
		binary.LittleEndian.PutUint64(num, uint64(len(key)))
		buf = append(buf, num...)
		buf = append(buf, key...)
		binary.LittleEndian.PutUint64(num, uint64(len(value)))
		buf = append(buf, num...)
		buf = append(buf, value...)
		if _, err := gzw.Write(buf); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return gzw.Close()
}
