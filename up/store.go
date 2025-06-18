package up

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrLocked          = errors.New("version store is locked for writing")
	ErrInitialVersion  = errors.New("initial version is current")
	ErrVersionNotFound = errors.New("version not found")
)

type Store interface {
	DB() *sql.DB
	Init(context.Context) error
	Lock(context.Context) error
	Release(context.Context) error
	Version(context.Context) (int64, error)
	Insert(context.Context, int64) error
	Remove(context.Context, int64) error
}
