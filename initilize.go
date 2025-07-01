package sqlite

import (
	"os"
	"path/filepath"

	"gopkg.hlmpn.dev/pkg/go-logger"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Init initializes the database
func Init(path string) {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	switch {
	case os.IsExist(err):
		// Directory already exists, no problem
	case err != nil:
		logger.LogErrorf("@db.Init: Error creating database directory: %v", err)
	}

	_, err = os.Stat(path)
	switch {
	case os.IsNotExist(err):
		_, createErr := os.Create(path)
		if createErr != nil {
			logger.LogErrorf("@db.Init: Error creating database: %v", createErr)
		}
	case err != nil:
		logger.LogErrorf("@db.Init: Error opening database: %v", err)
	}

	// Create new DB instance with connection pool
	db = &DB{
		path: path,
		pool: newPool(path),
	}

	// Test initial connection by executing a simple query
	conn := db.Conn()
	if conn == nil {
		logger.LogErrorf("@db.Init: %v", ErrNoConnection)
	}
	defer db.FreeConn(conn)

	err = sqlitex.ExecuteTransient(conn, "SELECT 1", nil)
	if err != nil {
		logger.LogErrorf("@db.Init: %v: %v", ErrQueryFailed, err)
	}
}

func InitWithSchema(tables any, path string) {
	Init(path)
	schema = tables
	InitializeDB()
}
