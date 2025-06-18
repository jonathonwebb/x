package up_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jonathonwebb/x/up"
)

func TestMigration_Run(t *testing.T) {
	tests := []struct {
		name      string
		migration *up.Migration
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success",
			migration: &up.Migration{
				Version: 1,
				Name:    "add_users_table",
				RunFunc: func(ctx context.Context, db *sql.DB) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "explicit_failure",
			migration: &up.Migration{
				Version: 2,
				Name:    "add_posts_table",
				RunFunc: func(ctx context.Context, db *sql.DB) error {
					return errors.New("migration failed")
				},
			},
			wantErr: true,
			errMsg:  "migration failed",
		},
		{
			name: "nil_run_function",
			migration: &up.Migration{
				Version: 3,
				Name:    "add_comments_table",
				RunFunc: nil,
			},
			wantErr: true,
			errMsg:  "no run function",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			err := test.migration.Run(ctx, nil)

			if test.wantErr {
				if err == nil {
					t.Errorf("Migration(%d).Run() = nil, want error", test.migration.Version)
					return
				}

				if test.errMsg != "" && !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Migration(%d).Run() error = %q, want to contain %q",
						test.migration.Version, err.Error(), test.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Migration(%d).Run() = %v, want nil", test.migration.Version, err)
				}
			}
		})
	}
}

func TestMigration_Revert(t *testing.T) {
	tests := []struct {
		name      string
		migration *up.Migration
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success",
			migration: &up.Migration{
				Version: 1,
				Name:    "add_users_table",
				RevertFunc: func(ctx context.Context, db *sql.DB) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "explicit_failure",
			migration: &up.Migration{
				Version: 2,
				Name:    "add_posts_table",
				RevertFunc: func(ctx context.Context, db *sql.DB) error {
					return errors.New("rollback failed")
				},
			},
			wantErr: true,
			errMsg:  "rollback failed",
		},
		{
			name: "nil_revert_function",
			migration: &up.Migration{
				Version:    4,
				Name:       "add_categories_table",
				RevertFunc: nil,
			},
			wantErr: true,
			errMsg:  "no revert function",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			err := test.migration.Revert(ctx, nil)

			if test.wantErr {
				if err == nil {
					t.Errorf("Migration(%d).Revert() = nil, want error", test.migration.Version)
					return
				}

				if test.errMsg != "" && !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Migration(%d).Revert() error = %q, want to contain %q",
						test.migration.Version, err.Error(), test.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Migration(%d).Revert() = %v, want nil", test.migration.Version, err)
				}
			}
		})
	}
}

func TestMigration_ContextPropagation(t *testing.T) {
	type contextKey string
	key := contextKey("test")
	expectedValue := "test-value"

	t.Run("run_function", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), key, expectedValue)

		migration := &up.Migration{
			Version: 1,
			Name:    "context_test_run",
			RunFunc: func(ctx context.Context, db *sql.DB) error {
				gotValue := ctx.Value(key)
				if gotValue != expectedValue {
					t.Errorf("ctx.Value(%q) = %v, want %v", key, gotValue, expectedValue)
				}
				return nil
			},
		}

		if err := migration.Run(ctx, nil); err != nil {
			t.Errorf("Migration(%d).Run() = %v, want nil", migration.Version, err)
		}
	})

	t.Run("revert_function", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), key, expectedValue)

		migration := &up.Migration{
			Version: 1,
			Name:    "context_test_revert",
			RevertFunc: func(ctx context.Context, db *sql.DB) error {
				gotValue := ctx.Value(key)
				if gotValue != expectedValue {
					t.Errorf("ctx.Value(%q) = %v, want %v", key, gotValue, expectedValue)
				}
				return nil
			},
		}

		if err := migration.Revert(ctx, nil); err != nil {
			t.Errorf("Migration(%d).Revert() = %v, want nil", migration.Version, err)
		}
	})
}
