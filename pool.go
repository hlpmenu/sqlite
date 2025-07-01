package sqlite

import (
	"sync"

	"gopkg.hlmpn.dev/pkg/go-logger"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// pool manages SQLite connections
type pool struct {
	pool sync.Pool
	path string
	mu   sync.Mutex
}

// newPool creates a new connection pool
func newPool(path string) *pool {
	p := &pool{
		path: path,
	}

	p.pool.New = func() any {
		p.mu.Lock()
		defer p.mu.Unlock()

		conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite)
		if err != nil {
			logger.LogRedf("@db.pool.New: Error opening database connection: %v", err)
			return nil
		}

		// Enable foreign keys
		if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
			conn.Close()
			return nil
		}

		return conn
	}

	return p
}

// Get returns a connection from the pool
func (p *pool) Get() *sqlite.Conn {
	conn := p.pool.Get().(*sqlite.Conn)
	if conn == nil {
		return nil
	}
	return conn
}

// Put returns a connection to the pool
func (p *pool) Put(conn *sqlite.Conn) {
	if conn != nil {
		p.pool.Put(conn)
	}
}

// Close closes all connections in the pool
func (p *pool) Close() {
	// Note: sync.Pool doesn't provide a way to close all items
	// Connections will be garbage collected when the pool is no longer referenced
}

func (p *pool) ManualConnect(path string) *sqlite.Conn {
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite)
	if err != nil {
		return nil
	}

	// Enable foreign keys
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		conn.Close()
		return nil
	}
	return conn
}
