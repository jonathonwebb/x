// Package cli provides a simple command-line interface framework.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// An Env represents the execution environment for a [Command].
//
// M is the type of custom metadata that will be available to Command actions.
type Env[M any] struct {
	Err  io.Writer         // error output stream
	Out  io.Writer         // standard output stream
	Args []string          // command-line arguments
	Vars map[string]string // environment variables
	Meta M                 // custom metadata
}

// Printf formats and writes a message to the standard output stream.
func (e Env[M]) Printf(format string, args ...any) {
	if e.Out != nil {
		fmt.Fprintf(e.Out, format, args...)
	}
}

// Errorf formats and writes an error message to the error output stream.
func (e Env[M]) Errorf(format string, args ...any) {
	if e.Err != nil {
		fmt.Fprintf(e.Err, format, args...)
	}
}

func (e Env[M]) hasVar(name string) bool {
	if e.Vars == nil {
		return false
	}
	_, ok := e.Vars[name]
	return ok
}

func (e Env[M]) getVar(name string) string {
	if e.Vars == nil {
		return ""
	}
	return e.Vars[name]
}

// DefaultEnv returns an [Env] using the current process's environment.
//
// The returned Env will use the [os.Stderr] and [os.Stdout] streams, [os.Args],
// and environment variables from [os.Environ].
func DefaultEnv[M any](meta M) Env[M] {
	vars := make(map[string]string)
	for _, v := range os.Environ() {
		key, value, _ := strings.Cut(v, "=")
		vars[key] = value
	}
	return Env[M]{
		Err:  os.Stderr,
		Out:  os.Stdout,
		Args: os.Args,
		Vars: vars,
		Meta: meta,
	}
}

// An ExitStatus is the result of command execution.
type ExitStatus int

const (
	ExitSuccess ExitStatus = 0 // execution succeeded
	ExitFailure ExitStatus = 1 // execution failed due to an error
	ExitUsage   ExitStatus = 2 // execution failed due to invalid user input
)

var (
	errMissingCommand = errors.New("missing command")
	errUnknownCommand = errors.New("unknown command")
)

// A Command represents a CLI command.
//
// T is the type of the target value for configuration storage.
// M is the type of the metadata provided by the execution [Env].
type Command[T any, M any] struct {
	Name        string                                                     // name used to invoke the command.
	Usage       string                                                     // short usage text
	Help        string                                                     // long help text
	Flags       func(flags *flag.FlagSet, target T)                        // function for defining flags
	Vars        map[string]string                                          // map of flag names -> environment variables
	Action      func(ctx context.Context, env Env[M], target T) ExitStatus // command action
	Subcommands []Command[T, M]                                            // command subcommands

	fs *flag.FlagSet
}

func (c *Command[T, M]) flagSet() *flag.FlagSet {
	if c.fs == nil {
		c.fs = flag.NewFlagSet(c.Name, flag.ContinueOnError)
		c.fs.Usage = func() { /* no-op */ }
		c.fs.SetOutput(io.Discard)
	}
	return c.fs
}

func (c *Command[T, M]) getFlagVar(flagName string) (string, bool) {
	if c.Vars == nil {
		return "", false
	}
	varName, ok := c.Vars[flagName]
	return varName, ok
}

func (c *Command[T, M]) findSubcommand(name string) *Command[T, M] {
	if c.Subcommands == nil {
		return nil
	}
	for i := range c.Subcommands {
		if c.Subcommands[i].Name == name {
			return &c.Subcommands[i]
		}
	}
	return nil
}

type boolFlag interface {
	flag.Value
	IsBoolFlag() bool
}

// Execute parses command-line arguments from the environment, then either calls
// the command's action or defers to the specified subcommand's Execute method.
func (c *Command[T, M]) Execute(ctx context.Context, env Env[M], target T) ExitStatus {
	if c.Flags != nil {
		c.Flags(c.flagSet(), target)
	}

	if len(env.Args) < 1 {
		env.Errorf("command requires at least one arg\n")
		return ExitFailure
	}

	if err := c.flagSet().Parse(env.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			env.Printf("%s\n\n%s\n", c.Usage, c.Help)
			return ExitSuccess
		}
		env.Errorf("%s\n%v\n", c.Usage, err)
		return ExitUsage
	}

	setByUser := make(map[string]bool)
	c.flagSet().Visit(func(f *flag.Flag) {
		setByUser[f.Name] = true
	})

	var flagErr error
	c.flagSet().VisitAll(func(f *flag.Flag) {
		if flagErr != nil {
			return
		}

		if _, wasSet := setByUser[f.Name]; wasSet {
			return
		}

		varName, hasVar := c.getFlagVar(f.Name)
		if !hasVar {
			return
		}

		if !env.hasVar(varName) {
			return
		}

		value := env.getVar(varName)
		if err := c.flagSet().Set(f.Name, value); err != nil {
			if fv, ok := f.Value.(boolFlag); ok && fv.IsBoolFlag() {
				flagErr = fmt.Errorf("invalid boolean value %q for var $%s: %v", value, varName, err)
			} else {
				flagErr = fmt.Errorf("invalid value %q for var $%s: %v", value, varName, err)
			}
		}
	})
	if flagErr != nil {
		env.Errorf("%s\n%v\n", c.Usage, flagErr)
		return ExitUsage
	}

	env.Args = c.flagSet().Args()

	if c.Action != nil {
		return c.Action(ctx, env, target)
	}

	if len(env.Args) == 0 {
		env.Errorf("%s\n%v\n", c.Usage, errMissingCommand)
		return ExitUsage
	}

	subCmd := c.findSubcommand(env.Args[0])
	if subCmd != nil {
		return subCmd.Execute(ctx, env, target)
	}

	env.Errorf("%s\n%v\n", c.Usage, errUnknownCommand)
	return ExitUsage
}
