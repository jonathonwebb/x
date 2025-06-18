package sqlite3store_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/jonathonwebb/x/up"
	"github.com/jonathonwebb/x/up/stores/sqlite3store"
	_ "github.com/mattn/go-sqlite3"
)

func TestNew(t *testing.T) {
	db := createTestDB(t)
	store := sqlite3store.New(db)

	if store == nil {
		t.Fatalf("New(db) = %v, want non-nil", store)
	}
}

func TestSqlite3Store_Init(t *testing.T) {
	t.Run("fresh_database", func(t *testing.T) {
		db := createTestDB(t)
		store := sqlite3store.New(db)

		if err := store.Init(t.Context()); err != nil {
			t.Errorf("store.Init(ctx) = %v, want no error", err)
		}
	})

	t.Run("existing_database", func(t *testing.T) {
		db := createTestDB(t)
		store := sqlite3store.New(db)

		if err := store.Init(t.Context()); err != nil {
			t.Errorf("store.Init(ctx) = %v, want no error", err)
		}
	})

	t.Run("partial_database", func(t *testing.T) {
		db := createTestDB(t)
		store := sqlite3store.New(db)
		_, err := db.ExecContext(t.Context(), "CREATE TABLE schema_lock (id INTEGER PRIMARY KEY)")
		if err != nil {
			t.Fatalf("failed to create table: %v", err)
		}

		if err := store.Init(t.Context()); err != nil {
			t.Errorf("store.Init(ctx) = %v, want no error", err)
		}
	})
}

func TestSqlite3Store_Lock(t *testing.T) {
	db := createTestDB(t)
	store := sqlite3store.New(db)
	if err := store.Init(t.Context()); err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	if err := store.Lock(t.Context()); err != nil {
		t.Errorf("store.Lock(ctx) = %v, want no error", err)
	}

	if !lockExists(t, store) {
		t.Error("expected lock to exist after Lock()")
	}

	if err := store.Lock(t.Context()); !errors.Is(err, up.ErrLocked) {
		t.Errorf("store.Lock(ctx) = %v, want ErrLocked", err)
	}
}

func TestSqlite3Store_Release(t *testing.T) {
	db := createTestDB(t)
	store := sqlite3store.New(db)
	if err := store.Init(t.Context()); err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	if err := store.Lock(t.Context()); err != nil {
		t.Fatalf("failed to lock: %v", err)
	}

	if !lockExists(t, store) {
		t.Error("expected lock to exist after Lock()")
	}

	if err := store.Release(t.Context()); err != nil {
		t.Errorf("store.Release(ctx) = %v, want no error", err)
	}

	if lockExists(t, store) {
		t.Error("expected lock to not exist after Release()")
	}

	if err := store.Lock(t.Context()); err != nil {
		t.Errorf("store.Lock(ctx) = %v, want no error", err)
	}
}

func TestSqlite3Store_Version(t *testing.T) {
	tests := []struct {
		name     string
		versions []int64
	}{
		{
			name:     "no_migrations",
			versions: []int64{},
		},
		{
			name:     "single_migration",
			versions: []int64{1},
		},
		{
			name:     "multiple_migrations",
			versions: []int64{1, 2, 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := createTestDB(t)
			store := sqlite3store.New(db)
			if err := store.Init(t.Context()); err != nil {
				t.Fatalf("failed to init: %v", err)
			}

			for _, v := range test.versions {
				if err := store.Insert(t.Context(), v); err != nil {
					t.Fatalf("failed to insert version %d: %v", v, err)
				}
			}

			var want int64
			if len(test.versions) > 0 {
				want = test.versions[len(test.versions)-1]
			}
			got, err := store.Version(t.Context())
			if err != nil {
				t.Errorf("store.Version(ctx) = %v, want no error", err)
			}
			if got != want {
				t.Errorf("store.Version(ctx) = %d, want %d", got, want)
			}
		})
	}
}

func TestSqlite3Store_Insert(t *testing.T) {
	tests := []struct {
		name          string
		versions      []int64
		insertVersion int64
		wantErr       bool
	}{
		{
			name:          "insert_first",
			versions:      []int64{},
			insertVersion: 1,
			wantErr:       false,
		},
		{
			name:          "insert_additional",
			versions:      []int64{1, 2},
			insertVersion: 3,
			wantErr:       false,
		},
		{
			name:          "insert_duplicate",
			versions:      []int64{1, 2},
			insertVersion: 2,
			wantErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := createTestDB(t)
			store := sqlite3store.New(db)
			if err := store.Init(t.Context()); err != nil {
				t.Fatalf("failed to initialize store: %v", err)
			}

			for _, v := range test.versions {
				if err := store.Insert(t.Context(), v); err != nil {
					t.Fatalf("failed to insert version %d: %v", v, err)
				}
			}

			err := store.Insert(t.Context(), test.insertVersion)
			if err != nil && !test.wantErr {
				t.Errorf("store.Insert(ctx, %d) error = %v, want no error", test.insertVersion, err)
			}
			if err == nil && test.wantErr {
				t.Errorf("store.Insert(ctx, %d) error = %v, want error", test.insertVersion, err)
			}

			if !test.wantErr {
				versions := currentVersions(t, store)
				wantVersions := append(test.versions, test.insertVersion)
				if !slices.Equal(versions, wantVersions) {
					t.Errorf("got versions %v, want %v", versions, wantVersions)
				}
			}
		})
	}
}

func TestSqlite3Store_Remove(t *testing.T) {
	tests := []struct {
		name          string
		versions      []int64
		removeVersion int64
		wantErr       bool
	}{
		{
			name:          "remove_existing_version",
			versions:      []int64{1, 2, 3},
			removeVersion: 3,
			wantErr:       false,
		},
		{
			name:          "remove_nonexistent_version",
			versions:      []int64{1, 2, 3},
			removeVersion: 4,
			wantErr:       true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := createTestDB(t)
			store := sqlite3store.New(db)
			if err := store.Init(t.Context()); err != nil {
				t.Fatalf("failed to initialize store: %v", err)
			}

			for _, v := range test.versions {
				if err := store.Insert(t.Context(), v); err != nil {
					t.Fatalf("failed to insert version %d: %v", v, err)
				}
			}

			err := store.Remove(t.Context(), test.removeVersion)

			if err == nil && test.wantErr {
				t.Errorf("store.Remove(ctx, %d) error = %v, want error", test.removeVersion, err)
			}

			if !test.wantErr {
				versions := currentVersions(t, store)
				wantVersions := slices.DeleteFunc(test.versions, func(v int64) bool { return v == test.removeVersion })
				if !slices.Equal(versions, wantVersions) {
					t.Errorf("got versions %v, want %v", versions, wantVersions)
				}
			}
		})
	}
}

func createTestDB(t testing.TB) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func lockExists(t testing.TB, store *sqlite3store.Sqlite3Store) bool {
	t.Helper()
	var count int
	err := store.DB().QueryRow("SELECT COUNT(*) FROM schema_lock WHERE id = 1").Scan(&count)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("failed to check lock state: %v", err)
	}
	return count > 0
}

func currentVersions(t testing.TB, store *sqlite3store.Sqlite3Store) []int64 {
	t.Helper()
	var versions []int64
	rows, err := store.DB().Query("SELECT version_id FROM schema_migrations ORDER BY version_id ASC")
	if err != nil {
		t.Fatalf("failed to get current versions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("failed to scan version: %v", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate over versions: %v", err)
	}
	return versions
}
