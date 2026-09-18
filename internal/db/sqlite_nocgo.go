//go:build !cgo

package db

import "fmt"

type DB struct{}

type Row []string

func Open(path string) (*DB, error) {
	return nil, fmt.Errorf("sqlite requires cgo; use pure-Go file store instead (CONTEXTOS_STORAGE=file)")
}

func (d *DB) Close() {}

func (d *DB) Exec(sql string, args ...any) (int64, error) {
	return 0, fmt.Errorf("sqlite requires cgo; use pure-Go file store instead")
}

func (d *DB) ExecScript(sql string) error {
	return fmt.Errorf("sqlite requires cgo; use pure-Go file store instead")
}

func (d *DB) Query(sql string, args ...any) ([]Row, error) {
	return nil, fmt.Errorf("sqlite requires cgo; use pure-Go file store instead")
}
