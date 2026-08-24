package gorm

import (
	"context"
	"reflect"
)

type DB struct {
	Error   error
	Created []any
	Deleted []any
	nextID  uint64
}

func (db *DB) WithContext(context.Context) *DB {
	return db
}

func (db *DB) Create(v interface{}) *DB {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.Elem().Kind() == reflect.Struct {
		id := rv.Elem().FieldByName("ID")
		if id.IsValid() && id.CanSet() && id.IsZero() {
			db.nextID++
			switch id.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				id.SetInt(int64(db.nextID))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				id.SetUint(db.nextID)
			}
		}
		db.Created = append(db.Created, rv.Elem().Interface())
	}
	return db
}

func (db *DB) Delete(v interface{}) *DB {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.Elem().Kind() == reflect.Struct {
		db.Deleted = append(db.Deleted, rv.Elem().Interface())
	}
	return db
}
