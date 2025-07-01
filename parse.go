package sqlite

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// timeType is stored as a constant to avoid repeated reflection calls
var timeType = reflect.TypeOf(time.Time{})

// timeFormatSQLite should ideally be a shared constant within the db package
const timeFormatSQLiteParse = "2006-01-02 15:04:05.999"

func RowsToSlice[T any](rows Rows) ([]*T, error) {
	items := make([]*T, 0, len(rows))
	for _, row := range rows {
		item := new(T)
		if err := mapRowToStruct(row, item); err != nil {
			return nil, fmt.Errorf("mapping error: %v", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// mapRowToStruct handles mapping a single database row to a struct
func mapRowToStruct(row Row, dest any) error {
	destValue := reflect.ValueOf(dest).Elem()
	destType := destValue.Type()

	for i := range destType.NumField() {
		field := destType.Field(i)
		fieldValue := destValue.Field(i)
		fieldType := field.Type
		fieldKind := fieldType.Kind()
		dbTag := field.Tag.Get("db")
		dbTypeTag := strings.ToLower(field.Tag.Get("db_type"))
		value, valueExists := row[dbTag]

		// Process all field cases in a single switch statement
		switch {
		// 1. Skip if exclusion, no db tag, or no value
		case field.Tag.Get("db_exclude") != "" || dbTag == "" || !valueExists || value == nil:
			continue

		// 2. JSONB check
		case field.Tag.Get("jsonb") == "true":
			if value == "" {
				continue
			}

			destType := fieldType.Elem()
			jsonDest := reflect.New(destType).Interface()

			// In SQLite, JSONB data should be stored as a string, but we check to be safe
			strValue, ok := value.(string)
			if !ok {
				return fmt.Errorf("expected string for JSONB field %s, got %T", field.Name, value)
			}

			// Log the value to debug

			err := json.Unmarshal([]byte(strValue), jsonDest)
			if err != nil {
				return fmt.Errorf("unmarshal %s error: %v", field.Name, err)
			}

			fieldValue.Set(reflect.ValueOf(jsonDest))
			continue

		// 3. DB types and foreign keys
		case dbTypeTag == "blob":
			uint8Value, isUint8 := value.([]uint8)
			if isUint8 {
				fieldValue.Set(reflect.ValueOf(uint8Value))
				continue
			}

		// 4. Handle time.Time fields
		case fieldType == timeType:
			strValue, isString := value.(string)
			if isString {
				t, err := time.Parse(time.RFC3339, strValue)
				if err != nil {
					return fmt.Errorf("failed to parse time for field %s from value %q: %v", field.Name, strValue, err)
				}
				fieldValue.Set(reflect.ValueOf(t))
				continue
			}

		// 5. Basic types - String
		case fieldKind == reflect.String:
			strValue, isString := value.(string)
			if isString {
				fieldValue.SetString(strValue)
			}

		// 6. Basic types - Integer types
		case fieldKind == reflect.Int:
			intValue, isInt := value.(int)
			if isInt {
				fieldValue.SetInt(int64(intValue))
			}
		case fieldKind == reflect.Int32 && value != nil:
			// Int32 - try int64 value first
			int64Value, isInt64 := value.(int64)
			if isInt64 {
				fieldValue.SetInt(int64Value)
				continue
			}

		case fieldKind == reflect.Int32 && value != nil:
			// Int32 - try int32 value
			int32Value, isInt32 := value.(int32)
			if isInt32 {
				fieldValue.SetInt(int64(int32Value))
				continue
			}

		case fieldKind == reflect.Int32 && value != nil:
			// Int32 - try int value last
			intValue, isInt := value.(int)
			if isInt {
				fieldValue.SetInt(int64(intValue))
				continue
			}

		case fieldKind == reflect.Int64:
			int64Value, isInt64 := value.(int64)
			if isInt64 {
				fieldValue.SetInt(int64Value)
			}

		// 7. Boolean
		case fieldKind == reflect.Bool && value != nil:
			// SQLite stores booleans as integer 0 or 1.
			int64Value, isInt64 := value.(int64)
			if isInt64 {
				fieldValue.SetBool(int64Value != 0)
			}
		}
	}

	return nil
}
