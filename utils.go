package sqlite

import (
	"strings"
)

func trimQuery(query string) string {
	return strings.TrimSpace(query)
}
