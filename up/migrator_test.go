package up_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/jonathonwebb/x/up"
)

type fakeStore struct {
	versions []int64
	applied  []int64
	reverted []int64
	locked   bool
	mu       sync.Mutex

	initCalls    int
	lockCalls    int
	releaseCalls int
	versionCalls int
	insertCalls  int
	removeCalls  int

	initFunc    func(context.Context, *fakeStore) error
	lockFunc    func(context.Context, *fakeStore) error
	releaseFunc func(context.Context, *fakeStore) error
	versionFunc func(context.Context, *fakeStore) (int64, error)
	insertFunc  func(context.Context, int64, *fakeStore) error
	removeFunc  func(context.Context, int64, *fakeStore) error
}

func defaultInitFunc(_ context.Context, _ *fakeStore) error {
	return nil
}

func defaultLockFunc(_ context.Context, s *fakeStore) error {
	if s.locked {
		return up.ErrLocked
	}
	s.mu.Lock()
	s.locked = true
	s.mu.Unlock()
	return nil
}

func defaultReleaseFunc(_ context.Context, s *fakeStore) error {
	s.mu.Lock()
	s.locked = false
	s.mu.Unlock()
	return nil
}

func defaultVersionFunc(_ context.Context, s *fakeStore) (int64, error) {
	if len(s.versions) == 0 {
		return 0, up.ErrInitialVersion
	}
	return s.versions[len(s.versions)-1], nil
}

func defaultInsertFunc(_ context.Context, v int64, s *fakeStore) error {
	s.mu.Lock()
	s.versions = append(s.versions, v)
	s.applied = append(s.applied, v)
	s.mu.Unlock()
	return nil
}

func defaultRemoveFunc(_ context.Context, v int64, s *fakeStore) error {
	if len(s.versions) > 0 {
		s.mu.Lock()
		s.versions = s.versions[:len(s.versions)-1]
		s.reverted = append(s.reverted, v)
		s.mu.Unlock()
	}
	return nil
}

func (s *fakeStore) DB() *sql.DB { return nil }

func (s *fakeStore) Init(ctx context.Context) error {
	s.initCalls += 1
	if s.initFunc != nil {
		return s.initFunc(ctx, s)
	}
	return defaultInitFunc(ctx, s)
}

func (s *fakeStore) Lock(ctx context.Context) error {
	s.lockCalls += 1
	if s.lockFunc != nil {
		return s.lockFunc(ctx, s)
	}
	return defaultLockFunc(ctx, s)
}

func (s *fakeStore) Release(ctx context.Context) error {
	s.releaseCalls += 1
	if s.releaseFunc != nil {
		return s.releaseFunc(ctx, s)
	}
	return defaultReleaseFunc(ctx, s)
}

func (s *fakeStore) Version(ctx context.Context) (int64, error) {
	s.versionCalls += 1
	if s.versionFunc != nil {
		return s.versionFunc(ctx, s)
	}
	return defaultVersionFunc(ctx, s)
}

func (s *fakeStore) Insert(ctx context.Context, v int64) error {
	s.insertCalls += 1
	if s.insertFunc != nil {
		return s.insertFunc(ctx, v, s)
	}
	return defaultInsertFunc(ctx, v, s)
}

func (s *fakeStore) Remove(ctx context.Context, v int64) error {
	s.removeCalls += 1
	if s.removeFunc != nil {
		return s.removeFunc(ctx, v, s)
	}
	return defaultRemoveFunc(ctx, v, s)
}

func noopMigration(ctx context.Context, db *sql.DB) error { return nil }

func errorMigration(msg string) func(context.Context, *sql.DB) error {
	return func(_ context.Context, _ *sql.DB) error {
		return errors.New(msg)
	}
}

// Helper to create standard migrations for testing
func createMigrations(versions ...int64) []*up.Migration {
	migrations := make([]*up.Migration, len(versions))
	for i, v := range versions {
		migrations[i] = &up.Migration{
			Version:    v,
			RunFunc:    noopMigration,
			RevertFunc: noopMigration,
		}
	}
	return migrations
}

func TestMigrator_Run(t *testing.T) {
	tests := []struct {
		name              string
		initialVersions   []int64
		migrations        []*up.Migration
		target            int64
		holdLockOnFailure bool
		storeConfig       func(*fakeStore) // Configure store behavior

		wantErr      bool
		wantVersions []int64
		wantApplied  []int64
		wantLocked   bool
	}{
		// Basic scenarios
		{
			name:            "no_migrations_no_target",
			initialVersions: []int64{},
			migrations:      []*up.Migration{},
			target:          0,
			wantVersions:    []int64{},
			wantApplied:     []int64{},
		},
		{
			name:            "fresh_database_apply_all",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			wantVersions:    []int64{1, 2, 3},
			wantApplied:     []int64{1, 2, 3},
		},
		{
			name:            "partial_applied_continue",
			initialVersions: []int64{1},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			wantVersions:    []int64{1, 2, 3},
			wantApplied:     []int64{2, 3},
		},
		{
			name:            "all_already_applied",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			wantVersions:    []int64{1, 2, 3},
			wantApplied:     []int64{}, // Nothing new applied
		},
		{
			name:            "target_below_current_version",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          2,
			wantVersions:    []int64{1, 2, 3},
			wantApplied:     []int64{}, // No migrations applied
		},

		// Version selection scenarios
		{
			name:            "skip_to_specific_version",
			initialVersions: []int64{1},
			migrations:      createMigrations(1, 2, 3, 4),
			target:          3,
			wantVersions:    []int64{1, 2, 3},
			wantApplied:     []int64{2, 3},
		},
		{
			name:            "gaps_in_migration_versions",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 5, 10),
			target:          10,
			wantVersions:    []int64{1, 5, 10},
			wantApplied:     []int64{1, 5, 10},
		},
		{
			name:            "target_version_not_in_migrations",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 4),
			target:          3,
			wantVersions:    []int64{1, 2},
			wantApplied:     []int64{1, 2},
		},

		// Migration validation errors
		{
			name:            "zero_version",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: 0, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       1,
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "negative_version",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: -1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       1,
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "misordered_migrations",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: 3, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       3,
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "duplicate_versions",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       3,
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},

		// Store operation errors
		{
			name:            "store_init_error",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			storeConfig: func(s *fakeStore) {
				s.initFunc = func(ctx context.Context, _ *fakeStore) error {
					return fmt.Errorf("init error")
				}
			},
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "store_lock_error",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			storeConfig: func(s *fakeStore) {
				s.lockFunc = func(ctx context.Context, _ *fakeStore) error {
					return fmt.Errorf("lock error")
				}
			},
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "store_already_locked",
			initialVersions: []int64{},
			migrations:      createMigrations(1),
			target:          1,
			storeConfig: func(s *fakeStore) {
				s.locked = true
			},
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
			wantLocked:   true,
		},
		{
			name:            "store_version_error",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			storeConfig: func(s *fakeStore) {
				s.versionFunc = func(ctx context.Context, s *fakeStore) (int64, error) {
					return 0, fmt.Errorf("version error")
				}
			},
			wantErr:      true,
			wantVersions: []int64{},
			wantApplied:  []int64{},
		},
		{
			name:            "store_insert_error",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			storeConfig: func(s *fakeStore) {
				s.insertFunc = func(ctx context.Context, v int64, s *fakeStore) error {
					if s.insertCalls == 2 { // Fail on second migration
						return fmt.Errorf("insert error")
					}
					return defaultInsertFunc(ctx, v, s)
				}
			},
			wantErr:      true,
			wantVersions: []int64{1},
			wantApplied:  []int64{1},
		},
		{
			name:            "store_release_error",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          3,
			storeConfig: func(s *fakeStore) {
				s.releaseFunc = func(ctx context.Context, _ *fakeStore) error {
					return fmt.Errorf("release error")
				}
			},
			wantErr:      true, // Should get joined error
			wantVersions: []int64{1, 2, 3},
			wantApplied:  []int64{1, 2, 3},
			wantLocked:   true,
		},

		// Migration execution errors
		{
			name:            "migration_run_error",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: errorMigration("run error"), RevertFunc: noopMigration},
				{Version: 3, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       3,
			wantErr:      true,
			wantVersions: []int64{1},
			wantApplied:  []int64{1},
		},
		{
			name:            "migration_run_error_hold_lock",
			initialVersions: []int64{},
			migrations: []*up.Migration{
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: errorMigration("run error"), RevertFunc: noopMigration},
				{Version: 3, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:            3,
			holdLockOnFailure: true,
			wantErr:           true,
			wantVersions:      []int64{1},
			wantApplied:       []int64{1},
			wantLocked:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				versions: slices.Clone(tt.initialVersions),
			}
			if tt.storeConfig != nil {
				tt.storeConfig(store)
			}

			migrator := &up.Migrator{
				Store:             store,
				Sources:           tt.migrations,
				HoldLockOnFailure: tt.holdLockOnFailure,
			}

			_, err := migrator.Run(context.Background(), tt.target)

			if tt.wantErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			if !slices.Equal(tt.wantVersions, store.versions) {
				t.Errorf("versions mismatch\nwant: %v\ngot:  %v", tt.wantVersions, store.versions)
			}
			if !slices.Equal(tt.wantApplied, store.applied) {
				t.Errorf("applied mismatch\nwant: %v\ngot:  %v", tt.wantApplied, store.applied)
			}
			if tt.wantLocked != store.locked {
				t.Errorf("lock state mismatch: want %v, got %v", tt.wantLocked, store.locked)
			}
		})
	}
}

func TestMigrator_Revert(t *testing.T) {
	tests := []struct {
		name              string
		initialVersions   []int64
		migrations        []*up.Migration
		target            int64
		holdLockOnFailure bool
		storeConfig       func(*fakeStore)

		wantErr      bool
		wantVersions []int64
		wantReverted []int64
		wantLocked   bool
	}{
		{
			name:            "no_migrations_to_revert",
			initialVersions: []int64{1},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			wantVersions:    []int64{1},
			wantReverted:    []int64{},
		},
		{
			name:            "revert_all_migrations",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          0, // Special value to revert all (changed from -1 to 0)
			wantVersions:    []int64{},
			wantReverted:    []int64{3, 2, 1},
		},
		{
			name:            "revert_partial_migrations",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			wantVersions:    []int64{1},
			wantReverted:    []int64{3, 2},
		},
		{
			name:            "already_at_target_version",
			initialVersions: []int64{1, 2},
			migrations:      createMigrations(1, 2, 3),
			target:          2,
			wantVersions:    []int64{1, 2},
			wantReverted:    []int64{},
		},
		{
			name:            "empty_database",
			initialVersions: []int64{},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			wantVersions:    []int64{},
			wantReverted:    []int64{},
		},
		{
			name:            "target_below_zero",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          -1,
			wantErr:         true,
			wantVersions:    []int64{1, 2, 3},
			wantReverted:    []int64{},
		},
		{
			name:            "target_version_not_in_migrations",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          5,
			wantErr:         true,
			wantVersions:    []int64{1, 2, 3},
			wantReverted:    []int64{},
		},
		{
			name:            "missing_remote_version_migration",
			initialVersions: []int64{1, 2, 5},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			wantErr:         true,
			wantVersions:    []int64{1, 2, 5},
			wantReverted:    []int64{},
		},
		{
			name:            "store_version_error_initial",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			storeConfig: func(s *fakeStore) {
				s.versionFunc = func(ctx context.Context, s *fakeStore) (int64, error) {
					return 0, fmt.Errorf("version error")
				}
			},
			wantErr:      true,
			wantVersions: []int64{1, 2, 3},
			wantReverted: []int64{},
		},
		{
			name:            "store_version_error_subsequent",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			storeConfig: func(s *fakeStore) {
				s.versionFunc = func(ctx context.Context, s *fakeStore) (int64, error) {
					if s.versionCalls > 1 {
						return 0, fmt.Errorf("version error")
					}
					return defaultVersionFunc(ctx, s)
				}
			},
			wantErr:      true,
			wantVersions: []int64{1, 2},
			wantReverted: []int64{3},
		},
		{
			name:            "store_remove_error",
			initialVersions: []int64{1, 2, 3},
			migrations:      createMigrations(1, 2, 3),
			target:          1,
			storeConfig: func(s *fakeStore) {
				s.removeFunc = func(ctx context.Context, v int64, s *fakeStore) error {
					if s.removeCalls == 2 { // Fail on second remove
						return fmt.Errorf("remove error")
					}
					return defaultRemoveFunc(ctx, v, s)
				}
			},
			wantErr:      true,
			wantVersions: []int64{1, 2},
			wantReverted: []int64{3},
		},
		{
			name:            "migration_revert_error",
			initialVersions: []int64{1, 2, 3},
			migrations: []*up.Migration{
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: noopMigration, RevertFunc: errorMigration("revert error")},
				{Version: 3, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:       1,
			wantErr:      true,
			wantVersions: []int64{1, 2},
			wantReverted: []int64{3},
		},
		{
			name:            "migration_revert_error_hold_lock",
			initialVersions: []int64{1, 2, 3},
			migrations: []*up.Migration{
				{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
				{Version: 2, RunFunc: noopMigration, RevertFunc: errorMigration("revert error")},
				{Version: 3, RunFunc: noopMigration, RevertFunc: noopMigration},
			},
			target:            1,
			holdLockOnFailure: true,
			wantErr:           true,
			wantVersions:      []int64{1, 2},
			wantReverted:      []int64{3},
			wantLocked:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				versions: slices.Clone(tt.initialVersions),
			}
			if tt.storeConfig != nil {
				tt.storeConfig(store)
			}

			migrator := &up.Migrator{
				Store:             store,
				Sources:           tt.migrations,
				HoldLockOnFailure: tt.holdLockOnFailure,
			}

			_, err := migrator.Revert(context.Background(), tt.target)

			if tt.wantErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			if !slices.Equal(tt.wantVersions, store.versions) {
				t.Errorf("versions mismatch\nwant: %v\ngot:  %v", tt.wantVersions, store.versions)
			}
			if !slices.Equal(tt.wantReverted, store.reverted) {
				t.Errorf("reverted mismatch\nwant: %v\ngot:  %v", tt.wantReverted, store.reverted)
			}
			if tt.wantLocked != store.locked {
				t.Errorf("lock state mismatch: want %v, got %v", tt.wantLocked, store.locked)
			}
		})
	}
}

func TestMigrator_ValidationConsistency(t *testing.T) {
	invalidMigrations := [][]*up.Migration{
		{
			{Version: 0, RunFunc: noopMigration, RevertFunc: noopMigration},
		},
		{
			{Version: -1, RunFunc: noopMigration, RevertFunc: noopMigration},
		},
		{
			{Version: 2, RunFunc: noopMigration, RevertFunc: noopMigration},
			{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
		},
		{
			{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
			{Version: 1, RunFunc: noopMigration, RevertFunc: noopMigration},
		},
	}

	for i, migrations := range invalidMigrations {
		t.Run(fmt.Sprintf("invalid_migrations_%d", i), func(t *testing.T) {
			store := &fakeStore{}
			migrator := &up.Migrator{
				Store:   store,
				Sources: migrations,
			}

			// Both Run and Revert should fail validation consistently
			_, upErr := migrator.Run(context.Background(), 1)
			_, downErr := migrator.Revert(context.Background(), 0)

			if upErr == nil {
				t.Error("Run should have failed validation")
			}
			if downErr == nil {
				t.Error("Revert should have failed validation")
			}

			// Store should not have been touched
			if store.initCalls > 0 || store.lockCalls > 0 {
				t.Error("Store should not be accessed when validation fails")
			}
		})
	}
}

func TestMigrator_InitialVersionHandling(t *testing.T) {
	t.Run("run_from_initial_version", func(t *testing.T) {
		store := &fakeStore{versions: []int64{}}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2),
		}

		_, err := migrator.Run(context.Background(), 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []int64{1, 2}
		if !slices.Equal(want, store.versions) {
			t.Errorf("want %v, got %v", want, store.versions)
		}
	})

	t.Run("revert_from_initial_version", func(t *testing.T) {
		store := &fakeStore{versions: []int64{}}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2),
		}

		_, err := migrator.Revert(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(store.reverted) > 0 {
			t.Errorf("expected no reversions, got %v", store.reverted)
		}
	})
}

func TestMigrator_LockBehavior(t *testing.T) {
	t.Run("successful_operations_release_lock", func(t *testing.T) {
		store := &fakeStore{}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2),
		}

		_, err := migrator.Run(context.Background(), 2)
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}

		if store.lockCalls != 1 || store.releaseCalls != 1 {
			t.Errorf("expected 1 lock/release call, got %d/%d", store.lockCalls, store.releaseCalls)
		}
		if store.locked {
			t.Error("lock should be released after successful operation")
		}

		_, err = migrator.Revert(context.Background(), 1)
		if err != nil {
			t.Fatalf("revert failed: %v", err)
		}

		if store.lockCalls != 2 || store.releaseCalls != 2 {
			t.Errorf("expected 2 lock/release calls, got %d/%d", store.lockCalls, store.releaseCalls)
		}
		if store.locked {
			t.Error("lock should be released after successful operation")
		}
	})

	t.Run("hold_lock_on_failure_flag", func(t *testing.T) {
		tests := []struct {
			name     string
			holdLock bool
			wantLock bool
		}{
			{"default_behavior", false, false},
			{"hold_lock_enabled", true, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := &fakeStore{}
				migrator := &up.Migrator{
					Store: store,
					Sources: []*up.Migration{
						{Version: 1, RunFunc: errorMigration("test error"), RevertFunc: noopMigration},
					},
					HoldLockOnFailure: tt.holdLock,
				}

				_, err := migrator.Run(context.Background(), 1)
				if err == nil {
					t.Error("expected error from migration")
				}

				if store.locked != tt.wantLock {
					t.Errorf("lock state: want %v, got %v", tt.wantLock, store.locked)
				}
			})
		}
	})
}

func TestMigrator_StoreCallPatterns(t *testing.T) {
	t.Run("run_call_sequence", func(t *testing.T) {
		store := &fakeStore{}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2),
		}

		_, err := migrator.Run(context.Background(), 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if store.initCalls != 1 {
			t.Errorf("expected 1 init call, got %d", store.initCalls)
		}
		if store.lockCalls != 1 {
			t.Errorf("expected 1 lock call, got %d", store.lockCalls)
		}
		if store.versionCalls != 1 {
			t.Errorf("expected 1 version call, got %d", store.versionCalls)
		}
		if store.insertCalls != 2 {
			t.Errorf("expected 2 insert calls, got %d", store.insertCalls)
		}
		if store.releaseCalls != 1 {
			t.Errorf("expected 1 release call, got %d", store.releaseCalls)
		}
	})

	t.Run("revert_call_sequence", func(t *testing.T) {
		store := &fakeStore{
			versions: []int64{1, 2, 3},
		}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2, 3),
		}

		_, err := migrator.Revert(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedVersionCalls := 3
		if store.versionCalls != expectedVersionCalls {
			t.Errorf("expected %d version calls, got %d", expectedVersionCalls, store.versionCalls)
		}
		if store.removeCalls != 2 {
			t.Errorf("expected 2 remove calls, got %d", store.removeCalls)
		}
	})
}

func TestMigrator_BoundaryConditions(t *testing.T) {
	t.Run("zero_target_version", func(t *testing.T) {
		store := &fakeStore{}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2, 3),
		}

		_, err := migrator.Run(context.Background(), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(store.applied) > 0 {
			t.Errorf("expected no migrations applied, got %v", store.applied)
		}
	})

	t.Run("very_high_target_version", func(t *testing.T) {
		store := &fakeStore{}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2, 3),
		}

		_, err := migrator.Run(context.Background(), 9999)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []int64{1, 2, 3}
		if !slices.Equal(want, store.applied) {
			t.Errorf("want %v, got %v", want, store.applied)
		}
	})

	t.Run("single_migration", func(t *testing.T) {
		store := &fakeStore{versions: []int64{1, 2}}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1, 2, 3),
		}

		_, err := migrator.Run(context.Background(), 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []int64{3}
		if !slices.Equal(want, store.applied) {
			t.Errorf("want %v, got %v", want, store.applied)
		}
	})
}

func TestMigrator_ConcurrentSafety(t *testing.T) {
	t.Run("concurrent_lock_attempts", func(t *testing.T) {
		store := &fakeStore{}
		migrator := &up.Migrator{
			Store:   store,
			Sources: createMigrations(1),
		}

		err := store.Lock(context.Background())
		if err != nil {
			t.Fatalf("failed to pre-lock store: %v", err)
		}

		_, err = migrator.Run(context.Background(), 1)
		if err == nil {
			t.Error("expected error when store is already locked")
		}
		if !store.locked {
			t.Error("store should remain locked")
		}
	})
}

func TestMigrator_Integration(t *testing.T) {
	store := &fakeStore{}
	migrations := createMigrations(1, 2, 3, 4, 5)
	migrator := &up.Migrator{
		Store:   store,
		Sources: migrations,
	}

	steps := []struct {
		target   int64
		wantVers []int64
	}{
		{target: 2, wantVers: []int64{1, 2}},
		{target: 4, wantVers: []int64{1, 2, 3, 4}},
		{target: 5, wantVers: []int64{1, 2, 3, 4, 5}},
	}

	for i, step := range steps {
		t.Run(fmt.Sprintf("step_%d_to_%d", i+1, step.target), func(t *testing.T) {
			_, err := migrator.Run(context.Background(), step.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(step.wantVers, store.versions) {
				t.Errorf("want %v, got %v", step.wantVers, store.versions)
			}
		})
	}

	rollbackSteps := []struct {
		target   int64
		wantVers []int64
	}{
		{target: 3, wantVers: []int64{1, 2, 3}},
		{target: 1, wantVers: []int64{1}},
		{target: 0, wantVers: []int64{}}, // Changed from -1 to 0
	}

	for i, step := range rollbackSteps {
		t.Run(fmt.Sprintf("rollback_%d_to_%d", i+1, step.target), func(t *testing.T) {
			_, err := migrator.Revert(context.Background(), step.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(step.wantVers, store.versions) {
				t.Errorf("want %v, got %v", step.wantVers, store.versions)
			}
		})
	}
}
