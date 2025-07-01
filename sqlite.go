package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"gopkg.hlmpn.dev/pkg/go-logger"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// DB represents a SQLite database
type DB struct {
	path string
	pool *pool
	mu   sync.Mutex
}

// db is the unexported global instance
var db *DB

func IsInitialized() bool {
	if db == nil || db.pool == nil {
		return false
	}
	return true
}

// Lock locks the database for write operations
func (d *DB) Lock() {
	d.mu.Lock()
}

// Unlock unlocks the database after write operations
func (d *DB) Unlock() {
	d.mu.Unlock()
}

// Conn returns a connection from the pool
func (d *DB) Conn() *sqlite.Conn {
	conn := d.pool.Get()
	if conn == nil {
		return d.pool.ManualConnect(d.path)
	}
	return conn
}

// FreeConn returns a connection to the pool
func (d *DB) FreeConn(conn *sqlite.Conn) {
	d.pool.Put(conn)
}

type Rows []Row

func (r *Rows) Len() int {
	return len(*r)
}

func (r *Rows) Add(row Row) {
	*r = append(*r, row)
}

type Row map[string]any

func (r Row) Set(key string, value any) {
	r[key] = value
}

// Query executes a read operation
func (d *DB) Query(query string, args ...any) (*Rows, error) {
	rows := make(Rows, 0)

	queryResultFunc := func(stmt *sqlite.Stmt) error {
		row := make(Row, 0)

		if stmt == nil {
			logger.LogRedf("@db.Query: Error executing query: %v", ErrQueryFailed)
			return ErrQueryFailed
		}

		for i := range stmt.ColumnCount() {
			if stmt.ColumnIsNull(i) {
				continue
			}
			colName := stmt.ColumnName(i)
			// Use a switch to assign the native value.
			switch stmt.ColumnType(i) {
			case sqlite.TypeInteger:
				row.Set(colName, stmt.ColumnInt64(i))
			case sqlite.TypeFloat:
				row.Set(colName, stmt.ColumnFloat(i))
			case sqlite.TypeBlob:
				b := make([]byte, stmt.ColumnLen(i))
				stmt.ColumnBytes(i, b)
				row.Set(colName, b)
			case sqlite.TypeText:
				row.Set(colName, stmt.ColumnText(i))
			case sqlite.TypeNull:
				return fmt.Errorf("NULL value encountered for column %s", colName)
			default:
				return fmt.Errorf("unexpected column type for column %s", colName)
			}

		}

		rows.Add(row)

		return nil
	}
	query = trimQuery(query)

	conn := d.Conn()
	if conn == nil {
		return nil, ErrNoConnection
	}
	defer d.FreeConn(conn)

	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		ResultFunc: queryResultFunc,
		Args:       args,
	})
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	return &rows, nil
}

// Exec executes a write operation
func (d *DB) Exec(query string, args ...any) error {
	conn := d.Conn()
	if conn == nil {
		return ErrNoConnection
	}
	defer d.FreeConn(conn)
	query = trimQuery(query)

	// Process complex args like JSONB fields
	processedArgs, err := preprocessArgs(args...)
	if err != nil {
		return fmt.Errorf("error preprocessing args: %w", err)
	}

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: processedArgs,
	})
	if err != nil {
		return errors.New(ErrExecFailed.Error() + "error: " + err.Error())
	}
	return nil
}

// preprocessArgs marshals arguments that are structs, slices, or pointers to them, into JSON strings.
// This is used for storing complex Go types in JSON/JSONB database columns.
// Other argument types are passed through unchanged.
func preprocessArgs(args ...any) ([]any, error) {
	processedArgs := make([]any, len(args))

	for i, arg := range args {
		// Default: pass through unchanged. This covers basic types, and also untyped nil.
		processedArgs[i] = arg

		if arg == nil { // Handles untyped nil or interface containing nil.
			// An untyped nil or an interface containing nil is passed as is.
			// If it's intended for a JSON column and should be JSON "null",
			// json.Marshal(nil) would produce "null".
			// However, this function primarily targets converting Go structs/slices (and pointers to them)
			// into JSON strings. For a raw `nil` interface value, we let it pass,
			// assuming the database driver or SQL handles it (e.g., as SQL NULL).
			// The reported issue was specific to typed nil pointers becoming "<nil>",
			// which the logic below now addresses.
			continue
		}

		valType := reflect.TypeOf(arg)
		kind := valType.Kind()

		shouldMarshalToJSON := false
		switch kind { //nolint:exhaustive // Only handle specific types, others pass through
		case reflect.Struct, reflect.Slice:
			shouldMarshalToJSON = true
		case reflect.Ptr:
			// Check if it's a pointer to a struct or slice
			elemKind := valType.Elem().Kind()
			if elemKind == reflect.Struct || elemKind == reflect.Slice {
				shouldMarshalToJSON = true
			}
			// Pointers to other types (e.g., *int, *string) are passed through as is.
			// The SQLite driver is expected to handle these for nullable basic type columns.
		}

		if shouldMarshalToJSON {
			jsonBytes, err := json.Marshal(arg) // Marshals arg. For nil pointers to structs/slices, this produces "null".
			if err != nil {
				errMsg := fmt.Errorf("failed to marshal type %T to JSON: %w", arg, err)
				logger.Warnf("@db.preprocessArgs: [Marshalling argument to JSON failed] Error: %s", errMsg.Error())
				return nil, errMsg
			}
			processedArgs[i] = string(jsonBytes) // Store the JSON string (e.g., "{}", "[]", or "null")
		}
		// If not shouldMarshalToJSON, processedArgs[i] remains the original 'arg' (e.g. int, string, *int).
	}

	return processedArgs, nil
}
