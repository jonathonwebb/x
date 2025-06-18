package up_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonathonwebb/x/up"
)

func TestFuncLoader_Load(t *testing.T) {
	t.Run("empty loader", func(t *testing.T) {
		loader := up.NewFuncLoader()
		migrations, err := loader.Load(context.Background())

		if err != nil {
			t.Errorf("got %v, wanted no error", err)
		}

		var want []*up.Migration
		if diff := cmp.Diff(want, migrations); diff != "" {
			t.Errorf("migrations mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("loader with migrations", func(t *testing.T) {
		migration1 := &up.Migration{Version: 1, Name: "First migration"}
		migration2 := &up.Migration{Version: 2, Name: "Second migration"}

		loader := up.NewFuncLoader(migration1, migration2)
		migrations, err := loader.Load(context.Background())

		if err != nil {
			t.Errorf("got %v, wanted no error", err)
		}

		want := []*up.Migration{migration1, migration2}
		if diff := cmp.Diff(want, migrations); diff != "" {
			t.Errorf("migrations mismatch (-want +got):\n%s", diff)
		}
	})
}
