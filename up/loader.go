package up

import "context"

// A Loader provides a way to load migrations from a source.
type Loader interface {
	Load(context.Context) ([]*Migration, error)
}

// FuncLoader is a Loader that loads a slice of Migrations.
type FuncLoader struct {
	migrations []*Migration
}

// NewFuncLoader creates a new FuncLoader with the given migrations.
func NewFuncLoader(migrations ...*Migration) *FuncLoader {
	return &FuncLoader{
		migrations: migrations,
	}
}

// Load loads the migrations from the FuncLoader.
func (l *FuncLoader) Load(ctx context.Context) ([]*Migration, error) {
	return l.migrations, nil
}
