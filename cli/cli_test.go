package cli_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jonathonwebb/x/cli"
)

func ExampleCommand() {
	type meta struct {
		build string
	}

	type values struct {
		env  string
		addr string
	}

	serveCmd := cli.Command[*values, meta]{
		Name:  "serve",
		Usage: "serve [flags]",
		Help:  "flags:\n  -addr",
		SetFlags: func(flags *flag.FlagSet, target *values) {
			flags.StringVar(&target.addr, "addr", "", "")
		},
		Vars: map[string]string{
			"addr": "FOO_ADDR",
		},
		Action: func(ctx context.Context, env cli.Env[meta], target *values) cli.ExitStatus {
			fmt.Printf("env=%s\n", target.env)
			fmt.Printf("addr=%s\n", target.addr)
			return cli.ExitSuccess
		},
	}

	rootCmd := cli.Command[*values, meta]{
		Name:  "foo",
		Usage: "usage: foo [flags] command",
		Help:  "commands:\n  serve\n\nflags:\n  -env",
		SetFlags: func(flags *flag.FlagSet, target *values) {
			flags.StringVar(&target.env, "env", "", "")
		},
		Subcommands: []cli.Command[*values, meta]{serveCmd},
	}

	v := values{}
	status := rootCmd.Execute(context.Background(), cli.Env[meta]{
		Args: []string{"foo", "-env=dev", "serve"},
		Vars: map[string]string{
			"FOO_ADDR": "localhost:8000",
		},
	}, &v)

	fmt.Printf("status=%d", status)

	// Output: env=dev
	// addr=localhost:8000
	// status=0
}

type testCommandResults[T any] struct {
	outbuf string
	errbuf string
	status cli.ExitStatus
	target T
}

type testCommandOptions[T any, M any] struct {
	args   []string
	vars   map[string]string
	target T
	meta   M
}

func executeTestCommand[T any, M any](ctx context.Context, cmd *cli.Command[T, M], opts testCommandOptions[T, M]) testCommandResults[T] {
	var outbuf, errbuf bytes.Buffer
	env := cli.Env[M]{
		Err:  &errbuf,
		Out:  &outbuf,
		Args: opts.args,
		Vars: opts.vars,
		Meta: opts.meta,
	}
	status := cmd.Execute(ctx, env, opts.target)
	return testCommandResults[T]{
		outbuf: outbuf.String(),
		errbuf: errbuf.String(),
		status: status,
		target: opts.target,
	}
}

func TestDefaultEnv(t *testing.T) {
	t.Setenv("TEST_ENV_VAR", "test_value")

	type testMeta struct {
		version string
	}
	meta := testMeta{version: "1.0.0"}

	env := cli.DefaultEnv(meta)

	if env.Err != os.Stderr {
		t.Errorf("env.Err = %v, want %v", env.Err, os.Stderr)
	}

	if env.Out != os.Stdout {
		t.Errorf("env.Out = %v, want %v", env.Out, os.Stdout)
	}

	if env.Args == nil {
		t.Errorf("env.Args is nil, want non-nil")
	}

	value, exists := env.Vars["TEST_ENV_VAR"]
	if !exists {
		t.Errorf("env.Vars[\"TEST_ENV_VAR\"] does not exist")
	}

	if value != "test_value" {
		t.Errorf("env.Vars[\"TEST_ENV_VAR\"] = %q, want %q", value, "test_value")
	}

	if env.Meta != meta {
		t.Errorf("env.Meta = %v, want %v", env.Meta, meta)
	}
}

func TestEnv_Printf(t *testing.T) {
	t.Run("with output writer", func(t *testing.T) {
		var buf bytes.Buffer
		env := cli.Env[any]{
			Out: &buf,
		}

		env.Printf("test %s", "message")

		want := "test message"
		if got := buf.String(); got != want {
			t.Errorf("Printf() wrote %q, want %q", got, want)
		}
	})

	t.Run("without output writer", func(t *testing.T) {
		env := cli.Env[any]{
			Out: nil,
		}

		// don't panic!
		env.Printf("test %s", "message")
	})
}

func TestEnv_Errorf(t *testing.T) {
	t.Run("with error writer", func(t *testing.T) {
		var buf bytes.Buffer
		env := cli.Env[any]{
			Err: &buf,
		}

		env.Errorf("error %s", "message")

		want := "error message"
		if got := buf.String(); got != want {
			t.Errorf("Errorf() wrote %q, want %q", got, want)
		}
	})

	t.Run("without error writer", func(t *testing.T) {
		env := cli.Env[any]{
			Err: nil,
		}

		// don't panic!
		env.Errorf("error %s", "message")
	})
}

func TestCommand_Execute(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmd := &cli.Command[any, any]{
			Name: "foo",
			Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[any, any]{
			args: []string{"foo"},
		})

		wantStatus := cli.ExitSuccess
		if res.status != wantStatus {
			t.Errorf("res.status=%v, want %v", res.status, wantStatus)
		}
	})

	t.Run("nil args", func(t *testing.T) {
		cmd := &cli.Command[any, any]{
			Name: "foo",
			Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[any, any]{
			args: nil,
		})

		wantStatus := cli.ExitFailure
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}
	})

	t.Run("no args", func(t *testing.T) {
		cmd := &cli.Command[any, any]{
			Name: "foo",
			Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[any, any]{
			args: []string{},
		})

		wantStatus := cli.ExitFailure
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}
	})

	t.Run("nil vars", func(t *testing.T) {
		type values struct {
			bar string
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.StringVar(&v.bar, "bar", "default", "")
			},
			Vars: map[string]string{
				"bar": "FOO_BAR",
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo"},
			vars: nil,
		})

		wantStatus := cli.ExitSuccess
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		wantValue := "default"
		if gotValue := v.bar; gotValue != wantValue {
			t.Errorf("gotValue=%v, want %v", gotValue, wantValue)
		}
	})

	t.Run("-h", func(t *testing.T) {
		cmd := &cli.Command[any, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[any, any]{
			args: []string{"foo", "-h"},
		})

		wantStatus := cli.ExitSuccess
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v,want %v", gotStatus, wantStatus)
		}

		wantOut := "usage: foo [flags]\n\ndoes foo and then bar\n"
		gotOut := res.outbuf
		if diff := cmp.Diff(wantOut, gotOut); diff != "" {
			t.Errorf("output mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("-help", func(t *testing.T) {
		cmd := &cli.Command[any, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[any, any]{
			args: []string{"foo", "-help"},
		})

		wantStatus := cli.ExitSuccess
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		wantOut := "usage: foo [flags]\n\ndoes foo and then bar\n"
		gotOut := res.outbuf
		if diff := cmp.Diff(wantOut, gotOut); diff != "" {
			t.Errorf("output mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("valid flag", func(t *testing.T) {
		type values struct {
			bar  string
			quux bool
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.StringVar(&v.bar, "bar", "", "")
				flags.BoolVar(&v.quux, "quux", false, "")
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo", "-bar=baz"},
		})

		wantStatus := cli.ExitSuccess
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		wantValue := "baz"
		if gotValue := v.bar; gotValue != wantValue {
			t.Errorf("gotValue=%v, want %v", gotValue, wantValue)
		}
	})

	t.Run("invalid flag", func(t *testing.T) {
		type values struct {
			bar  string
			quux bool
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.StringVar(&v.bar, "bar", "", "")
				flags.BoolVar(&v.quux, "quux", false, "")
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo", "-invalid_arg"},
		})

		wantStatus := cli.ExitUsage
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}
	})

	t.Run("valid env var", func(t *testing.T) {
		type values struct {
			bar  string
			quux bool
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.StringVar(&v.bar, "bar", "", "")
				flags.BoolVar(&v.quux, "quux", false, "")
			},
			Vars: map[string]string{
				"bar":  "FOO_BAR",
				"quux": "FOO_QUUX",
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo"},
			vars: map[string]string{
				"FOO_BAR": "baz",
			},
		})

		wantStatus := cli.ExitSuccess
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		wantValue := "baz"
		if gotValue := v.bar; gotValue != wantValue {
			t.Errorf("gotValue=%v, want %v", gotValue, wantValue)
		}
	})

	t.Run("invalid env var", func(t *testing.T) {
		type values struct {
			bar  uint
			quux bool
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.UintVar(&v.bar, "bar", 0, "")
				flags.BoolVar(&v.quux, "quux", false, "")
			},
			Vars: map[string]string{
				"bar":  "FOO_BAR",
				"quux": "FOO_QUUX",
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo"},
			vars: map[string]string{
				"FOO_BAR": "baz",
			},
		})

		wantStatus := cli.ExitUsage
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}
	})

	t.Run("invalid bool env var", func(t *testing.T) {
		type values struct {
			bar  uint
			quux bool
		}
		var v values

		cmd := &cli.Command[*values, any]{
			Name:  "foo",
			Usage: "usage: foo [flags]",
			Help:  "does foo and then bar",
			SetFlags: func(flags *flag.FlagSet, target *values) {
				flags.UintVar(&v.bar, "bar", 0, "")
				flags.BoolVar(&v.quux, "quux", false, "")
			},
			Vars: map[string]string{
				"bar":  "FOO_BAR",
				"quux": "FOO_QUUX",
			},
			Action: func(ctx context.Context, env cli.Env[any], target *values) cli.ExitStatus {
				return cli.ExitSuccess
			},
		}
		res := executeTestCommand(t.Context(), cmd, testCommandOptions[*values, any]{
			args: []string{"foo"},
			vars: map[string]string{
				"FOO_QUUX": "baz",
			},
		})

		wantStatus := cli.ExitUsage
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}
	})

	t.Run("missing command", func(t *testing.T) {
		rootCmd := &cli.Command[any, any]{
			Name:  "foo",
			Usage: "usage: foo [flags] command",
			Help:  "commands:\n  bar\n  baz",
			Subcommands: []cli.Command[any, any]{
				{
					Name:  "bar",
					Usage: "usage: foo bar",
					Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
						return cli.ExitSuccess
					},
				},
				{
					Name:  "baz",
					Usage: "usage: foo baz",
					Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
						return cli.ExitSuccess
					},
				},
			},
		}

		res := executeTestCommand(t.Context(), rootCmd, testCommandOptions[any, any]{
			args: []string{"foo"},
		})

		wantStatus := cli.ExitUsage
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		if !strings.Contains(res.errbuf, "missing command") {
			t.Errorf("error message %q does not contain 'missing command'", res.errbuf)
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		rootCmd := &cli.Command[any, any]{
			Name:  "foo",
			Usage: "usage: foo [flags] command",
			Help:  "commands:\n  bar\n  baz",
			Subcommands: []cli.Command[any, any]{
				{
					Name:  "bar",
					Usage: "usage: foo bar",
					Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
						return cli.ExitSuccess
					},
				},
				{
					Name:  "baz",
					Usage: "usage: foo baz",
					Action: func(ctx context.Context, env cli.Env[any], target any) cli.ExitStatus {
						return cli.ExitSuccess
					},
				},
			},
		}

		res := executeTestCommand(t.Context(), rootCmd, testCommandOptions[any, any]{
			args: []string{"foo", "unknown"},
		})

		wantStatus := cli.ExitUsage
		if gotStatus := res.status; gotStatus != wantStatus {
			t.Errorf("gotStatus=%v, want %v", gotStatus, wantStatus)
		}

		if !strings.Contains(res.errbuf, "unknown command") {
			t.Errorf("error message %q does not contain 'unknown command'", res.errbuf)
		}
	})
}
