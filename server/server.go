package server

import (
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"

	"git.tcp.direct/Mirrors/bitcask-mirror"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/finn"
	"github.com/tidwall/redcon"
)

const defaultTCPKeepAlive = time.Minute * 5

var (
	errWrongNumberOfArguments = errors.New("wrong number of arguments")
)

//goland:noinspection GoExportedElementShouldHaveComment
func ListenAndServe(addr, join, dir, logdir string, consistency, durability finn.Level) error {
	opts := finn.Options{
		Backend:     finn.FastLog,
		Consistency: consistency,
		Durability:  durability,
		ConnAccept:  AcceptConnection,
	}
	m, err := NewStateMachine(dir)
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

// StateMachine is the FSM for the Raft consensus.
type StateMachine struct {
	mu     sync.RWMutex
	dir    string
	db     *bitcask.Bitcask
	dbPath string
	// TODO: what was "addr" for?
	//	addr      string
	closed    bool
	cmdMapper map[string]cmdHandler
}

// NewStateMachine constructs a StateMachine type for our finite state machine for Raft consensus that will power our database.
func NewStateMachine(dir string) (*StateMachine, error) {
	kvm := &StateMachine{
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
func (kvm *StateMachine) Close() (err error) {
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	err = kvm.db.Close()
	kvm.closed = true
	return
}
