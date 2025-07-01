package sqlite_test

import (
	"os"
	"testing"

	"gopkg.hlmpn.dev/pkg/sqlite"
)

func TestMain(t *testing.T) {
	deleteTestDB()
	type MyTable struct {
		MyCol1  string `db:"my_col1"`
		IntCol  int    `db:"int_col"`
		BoolCol bool   `db:"bool_col"`
	}
	type Schema struct {
		MyTable *MyTable `db:"my_table"`
	}
	sqlite.InitWithSchema(&Schema{}, "test.db")
	t.Log("Database initialized")

	testquery := `INSERT INTO my_table (my_col1, int_col, bool_col) VALUES (?, ?, ?)`
	err := sqlite.Exec(testquery, "test", 123, true)
	if err != nil {
		t.Errorf("Error executing query: %v", err)
	}
	t.Log("Query executed")
	rows, err := sqlite.Query("SELECT * FROM my_table")
	if err != nil {
		t.Errorf("Error querying table: %v", err)
	}
	if rows == nil || rows.Len() == 0 {
		t.Errorf("No rows returned")
	}
	sc, err := sqlite.RowsToSlice[MyTable](*rows)
	if err != nil {
		t.Errorf("Error converting rows to slice: %v", err)
	}
	if len(sc) != 1 {
		t.Errorf("Expected 1 row, got %d", len(sc))
	}
	t.Logf("Rows: %v", sc[0])
}

func deleteTestDB() {
	os.RemoveAll("test.db")
}
