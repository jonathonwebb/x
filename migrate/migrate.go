package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Migration struct {
	Key  string
	Up   func(context.Context, *sql.DB) error
	Down func(context.Context, *sql.DB) error
}

func (m *Migration) id() (int64, error) {
	parts := strings.SplitN(m.Key, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid id %q: '_' delimiter required", m.Key)
	}
	if parts[1] == "" {
		return 0, fmt.Errorf("invalid id %q: missing name part", m.Key)
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: non-integer id", m.Key)
	}
	if id < 0 {
		return 0, fmt.Errorf("invalid id %q: negative id", m.Key)
	}
	return int64(id), nil
}

type Migrator struct {
	db      *sql.DB
	log     *slog.Logger
	ids     []int64
	sources map[int64]*Migration
}

func New(db *sql.DB, log *slog.Logger, sources []*Migration) (*Migrator, error) {
	sm := make(map[int64]*Migration, len(sources))
	ids := make([]int64, len(sources))
	for i, mg := range sources {
		id, err := mg.id()
		if err != nil {
			return nil, err
		}
		ids[i] = id

		if _, ok := sm[id]; ok {
			return nil, fmt.Errorf("duplicate migration id: %d", id)
		}
		sm[id] = mg
	}
	if !slices.IsSorted(ids) {
		return nil, fmt.Errorf("illegal migration source order")
	}

	return &Migrator{db, log, ids, sm}, nil
}

func NewInitialized(ctx context.Context, db *sql.DB, log *slog.Logger, sources []*Migration) (*Migrator, error) {
	m, err := New(db, log, sources)
	if err != nil {
		return nil, err
	}
	if err := m.ensureTables(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Migrator) Init(ctx context.Context) error {
	return m.ensureTables(ctx)
}

func (m *Migrator) Up(ctx context.Context, to int64) (err error) {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer func() {
		if rlErr := m.release(ctx); err != nil {
			err = errors.Join(err, rlErr)
		}
	}()

	latest, err := m.latest(ctx)
	if err != nil {
		return err
	}

	var toApply []int64
	for _, id := range m.ids {
		if id > latest {
			toApply = append(toApply, id)
		}
	}
	if len(toApply) == 0 {
		m.log.InfoContext(ctx, "nothing to do")
		return nil
	}

	m.log.InfoContext(ctx, "applying migrations", "count", len(toApply))
	for _, id := range toApply {
		mg := m.sources[id]
		m.log.InfoContext(ctx, "applying migration", "id", mg.Key)
		if mg.Up == nil {
			return fmt.Errorf("invalid migration %q: no up function", mg.Key)
		}
		if err = mg.Up(ctx, m.db); err != nil {
			return err
		}
		_, err = m.db.ExecContext(ctx, "INSERT INTO schema_migrations (version_id) VALUES (?);", id)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) UpN(n int) {}

func (m *Migrator) Down(to int64) {}

func (m *Migrator) DownN(n int) {}

func (m *Migrator) latest(ctx context.Context) (int64, error) {
	state, err := m.getState(ctx)
	if err != nil {
		return 0, err
	}

	var latest int64 = -1
	if len(state) > 0 {
		latest = state[len(state)-1]
	}
	return latest, nil
}

func (m *Migrator) getState(ctx context.Context) ([]int64, error) {
	stmt := `SELECT version_id FROM schema_migrations`
	rows, err := m.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vids []int64

	for rows.Next() {
		var vid int64
		err = rows.Scan(&vid)
		if err != nil {
			return nil, err
		}
		vids = append(vids, vid)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	if !slices.IsSorted(vids) {
		return nil, fmt.Errorf("illegal db migration state")
	}

	return vids, nil
}

var ErrLocked = errors.New("db already locked for migrations")

func (m *Migrator) lock(ctx context.Context) error {
	return m.withTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// TODO: poll?
		var lockId int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM schema_lock").Scan(&lockId)
		if err == nil {
			return ErrLocked
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_lock DEFAULT VALUES;"); err != nil {
			return err
		}

		return nil
	})
}

func (m *Migrator) release(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, "DELETE FROM schema_lock;")
	return err
}

const (
	createLockTableStmt      = `CREATE TABLE IF NOT EXISTS schema_lock (id INTEGER PRIMARY KEY);`
	createMigrationTableStmt = `CREATE TABLE IF NOT EXISTS schema_migrations (
	 id INTEGER PRIMARY KEY
	,version_id INTEGER UNIQUE NOT NULL
	,applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
);`
)

func (m *Migrator) ensureTables(ctx context.Context) error {
	return m.withTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, createLockTableStmt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, createMigrationTableStmt); err != nil {
			return err
		}
		return nil
	})
}

func (m *Migrator) withTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) (err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		tx.Rollback()
		m.log.DebugContext(ctx, "rollback tx")
	}()

	m.log.DebugContext(ctx, "begin tx")
	err = fn(ctx, tx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	m.log.DebugContext(ctx, "commit tx")

	return nil
}
