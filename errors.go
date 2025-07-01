package sqlite

import (
	"errors"
	"strings"
)

func NotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "no rows in result set") ||
		strings.Contains(errStr, "no rows returned") ||
		strings.Contains(errStr, "no data found") ||
		strings.Contains(errStr, "null") ||
		strings.Contains(errStr, "not found")
}

var ErrNotFound = errors.New("db: not found")

var (
	ErrNoConnection = errors.New("failed to get database connection")
	ErrQueryFailed  = errors.New("query execution failed")
	ErrExecFailed   = errors.New("execution failed")
)
