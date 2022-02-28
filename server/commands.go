package server

import (
	"path/filepath"

	"git.tcp.direct/Mirrors/bitcask-mirror"
	"github.com/tidwall/finn"
	"github.com/tidwall/redcon"
)

func (kvm *Machine) cmdSet(
	m finn.Applier, conn redcon.Conn, cmd redcon.Command,
) (interface{}, error) {
	if len(cmd.Args) != 3 {
		return nil, finn.ErrWrongNumberOfArguments
	}
	return m.Apply(conn, cmd,
		func() (interface{}, error) {
			kvm.mu.Lock()
			defer kvm.mu.Unlock()
			return nil, kvm.db.Put(cmd.Args[1], cmd.Args[2])
		},
		func(v interface{}) (interface{}, error) {
			conn.WriteString("OK")
			return nil, nil
		},
	)
}

func (kvm *Machine) cmdEcho(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	if len(cmd.Args) != 2 {
		return nil, finn.ErrWrongNumberOfArguments
	}
	conn.WriteBulk(cmd.Args[1])
	return nil, nil
}

func (kvm *Machine) cmdGet(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	if len(cmd.Args) != 2 {
		return nil, finn.ErrWrongNumberOfArguments
	}
	key := cmd.Args[1]
	return m.Apply(conn, cmd, nil,
		func(interface{}) (interface{}, error) {
			kvm.mu.RLock()
			defer kvm.mu.RUnlock()
			value, err := kvm.db.Get(key)
			if err != nil {
				if err == bitcask.ErrKeyNotFound {
					conn.WriteNull()
					return nil, nil
				}
				return nil, err
			}
			conn.WriteBulk(value)
			return nil, nil
		},
	)
}

func (kvm *Machine) cmdDel(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	var startIdx = 1
	return m.Apply(conn, cmd,
		func() (interface{}, error) {
			kvm.mu.Lock()
			defer kvm.mu.Unlock()
			var n int
			for i := startIdx; i < len(cmd.Args); i++ {
				key := cmd.Args[i]
				err := kvm.db.Delete(key)
				if err != nil {
					return 0, err
				}
				n++
			}
			return n, nil
		},
		func(v interface{}) (interface{}, error) {
			n := v.(int)
			conn.WriteInt(n)
			return nil, nil
		},
	)
}

func (kvm *Machine) cmdKeys(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	if len(cmd.Args) != 2 {
		return nil, errWrongNumberOfArguments
	}
	pattern := string(cmd.Args[1])
	scanPattern := trimPattern(pattern)
	return m.Apply(conn, cmd, nil,
		func(interface{}) (interface{}, error) {
			kvm.mu.RLock()
			defer kvm.mu.RUnlock()
			var keys [][]byte
			err := kvm.db.Scan([]byte(scanPattern), func(key []byte) error {
				if ok, _ := filepath.Match(pattern, string(key)); ok {
					keys = append(keys, []byte(key))
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			conn.WriteArray(len(keys))
			for i := 0; i < len(keys); i++ {
				conn.WriteBulk(keys[i])
			}
			return nil, nil
		},
	)
}

func (kvm *Machine) cmdFlushdb(m finn.Applier, conn redcon.Conn, cmd redcon.Command) (interface{}, error) {
	if len(cmd.Args) != 1 {
		return nil, finn.ErrWrongNumberOfArguments
	}
	return m.Apply(conn, cmd,
		func() (interface{}, error) {
			kvm.mu.Lock()
			defer kvm.mu.Unlock()
			if err := kvm.db.Sync(); err != nil {
				panic(err.Error())
			}
			return nil, nil
		},
		func(v interface{}) (interface{}, error) {
			conn.WriteString("OK")
			return nil, nil
		},
	)
}
