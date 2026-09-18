//go:build cgo

package db

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
#include <stdlib.h>

static sqlite3* ctx_open(const char* path) {
    sqlite3 *db = NULL;
    if (sqlite3_open(path, &db) != SQLITE_OK) {
        if (db) sqlite3_close(db);
        return NULL;
    }
    return db;
}
static void ctx_close(sqlite3* db) { if (db) sqlite3_close(db); }
static const char* ctx_errmsg(sqlite3* db) { return sqlite3_errmsg(db); }
static int ctx_exec(sqlite3* db, const char* sql, char** err_out) {
    return sqlite3_exec(db, sql, NULL, NULL, err_out);
}
static int ctx_bind_text(sqlite3_stmt* stmt, int idx, const char* s) {
    if (s == NULL) return sqlite3_bind_null(stmt, idx);
    return sqlite3_bind_text(stmt, idx, s, -1, SQLITE_TRANSIENT);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

type DB struct {
	sync.Mutex
	ptr *C.sqlite3
}

func Open(path string) (*DB, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	p := C.ctx_open(cpath)
	if p == nil {
		return nil, fmt.Errorf("sqlite open failed: %s", path)
	}
	db := &DB{ptr: p}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) Close() {
	if d == nil {
		return
	}
	d.Lock()
	defer d.Unlock()
	if d.ptr != nil {
		C.ctx_close(d.ptr)
		d.ptr = nil
	}
}

func (d *DB) Exec(sql string, args ...any) (int64, error) {
	d.Lock()
	defer d.Unlock()
	stmtSQL := C.CString(sql)
	defer C.free(unsafe.Pointer(stmtSQL))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(d.ptr, stmtSQL, -1, &stmt, nil); rc != C.SQLITE_OK {
		return 0, fmt.Errorf("prepare: %s", C.GoString(C.ctx_errmsg(d.ptr)))
	}
	defer C.sqlite3_finalize(stmt)
	for i, a := range args {
		if a == nil {
			C.ctx_bind_text(stmt, C.int(i+1), nil)
			continue
		}
		ca := C.CString(fmt.Sprint(a))
		C.ctx_bind_text(stmt, C.int(i+1), ca)
		C.free(unsafe.Pointer(ca))
	}
	rc := C.sqlite3_step(stmt)
	if rc != C.SQLITE_DONE && rc != C.SQLITE_ROW {
		return 0, fmt.Errorf("exec: %s", C.GoString(C.ctx_errmsg(d.ptr)))
	}
	return int64(C.sqlite3_changes(d.ptr)), nil
}

func (d *DB) ExecScript(sql string) error {
	d.Lock()
	defer d.Unlock()
	csql := C.CString(sql)
	defer C.free(unsafe.Pointer(csql))
	var cerr *C.char
	rc := C.ctx_exec(d.ptr, csql, &cerr)
	if rc != C.SQLITE_OK {
		msg := C.GoString(cerr)
		if cerr != nil {
			C.sqlite3_free(unsafe.Pointer(cerr))
		}
		return fmt.Errorf("exec script: %s", msg)
	}
	return nil
}

type Row []string

func (d *DB) Query(sql string, args ...any) ([]Row, error) {
	d.Lock()
	defer d.Unlock()
	stmtSQL := C.CString(sql)
	defer C.free(unsafe.Pointer(stmtSQL))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(d.ptr, stmtSQL, -1, &stmt, nil); rc != C.SQLITE_OK {
		return nil, fmt.Errorf("prepare: %s", C.GoString(C.ctx_errmsg(d.ptr)))
	}
	defer C.sqlite3_finalize(stmt)
	for i, a := range args {
		if a == nil {
			C.ctx_bind_text(stmt, C.int(i+1), nil)
			continue
		}
		ca := C.CString(fmt.Sprint(a))
		C.ctx_bind_text(stmt, C.int(i+1), ca)
		C.free(unsafe.Pointer(ca))
	}
	ncol := int(C.sqlite3_column_count(stmt))
	out := []Row{}
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			break
		}
		if rc != C.SQLITE_ROW {
			return nil, fmt.Errorf("query: %s", C.GoString(C.ctx_errmsg(d.ptr)))
		}
		r := make(Row, ncol)
		for i := 0; i < ncol; i++ {
			p := C.sqlite3_column_text(stmt, C.int(i))
			if p != nil {
				r[i] = C.GoString((*C.char)(unsafe.Pointer(p)))
			}
		}
		out = append(out, r)
	}
	return out, nil
}
