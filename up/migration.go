package up

import (
	"context"
	"database/sql"
	"fmt"
)

// A Migration represents a schema change operation. Version indicates the
// migration's order in the change sequence. The Run and Revert functions
// are used to apply and revert the migration, respectively.
type Migration struct {
	Version    int64
	Name       string
	RunFunc    func(context.Context, *sql.DB) error
	RevertFunc func(context.Context, *sql.DB) error
}

// Run applies the migration to the database.
func (m *Migration) Run(ctx context.Context, db *sql.DB) error {
	if m.RunFunc == nil {
		return fmt.Errorf("migration %q (%d) has no run function", m.Name, m.Version)
	}
	return m.RunFunc(ctx, db)
}

// Revert reverses the database migration.
func (m *Migration) Revert(ctx context.Context, db *sql.DB) error {
	if m.RevertFunc == nil {
		return fmt.Errorf("migration %q (%d) has no revert function", m.Name, m.Version)
	}
	return m.RevertFunc(ctx, db)
}
