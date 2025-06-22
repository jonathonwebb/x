// Package up provides a simple database migration tool.
package up

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

const (
	RunTargetLatest     = -1
	RevertTargetInitial = 0
)

// A Migrator stores migrations and provides methods to apply or revert them.
type Migrator struct {
	Store     Store
	Sources   []*Migration
	LogFunc   func(s string)
	DebugFunc func(s string)

	HoldLockOnFailure bool
}

func (m *Migrator) log(f string, a ...any) {
	if m.LogFunc != nil {
		m.LogFunc(fmt.Sprintf(f, a...))
	}
}

func (m *Migrator) debug(f string, a ...any) {
	if m.DebugFunc != nil {
		m.DebugFunc(fmt.Sprintf(f, a...))
	}
}

func (m *Migrator) check() error {
	var prev int64 = 0
	seen := map[int64]bool{}

	for _, migration := range m.Sources {
		if migration.Version <= 0 {
			return fmt.Errorf("migration version must be > 0, got %d", migration.Version)
		}
		if migration.Version < prev {
			return fmt.Errorf("migration order: %d found after %d", migration.Version, prev)
		}
		if _, ok := seen[migration.Version]; ok {
			return fmt.Errorf("duplicate migration version: %d", migration.Version)
		} else {
			seen[migration.Version] = true
		}
		prev = migration.Version
	}

	return nil
}

// Run applies migrations up to and including the specified version. The special
// value -1 applies all pending migrations.
func (m *Migrator) Run(ctx context.Context, to int64) (err error) {
	if err := m.check(); err != nil {
		return fmt.Errorf("invalid sources: %w", err)
	}

	if err := m.Store.Init(ctx); err != nil {
		return fmt.Errorf("failed to init version store: %w", err)
	}

	if err := m.Store.Lock(ctx); err != nil {
		return fmt.Errorf("failed to get version store lock: %w", err)
	}
	shouldRelease := true
	defer func() {
		if shouldRelease {
			m.debug("releasing version store lock")
			if rlErr := m.Store.Release(ctx); rlErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to release version store lock: %w", rlErr))
			}
		} else {
			m.debug("holding lock due to failure")
		}
	}()

	var remoteVersion int64 = 0
	remoteVersion, err = m.Store.Version(ctx)
	if err != nil {
		if !errors.Is(err, ErrInitialVersion) {
			return fmt.Errorf("failed to get version store state: %w", err)
		}
	}
	m.debug("current version: %d", remoteVersion)

	var toApply []*Migration
	for _, migration := range m.Sources {
		if migration.Version > remoteVersion && (to == RunTargetLatest || migration.Version <= to) {
			toApply = append(toApply, migration)
		}
	}

	m.log("%d migrations to apply", len(toApply))
	if len(toApply) == 0 {
		return nil
	}

	if m.HoldLockOnFailure {
		shouldRelease = false
	}

	for _, migration := range m.Sources {
		if migration.Version > remoteVersion && migration.Version <= to {
			m.debug("applying migration: %d", migration.Version)

			if err := migration.Run(ctx, m.Store.DB()); err != nil {
				return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
			}

			if err := m.Store.Insert(ctx, migration.Version); err != nil {
				return fmt.Errorf("failed to insert migration %d: %w", migration.Version, err)
			}
		}
	}

	shouldRelease = true
	return nil
}

// Revert reverses migrations down to and excluding the provided version. The
// special value 0 reverts all migrations.
func (m *Migrator) Revert(ctx context.Context, to int64) (err error) {
	if err := m.check(); err != nil {
		return fmt.Errorf("invalid sources: %w", err)
	}

	migrationCmpFunc := func(s *Migration, t int64) int {
		if s.Version < t {
			return -1
		}
		if s.Version > t {
			return 1
		}
		return 0
	}

	if to != RevertTargetInitial {
		_, ok := slices.BinarySearchFunc(m.Sources, to, migrationCmpFunc)
		if !ok {
			return fmt.Errorf("missing target version migration: %d", to)
		}
	}

	if err := m.Store.Init(ctx); err != nil {
		return fmt.Errorf("failed to init version store: %w", err)
	}

	if err := m.Store.Lock(ctx); err != nil {
		return fmt.Errorf("failed to get version store lock: %w", err)
	}

	shouldRelease := true
	defer func() {
		if shouldRelease {
			m.debug("releasing version store lock")
			if rlErr := m.Store.Release(ctx); rlErr != nil {
				err = errors.Join(err, fmt.Errorf("failed to release version store lock: %w", rlErr))
			}
		} else {
			m.debug("holding lock due to failure")
		}
	}()

	var remoteVersion int64

	remoteVersion, err = m.Store.Version(ctx)
	if err != nil {
		if errors.Is(err, ErrInitialVersion) {
			m.log("already at initial version, nothing to revert")
			return nil
		}
		return fmt.Errorf("failed to get version store state: %w", err)
	}
	m.debug("current version: %d", remoteVersion)

	if m.HoldLockOnFailure {
		shouldRelease = false
	}

	revertCount := 0
	for {
		if remoteVersion <= to {
			m.debug("reached target version %d, stopping", to)
			break
		}

		idx, ok := slices.BinarySearchFunc(m.Sources, remoteVersion, migrationCmpFunc)
		if !ok {
			return fmt.Errorf("missing remote version migration: %d", remoteVersion)
		}

		migration := m.Sources[idx]
		m.debug("reverting migration: %d", migration.Version)

		if err := migration.Revert(ctx, m.Store.DB()); err != nil {
			return fmt.Errorf("failed to revert migration %d: %w", migration.Version, err)
		}

		if err := m.Store.Remove(ctx, migration.Version); err != nil {
			return fmt.Errorf("failed to delete migration %d from version store: %w", migration.Version, err)
		}

		revertCount++

		remoteVersion, err = m.Store.Version(ctx)
		if err != nil {
			if errors.Is(err, ErrInitialVersion) {
				m.log("reverted %d migrations", revertCount)
				return nil
			}
			return fmt.Errorf("failed to get version store state: %w", err)
		}
	}

	m.log("reverted %d migrations", revertCount)

	shouldRelease = true
	return nil
}
