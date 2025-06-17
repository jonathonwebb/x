package up

import "context"

type Loader interface {
	Load(context.Context) ([]*Migration, error)
}

type FuncLoader struct {
	migrations []*Migration
}

func NewFuncLoader(migrations ...*Migration) *FuncLoader {
	return &FuncLoader{
		migrations: migrations,
	}
}

func (l *FuncLoader) Load(ctx context.Context) ([]*Migration, error) {
	return l.migrations, nil
}
