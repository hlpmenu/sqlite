package sqlite

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"gopkg.hlmpn.dev/pkg/go-logger"
)

// Example map of Go→SQLite types
var goToSQLiteTypeMap = map[string]string{
	"string":                 "TEXT",
	"int":                    "INTEGER",
	"int8":                   "INTEGER",
	"int16":                  "INTEGER",
	"int32":                  "INTEGER",
	"int64":                  "INTEGER",
	"uint":                   "INTEGER",
	"uint8":                  "INTEGER",
	"uint16":                 "INTEGER",
	"uint32":                 "INTEGER",
	"uint64":                 "INTEGER",
	"float32":                "REAL",
	"float64":                "REAL",
	"bool":                   "INTEGER",
	"[]byte":                 "BLOB",
	"time.Time":              "DATETIME",
	"timestamp.Timestamp":    "DATETIME",
	"[]string":               "TEXT",
	"[]int":                  "TEXT",
	"[]int64":                "TEXT",
	"[]float64":              "TEXT",
	"map[string]interface{}": "TEXT",
	"interface{}":            "TEXT",
	"any":                    "TEXT",
	"[]interface{}":          "TEXT",
	"[]any":                  "TEXT",
	"json.RawMessage":        "TEXT",
}

var schema any

// processStructFields processes fields from a struct type and adds them to the fields slice.
// It handles embedded structs by recursively processing their fields.
func processStructFields(structType reflect.Type, fields *[]string) {
	for i := range structType.NumField() {
		field := structType.Field(i)

		// Skip if marked as go_only
		if field.Tag.Get("db_exclude") == "go_only" {
			continue
		}

		// Check if this is an embedded struct (anonymous field)
		if field.Anonymous && field.Name != "" && field.Tag.Get("jsonb") != "true" {
			// Recursively process the embedded struct's fields
			embeddedType := field.Type
			processStructFields(embeddedType, fields)
			continue
		}

		columnName := field.Tag.Get("db")
		if columnName == "" {
			columnName = field.Name
		}

		switch columnName {
		case "created_at", "last_modified", "updated_at", "recorded_at":
			def := columnName + " DATETIME DEFAULT (datetime('now'))"
			*fields = append(*fields, def)
			continue
		}

		columnType := field.Tag.Get("db_type")
		jsonb := field.Tag.Get("jsonb")
		foreignKey := field.Tag.Get("foreign_key")
		autoincrement := field.Tag.Get("autoincrement")
		unique := field.Tag.Get("unique")
		notNull := field.Tag.Get("notnull")
		defaultVal := field.Tag.Get("default")

		// Decide base column type
		switch {
		case jsonb == "true":
			columnType = "TEXT"
		case foreignKey != "":
			columnType = "TEXT"
		case columnType != "":
			// if user typed "varchar(255)" or so, treat as TEXT
			up := strings.ToUpper(columnType)
			if strings.HasPrefix(up, "VARCHAR") {
				columnType = "TEXT"
			} else if mapped, ok := goToSQLiteTypeMap[strings.ToLower(columnType)]; ok {
				columnType = mapped
			}
			// otherwise, leave as is (e.g. "BLOB", "TEXT", etc.)
		default:
			// Reflect-based
			goType := field.Type.String()
			if mapped, ok := goToSQLiteTypeMap[goType]; ok {
				columnType = mapped
			} else {
				columnType = "TEXT"
			}
		}

		// Begin definition
		columnDef := fmt.Sprintf("%s %s", columnName, columnType)

		// Additional constraints
		if unique == "true" {
			columnDef += " UNIQUE"
		}
		if notNull == "true" {
			columnDef += " NOT NULL"
		}
		if defaultVal != "" {
			columnDef += " DEFAULT " + defaultVal
		}

		// Check if autoincrement is requested
		if autoincrement == "true" {
			// Must verify the field is an integer type
			// Usually reflect.Kind is the easiest:
			fieldKind := field.Type.Kind()
			if fieldKind != reflect.Int && fieldKind != reflect.Int64 {
				// You can choose to panic or just log/ignore
				log.Fatalf(
					"Field %s in %s is tagged 'autoincrement' but is not an int/int64 (kind: %v).",
					field.Name, "table", fieldKind,
				)
			}
			// Then override the definition
			columnDef = columnName + " INTEGER PRIMARY KEY AUTOINCREMENT"
		}

		*fields = append(*fields, columnDef)
	}
}

// generateCreateTableStatement creates a CREATE TABLE statement for SQLite,
// ensuring we only apply "autoincrement" if the field type is int or int64.
// It now correctly flattens embedded structs.
func generateCreateTableStatement(tableName string, tableStruct any) string {
	t := reflect.TypeOf(tableStruct)

	var fields []string
	processStructFields(t, &fields)

	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  %s
)`, tableName, strings.Join(fields, ",\n  "))

	return stmt
}

// CheckAndAddColumns checks for missing columns and adds them to the table
func CheckAndAddColumns(tableName string, tableStruct any) {
	t := reflect.TypeOf(tableStruct)
	existingCols, err := getExistingColumns(tableName)
	if err != nil {
		log.Printf("checkAndAddColumns: error retrieving existing columns: %v", err)
		return
	}

	// Create a temporary slice to hold all column definitions
	var allColumns []struct {
		Name            string
		Type            string
		IsEmbedded      bool
		IsAutoincrement bool
	}

	// Process all fields, including embedded ones
	var processFields func(structType reflect.Type, path string)
	processFields = func(structType reflect.Type, path string) {
		for i := range structType.NumField() {
			field := structType.Field(i)
			if field.Tag.Get("db_exclude") == "go_only" {
				continue
			}

			// Check if this is an embedded struct (anonymous field)
			if field.Anonymous && field.Name != "" && field.Tag.Get("jsonb") != "true" {
				// Recursively process the embedded struct's fields
				embeddedType := field.Type
				processFields(embeddedType, path)
				continue
			}

			columnName := field.Tag.Get("db")
			if columnName == "" {
				columnName = field.Name
			}

			// Skip if column already exists
			if _, ok := existingCols[columnName]; ok {
				continue
			}

			// build definition
			var colDef string
			isAutoincrement := false

			switch columnName {
			case "created_at", "last_modified", "updated_at":
				colDef = "DATETIME DEFAULT (datetime('now'))"
			default:
				goType := field.Type.String()
				dbType := field.Tag.Get("db_type")
				jsonb := field.Tag.Get("jsonb")
				foreignKey := field.Tag.Get("foreign_key")
				autoincrement := field.Tag.Get("autoincrement")

				switch {
				case autoincrement == "true":
					// ensure it's actually int or int64
					fieldKind := field.Type.Kind()
					if fieldKind != reflect.Int && fieldKind != reflect.Int64 {
						log.Fatalf(
							"Field %s in %s is tagged 'autoincrement' but is not an int/int64 (kind: %v).",
							field.Name, tableName, fieldKind,
						)
					}
					isAutoincrement = true
					colDef = "INTEGER"
				case jsonb == "true":
					colDef = "TEXT"
				case foreignKey != "":
					colDef = "TEXT"
				case dbType != "":
					up := strings.ToUpper(dbType)
					if strings.HasPrefix(up, "VARCHAR") {
						colDef = "TEXT"
					} else if mapped, ok := goToSQLiteTypeMap[strings.ToLower(dbType)]; ok {
						colDef = mapped
					} else {
						colDef = dbType
					}
				default:
					if mapped, ok := goToSQLiteTypeMap[goType]; ok {
						colDef = mapped
					} else {
						colDef = "TEXT"
					}
				}
			}

			allColumns = append(allColumns, struct {
				Name            string
				Type            string
				IsEmbedded      bool
				IsAutoincrement bool
			}{
				Name:            columnName,
				Type:            colDef,
				IsEmbedded:      field.Anonymous,
				IsAutoincrement: isAutoincrement,
			})
		}
	}

	// Start the processing
	processFields(t, "")

	// Now add all the missing columns
	for _, col := range allColumns {
		alterSQL := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, col.Name, col.Type)
		if err := db.Exec(alterSQL); err != nil {
			log.Printf("CheckAndAddColumns: cannot add column %s to table %s: %v",
				col.Name, tableName, err)
		}
	}
}

// getExistingColumns checks the SQLite schema for a table's columns via PRAGMA table_info.
func getExistingColumns(tableName string) (map[string]bool, error) {
	results := make(map[string]bool)

	query := fmt.Sprintf(`PRAGMA table_info(%s)`, tableName)
	rows, err := db.Query(query)
	if err != nil {
		return results, err
	}
	if rows == nil {
		return results, nil
	}
	defer func() {
		// if there's anything to finalize, do it here
	}()

	// Each row in PRAGMA table_info is: cid, name, type, notnull, dflt_value, pk
	for _, row := range *rows {
		nameVal, ok := row["name"]
		if !ok {
			continue
		}
		colName, _ := nameVal.(string)
		results[colName] = true
	}

	return results, nil
}

// InitializeDB iterates over your DBTABLES fields, creates each table, and (optionally) sets up triggers.
func InitializeDB() {
	if schema == nil {
		logger.LogErrorf("@db.InitializeDB: Missing tables")
	}

	// reflect on dbTables
	tables := reflect.TypeOf(schema).Elem()

	createStatements := make([]string, 0, tables.NumField())
	var triggerStatements []string // If you want to do updated_at triggers, etc.

	for i := range tables.NumField() {
		table := tables.Field(i)
		tableName := table.Tag.Get("db")
		if tableName == "" {
			// skip if no `db:"..."` tag
			continue
		}

		// get an instance of the struct
		tableStruct := reflect.New(table.Type.Elem()).Elem().Interface()

		// build create table statement
		createSQL := generateCreateTableStatement(tableName, tableStruct)
		createStatements = append(createStatements, createSQL) //nolint:gocritic // appendAssign: intentional

		// If you want triggers for updated_at:
		// e.g. BEFORE UPDATE triggers that set updated_at = datetime('now')
		/*
			tStruct := reflect.TypeOf(tableStruct)
			for j := 0; j < tStruct.NumField(); j++ {
				field := tStruct.Field(j)
				colName := field.Tag.Get("db")
				if colName == "updated_at" || colName == "last_modified" {
					triggerName := fmt.Sprintf("trg_%s_update_timestamp", tableName)
					trgSQL := fmt.Sprintf(`
						DROP TRIGGER IF EXISTS %s;
						CREATE TRIGGER %s
						BEFORE UPDATE ON %s
						FOR EACH ROW
						BEGIN
							SET NEW.%s = datetime('now');
						END;
					`, triggerName, triggerName, tableName, colName)
					triggerStatements = append(triggerStatements, trgSQL)
					break // only need one trigger per table
				}
			}
		*/
	}

	// Combine CREATE TABLE statements and trigger statements
	allStatements := append(createStatements, triggerStatements...) //nolint:gocritic // appendAssign: intentional
	if len(allStatements) == 0 {
		return
	}

	// In SQLite, you can do them individually or within a transaction. We can do individually here:
	for _, stmt := range allStatements {
		if err := db.Exec(stmt); err != nil {
			log.Fatalf("InitializeDB: statement failed:\n%s\nError: %v\n", stmt, err)
		}
	}
	log.Printf("InitializeDB: tables created or verified.")

	// Then optionally, check & add columns for each table:
	for i := range tables.NumField() {
		table := tables.Field(i)
		tableName := table.Tag.Get("db")
		if tableName == "" {
			continue
		}
		tableStruct := reflect.New(table.Type.Elem()).Elem().Interface()
		CheckAndAddColumns(tableName, tableStruct)
	}
}
