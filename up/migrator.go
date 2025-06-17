package up

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
)

const (
	RunTargetLatest     = -1
	RevertTargetInitial = 0
)

type Migrator struct {
	Store   Store
	Sources []*Migration
	LogW    io.Writer
	DebugW  io.Writer

	HoldLockOnFailure bool
}

func (m *Migrator) log(f string, a ...any) {
	if m.LogW != nil {
		fmt.Fprintf(m.LogW, f, a...)
	}
}

func (m *Migrator) debug(f string, a ...any) {
	if m.DebugW != nil {
		fmt.Fprintf(m.DebugW, f, a...)
	}
}

func (m *Migrator) check() error {
	m.debug("validating %d migration sources\n", len(m.Sources))

	var prev int64 = 0
	seen := map[int64]bool{}

	for _, migration := range m.Sources {
		m.debug("checking migration version %d\n", migration.Version)

		if migration.Version <= 0 {
			m.debug("validation failed: migration version must be > 0, got %d\n", migration.Version)
			return fmt.Errorf("migration version must be > 0, got %d", migration.Version)
		}
		if migration.Version < prev {
			m.debug("validation failed: migration %d found after %d\n", migration.Version, prev)
			return fmt.Errorf("migration order: %d found after %d", migration.Version, prev)
		}
		if _, ok := seen[migration.Version]; ok {
			m.debug("validation failed: duplicate migration version %d\n", migration.Version)
			return fmt.Errorf("duplicate migration version: %d", migration.Version)
		} else {
			seen[migration.Version] = true
		}
		prev = migration.Version
	}

	m.debug("migration source validation completed successfully\n")
	return nil
}

func (m *Migrator) Run(ctx context.Context, to int64) (err error) {
	m.log("starting migration run to version %d\n", to)
	m.debug("run parameters: target=%d, sources=%d, holdLockOnFailure=%t\n", to, len(m.Sources), m.HoldLockOnFailure)

	defer func() {
		if err == nil {
			m.log("migration run completed successfully\n")
		} else {
			m.log("migration run failed: %v\n", err)
		}
	}()

	if err := m.check(); err != nil {
		return fmt.Errorf("invalid sources: %w", err)
	}

	m.debug("initializing version store\n")
	if err := m.Store.Init(ctx); err != nil {
		return fmt.Errorf("failed to init version store: %w", err)
	}

	m.debug("acquiring version store lock\n")
	if err := m.Store.Lock(ctx); err != nil {
		return fmt.Errorf("failed to get version store lock: %w", err)
	}
	shouldRelease := true
	defer func() {
		if shouldRelease {
			m.debug("releasing version store lock\n")
			if rlErr := m.Store.Release(ctx); rlErr != nil {
				m.debug("failed to release lock: %v\n", rlErr)
				err = errors.Join(err, fmt.Errorf("failed to release version store lock: %w", rlErr))
			} else {
				m.debug("version store lock released successfully\n")
			}
		} else {
			m.debug("holding lock due to failure (HoldLockOnFailure=true)\n")
		}
	}()

	m.debug("getting current version from store\n")
	var remoteVersion int64 = 0
	remoteVersion, err = m.Store.Version(ctx)
	if err != nil {
		if !errors.Is(err, ErrInitialVersion) {
			return fmt.Errorf("failed to get version store state: %w", err)
		}
		m.debug("store is at initial version (no migrations applied yet)\n")
	}
	m.log("current version: %d\n", remoteVersion)

	var toApply []*Migration
	for _, migration := range m.Sources {
		if migration.Version > remoteVersion && (to == RunTargetLatest || migration.Version <= to) {
			toApply = append(toApply, migration)
			m.debug("migration %d queued for application\n", migration.Version)
		}
	}

	m.log("found %d migrations to apply\n", len(toApply))
	if len(toApply) == 0 {
		m.log("no migrations to apply, already at target version\n")
		return nil
	}

	if m.HoldLockOnFailure {
		shouldRelease = false
		m.debug("configured to hold lock on failure\n")
	}

	for _, migration := range m.Sources {
		if migration.Version > remoteVersion && migration.Version <= to {
			m.log("applying migration: %d\n", migration.Version)
			m.debug("executing migration %d run function\n", migration.Version)

			if err := migration.Run(ctx, m.Store.DB()); err != nil {
				m.debug("migration %d run function failed: %v\n", migration.Version, err)
				return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
			}

			m.debug("recording migration %d in version store\n", migration.Version)
			if err := m.Store.Insert(ctx, migration.Version); err != nil {
				m.debug("failed to record migration %d: %v\n", migration.Version, err)
				return fmt.Errorf("failed to insert migration %d in version store: %w", migration.Version, err)
			}

			m.debug("migration %d completed successfully\n", migration.Version)
		}
	}

	shouldRelease = true
	return nil
}

func (m *Migrator) Revert(ctx context.Context, to int64) (err error) {
	m.log("starting migration revert to version %d\n", to)
	m.debug("revert parameters: target=%d, sources=%d, holdLockOnFailure=%t\n", to, len(m.Sources), m.HoldLockOnFailure)

	defer func() {
		if err == nil {
			m.log("migration revert completed successfully\n")
		} else {
			m.log("migration revert failed: %v\n", err)
		}
	}()

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

	m.debug("validating target version %d exists in sources\n", to)
	if to != RevertTargetInitial {
		_, ok := slices.BinarySearchFunc(m.Sources, to, migrationCmpFunc)
		if !ok {
			m.debug("target version %d not found in migration sources\n", to)
			return fmt.Errorf("missing target version migration: %d", to)
		}
	} else {
		m.debug("target version is initial (0), will revert all migrations\n")
	}

	m.debug("initializing version store\n")
	if err := m.Store.Init(ctx); err != nil {
		return fmt.Errorf("failed to init version store: %w", err)
	}

	m.debug("acquiring version store lock\n")
	if err := m.Store.Lock(ctx); err != nil {
		return fmt.Errorf("failed to get version store lock: %w", err)
	}

	shouldRelease := true
	defer func() {
		if shouldRelease {
			m.debug("releasing version store lock\n")
			if rlErr := m.Store.Release(ctx); rlErr != nil {
				m.debug("failed to release lock: %v\n", rlErr)
				err = errors.Join(err, fmt.Errorf("failed to release version store lock: %w", rlErr))
			} else {
				m.debug("version store lock released successfully\n")
			}
		} else {
			m.debug("holding lock due to failure (HoldLockOnFailure=true)\n")
		}
	}()

	var remoteVersion int64

	m.debug("getting current version from store\n")
	remoteVersion, err = m.Store.Version(ctx)
	if err != nil {
		if errors.Is(err, ErrInitialVersion) {
			m.log("already at initial version, nothing to revert\n")
			return nil
		}
		return fmt.Errorf("failed to get version store state: %w", err)
	}
	m.log("current version: %d\n", remoteVersion)

	if m.HoldLockOnFailure {
		shouldRelease = false
		m.debug("configured to hold lock on failure\n")
	}

	revertCount := 0
	for {
		if remoteVersion <= to {
			m.debug("reached target version %d, stopping revert process\n", to)
			break
		}

		m.debug("looking for migration source for version %d\n", remoteVersion)
		idx, ok := slices.BinarySearchFunc(m.Sources, remoteVersion, migrationCmpFunc)
		if !ok {
			m.debug("migration source for version %d not found\n", remoteVersion)
			return fmt.Errorf("missing remote version migration: %d", remoteVersion)
		}

		migration := m.Sources[idx]
		m.log("reverting migration: %d\n", migration.Version)
		m.debug("executing migration %d revert function\n", migration.Version)

		if err := migration.Revert(ctx, m.Store.DB()); err != nil {
			m.debug("migration %d revert function failed: %v\n", migration.Version, err)
			return fmt.Errorf("failed to revert migration %d: %w", migration.Version, err)
		}

		m.debug("removing migration %d from version store\n", migration.Version)
		if err := m.Store.Remove(ctx, migration.Version); err != nil {
			m.debug("failed to remove migration %d from store: %v\n", migration.Version, err)
			return fmt.Errorf("failed to delete migration %d from version store: %w", migration.Version, err)
		}

		revertCount++
		m.debug("migration %d reverted successfully\n", migration.Version)

		m.debug("getting updated version from store\n")
		remoteVersion, err = m.Store.Version(ctx)
		if err != nil {
			if errors.Is(err, ErrInitialVersion) {
				m.debug("reached initial version after reverting %d migrations\n", revertCount)
				return nil
			}
			return fmt.Errorf("failed to get version store state: %w", err)
		}
		m.debug("new current version: %d\n", remoteVersion)
	}

	m.log("reverted %d migrations\n", revertCount)

	shouldRelease = true
	return nil
}
