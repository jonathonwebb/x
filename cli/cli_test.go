package cli_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jonathonwebb/x/cli"
)

func TestEnv_Printf(t *testing.T) {
	t.Run("with_writer", func(t *testing.T) {
		var buf bytes.Buffer
		env := cli.Env[any]{Out: &buf}
		env.Printf("hello %s", "world")

		want := "hello world"
		if got := buf.String(); got != want {
			t.Errorf("env.Printf(%q, %q) wrote %q, want %q", "hello %s", "world", got, want)
		}
	})

	t.Run("nil_writer", func(t *testing.T) {
		env := cli.Env[any]{Out: nil}
		env.Printf("hello %s", "world") // don't panic!
	})
}

func TestEnv_Errorf(t *testing.T) {
	t.Run("with_writer", func(t *testing.T) {
		var buf bytes.Buffer
		env := cli.Env[any]{Err: &buf}
		env.Errorf("hello %s", "world")

		want := "hello world"
		if got := buf.String(); got != want {
			t.Errorf("env.Errorf(%q, %q) wrote %q, want %q", "hello %s", "world", got, want)
		}
	})

	t.Run("nil_writer", func(t *testing.T) {
		env := cli.Env[any]{Err: nil}
		env.Errorf("hello %s", "world") // don't panic!
	})
}

func TestDefaultEnv(t *testing.T) {
	const testEnvVar = "TEST_ENV_VAR"
	const testEnvValue = "test_value"

	t.Setenv(testEnvVar, testEnvValue)

	type testMeta struct {
		version string
	}
	meta := testMeta{version: "x.x.x"}

	env := cli.DefaultEnv(meta)

	if got, want := env.Err, os.Stderr; got != want {
		t.Errorf("DefaultEnv(%+v).Err = %v, want %v", meta, got, want)
	}
	if got, want := env.Out, os.Stdout; got != want {
		t.Errorf("DefaultEnv(%+v).Out = %v, want %v", meta, got, want)
	}
	if env.Args == nil {
		t.Errorf("DefaultEnv(%+v).Args = %v, want non-nil", meta, env.Args)
	}

	value, exists := env.Vars[testEnvVar]
	if !exists {
		t.Errorf("DefaultEnv(%+v).Vars[%q] does not exist", meta, testEnvVar)
	}
	if got, want := value, testEnvValue; got != want {
		t.Errorf("DefaultEnv(%+v).Vars[%q] = %v, want %v", meta, testEnvVar, got, want)
	}

	if got, want := env.Meta, meta; got != want {
		t.Errorf("DefaultEnv(%+v).Meta = %v, want %v", meta, got, want)
	}
}

type testMeta struct {
	version string
}

type testTarget struct {
	env     string
	verbose bool
	port    uint
}

const (
	fooName  = "foo"
	fooUsage = "foo usage"
	fooHelp  = "foo help"
	fooOut   = "foo cmd\n"

	barName  = "bar"
	barUsage = "bar usage"
	barHelp  = "bar help"
	barOut   = "bar cmd\n"
)

type testCommandResults struct {
	env    *cli.Env[testMeta]
	outbuf string
	errbuf string
	status cli.ExitStatus
	target *testTarget
}

type testCommandOptions struct {
	args []string
	vars map[string]string
	meta testMeta
}

func executeTestCommand(t *testing.T, cmd *cli.Command[*testTarget, testMeta], opts testCommandOptions) *testCommandResults {
	t.Helper()
	var outbuf, errbuf bytes.Buffer
	env := &cli.Env[testMeta]{
		Err:  &errbuf,
		Out:  &outbuf,
		Args: opts.args,
		Vars: opts.vars,
		Meta: opts.meta,
	}
	target := &testTarget{}
	status := cmd.Execute(context.Background(), env, target)
	return &testCommandResults{
		env:    env,
		outbuf: outbuf.String(),
		errbuf: errbuf.String(),
		status: status,
		target: target,
	}
}

func testCommand(t *testing.T) *cli.Command[*testTarget, testMeta] {
	t.Helper()
	return &cli.Command[*testTarget, testMeta]{
		Name:  fooName,
		Usage: fooUsage,
		Help:  fooHelp,
		Flags: func(flags *flag.FlagSet, target *testTarget) {
			flags.StringVar(&target.env, "env", "prod", "")
			flags.BoolVar(&target.verbose, "verbose", false, "")
		},
		Vars: map[string]string{
			"env":     "FOO_ENV",
			"verbose": "FOO_VERBOSE",
		},
		Action: func(ctx context.Context, env *cli.Env[testMeta], target *testTarget) cli.ExitStatus {
			env.Printf(fooOut)
			return cli.ExitSuccess
		},
		Subcommands: []*cli.Command[*testTarget, testMeta]{
			{
				Name:  barName,
				Usage: barUsage,
				Help:  barHelp,
				Flags: func(flags *flag.FlagSet, target *testTarget) {
					flags.UintVar(&target.port, "port", 0, "")
				},
				Vars: map[string]string{
					"port": "BAR_PORT",
				},
				Action: func(ctx context.Context, env *cli.Env[testMeta], target *testTarget) cli.ExitStatus {
					env.Printf(barOut)
					return cli.ExitSuccess
				},
			},
		},
	}
}

func TestCommand_Execute(t *testing.T) {
	t.Run("nil_args", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: nil}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("with nil args: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with nil args: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, "no arguments"; !strings.Contains(got, want) {
			t.Errorf("with nil args: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("no_args", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("with no args: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with no args: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, "no arguments"; !strings.Contains(got, want) {
			t.Errorf("with no args: cmd.Execute() wrote error=%q, want %q", got, want)
		}
	})

	t.Run("nil_vars", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with nil vars: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, fooOut; got != want {
			t.Errorf("with nil vars: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with nil vars: cmd.Execute() wrote error=%q, want %q", got, want)
		}
	})

	t.Run("-h", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "-h"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with -h: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, "foo usage\n\nfoo help\n"; got != want {
			t.Errorf("with -h: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with -h: cmd.Execute() wrote error=%q, want %q", got, want)
		}
	})

	t.Run("-help", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "-help"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with -help: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, "foo usage\n\nfoo help\n"; got != want {
			t.Errorf("with -help: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with -help: cmd.Execute() wrote error=%q, want %q", got, want)
		}
	})

	t.Run("valid_flag", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "-env=dev"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with -env=dev: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, fooOut; got != want {
			t.Errorf("with -env=dev: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with -env=dev: cmd.Execute() wrote error=%q, want %q", got, want)
		}
		if got, want := res.target.env, "dev"; got != want {
			t.Errorf("with -env=dev: cmd.Execute() target env=%v, want %v", got, want)
		}
	})

	t.Run("invalid_flag", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "-invalid"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitUsage; got != want {
			t.Errorf("with -invalid: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with -invalid: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, fooUsage; !strings.Contains(got, want) {
			t.Errorf("with -invalid: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
		if got, want := res.errbuf, "flag provided but not defined: -invalid"; !strings.Contains(got, want) {
			t.Errorf("with -invalid: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("valid_var", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo"}, vars: map[string]string{"FOO_ENV": "dev"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with FOO_ENV=dev: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, fooOut; got != want {
			t.Errorf("with FOO_ENV=dev: cmd.Execute() output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with FOO_ENV=dev: cmd.Execute() wrote error=%q, want %q", got, want)
		}
		if got, want := res.target.env, "dev"; got != want {
			t.Errorf("with FOO_ENV=dev: cmd.Execute() target env=%v, want %v", got, want)
		}
	})

	t.Run("invalid_var", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "bar"}, vars: map[string]string{"BAR_PORT": "-1"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitUsage; got != want {
			t.Errorf("with BAR_PORT=-1: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with BAR_PORT=-1: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, "invalid value \"-1\" for var $BAR_PORT"; !strings.Contains(got, want) {
			t.Errorf("with BAR_PORT=-1: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("invalid_bool_var", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo"}, vars: map[string]string{"FOO_VERBOSE": "invalid"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitUsage; got != want {
			t.Errorf("with FOO_VERBOSE=invalid: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with FOO_VERBOSE=invalid: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, "invalid boolean value \"invalid\" for var $FOO_VERBOSE"; !strings.Contains(got, want) {
			t.Errorf("with FOO_VERBOSE=invalid: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("positionals", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Subcommands = nil
		opts := testCommandOptions{args: []string{"foo", "baz"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with positionals: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := len(res.env.Args), 1; got != want {
			t.Errorf("with positionals: cmd.Execute() remaining args=%v, want %v", got, want)
		}
	})

	t.Run("subcommand", func(t *testing.T) {
		cmd := testCommand(t)
		opts := testCommandOptions{args: []string{"foo", "bar"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitSuccess; got != want {
			t.Errorf("with subcommand: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, barOut; got != want {
			t.Errorf("with subcommand: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, ""; got != want {
			t.Errorf("with subcommand: cmd.Execute() wrote error=%q, want %q", got, want)
		}
	})

	t.Run("missing_subcommand", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Action = nil // Remove action so it requires a subcommand
		opts := testCommandOptions{args: []string{"foo"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitUsage; got != want {
			t.Errorf("with missing subcommand: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with missing subcommand: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, "missing command"; !strings.Contains(got, want) {
			t.Errorf("with missing subcommand: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
		if got, want := res.errbuf, fooUsage; !strings.Contains(got, want) {
			t.Errorf("with missing subcommand: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("unknown_subcommand", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Action = nil
		opts := testCommandOptions{args: []string{"foo", "invalid"}}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitUsage; got != want {
			t.Errorf("with unknown subcommand: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.outbuf, ""; got != want {
			t.Errorf("with unknown subcommand: cmd.Execute() wrote output=%q, want %q", got, want)
		}
		if got, want := res.errbuf, fooUsage; !strings.Contains(got, want) {
			t.Errorf("with unknown subcommand: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
		if got, want := res.errbuf, "unknown command"; !strings.Contains(got, want) {
			t.Errorf("with unknown subcommand: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})
}

func TestCommand_Execute_TemplateFailures(t *testing.T) {
	t.Run("usage_template_parse_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Usage = "{{.InvalidSyntax" // Invalid template syntax
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("usage template parse failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing usage template"; !strings.Contains(got, want) {
			t.Errorf("usage template parse failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("usage_template_exec_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Usage = "{{.NonExistentField}}" // Field doesn't exist in testMeta
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("usage template exec failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing usage template"; !strings.Contains(got, want) {
			t.Errorf("usage template exec failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("help_template_parse_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Help = "{{.InvalidSyntax" // Invalid template syntax
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("help template parse failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing help template"; !strings.Contains(got, want) {
			t.Errorf("help template parse failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("help_template_exec_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Help = "{{.NonExistentField}}" // Field doesn't exist in testMeta
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("help template exec failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing help template"; !strings.Contains(got, want) {
			t.Errorf("help template exec failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("vars_template_parse_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Vars["env"] = "{{.InvalidSyntax" // Invalid template syntax
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("vars template parse failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing template for var env"; !strings.Contains(got, want) {
			t.Errorf("vars template parse failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})

	t.Run("vars_template_exec_failure", func(t *testing.T) {
		cmd := testCommand(t)
		cmd.Vars["env"] = "{{.NonExistentField}}" // Field doesn't exist in testMeta
		opts := testCommandOptions{
			args: []string{"foo"},
			meta: testMeta{version: "1.0.0"},
		}
		res := executeTestCommand(t, cmd, opts)

		if got, want := res.status, cli.ExitFailure; got != want {
			t.Errorf("vars template exec failure: cmd.Execute()=%v, want %v", got, want)
		}
		if got, want := res.errbuf, "error executing template for var env"; !strings.Contains(got, want) {
			t.Errorf("vars template exec failure: cmd.Execute() wrote error=%q, want contains %q", got, want)
		}
	})
}

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
		Flags: func(flags *flag.FlagSet, target *values) {
			flags.StringVar(&target.addr, "addr", "", "")
		},
		Vars: map[string]string{
			"addr": "FOO_ADDR",
		},
		Action: func(ctx context.Context, env *cli.Env[meta], target *values) cli.ExitStatus {
			fmt.Printf("env=%s\n", target.env)
			fmt.Printf("addr=%s\n", target.addr)
			return cli.ExitSuccess
		},
	}

	rootCmd := cli.Command[*values, meta]{
		Name:  "foo",
		Usage: "usage: foo [flags] command",
		Help:  "commands:\n  serve\n\nflags:\n  -env",
		Flags: func(flags *flag.FlagSet, target *values) {
			flags.StringVar(&target.env, "env", "", "")
		},
		Subcommands: []*cli.Command[*values, meta]{&serveCmd},
	}

	v := values{}
	status := rootCmd.Execute(context.Background(), &cli.Env[meta]{
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
