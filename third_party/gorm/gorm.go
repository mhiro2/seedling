package gorm

import "context"

type DB struct {
	Error error
}

func (db *DB) WithContext(context.Context) *DB {
	return db
}

func (db *DB) Create(v interface{}) *DB {
	return db
}

func (db *DB) Delete(v interface{}) *DB {
	return db
}
