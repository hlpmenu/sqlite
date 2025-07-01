package sqlite

func Exec(query string, args ...any) error {
	return db.Exec(query, args...)
}

func Query(query string, args ...any) (*Rows, error) {
	return db.Query(query, args...)
}
