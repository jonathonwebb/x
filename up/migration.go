package up

import (
	"context"
	"database/sql"
	"fmt"
)

type Migration struct {
	Version    int64
	Name       string
	RunFunc    func(context.Context, *sql.DB) error
	RevertFunc func(context.Context, *sql.DB) error
}

func (m *Migration) Run(ctx context.Context, db *sql.DB) error {
	if m.RunFunc == nil {
		return fmt.Errorf("migration %q (%d) has no run function", m.Name, m.Version)
	}
	return m.RunFunc(ctx, db)
}

func (m *Migration) Revert(ctx context.Context, db *sql.DB) error {
	if m.RevertFunc == nil {
		return fmt.Errorf("migration %q (%d) has no revert function", m.Name, m.Version)
	}
	return m.RevertFunc(ctx, db)
}
