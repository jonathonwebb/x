// Package conf provides CLI configuration that behaves exactly as you expect
// (assuming that you're me).
package conf

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type Env struct {
	Stderr, Stdout io.Writer
	Args           []string
	Vars           map[string]string
	Meta           map[string]any
}

func DefaultEnv(meta map[string]any) *Env {
	env := make(map[string]string)
	for _, variable := range os.Environ() {
		parts := strings.SplitN(variable, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return &Env{
		Stderr: os.Stderr,
		Stdout: os.Stdout,
		Args:   os.Args,
		Vars:   env,
		Meta:   meta,
	}
}

type ExitStatus int

const (
	ExitSuccess    ExitStatus = 0
	ExitFailure    ExitStatus = 1
	ExitUsageError ExitStatus = 2
)

var (
	ErrMissingCommand = errors.New("missing command")
	ErrUnknownCommand = errors.New("missing command")
)

type Command struct {
	Name, Desc, Usage, Help string
	Flags                   func(*flag.FlagSet, any)
	Vars                    map[string]string
	Action                  func(context.Context, *Env, any) ExitStatus
	Commands                []*Command

	flags *flag.FlagSet
}

func (c *Command) ensureFlagSet() {
	if c.flags == nil {
		c.flags = flag.NewFlagSet(c.Name, flag.ContinueOnError)
	}
}

func (c *Command) initFlagSet() {
	c.ensureFlagSet()
	c.flags.Usage = func() {}
	c.flags.SetOutput(io.Discard)
}

func (c *Command) error(env *Env, err error) ExitStatus {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(env.Stdout, "%s\n", c.Help)
		return ExitSuccess
	}
	if errors.Is(err, ErrUnknownCommand) {
		fmt.Fprintf(env.Stderr, "unknown command %s\n", env.Args[0])
	} else if errors.Is(err, ErrMissingCommand) {
		fmt.Fprintf(env.Stderr, "missing command\n")
	} else {
		fmt.Fprintf(env.Stderr, "%v\n", err)
	}
	fmt.Fprintf(env.Stderr, "%s\n", c.Usage)
	return ExitUsageError
}

type boolFlag interface {
	flag.Value
	IsBoolFlag() bool
}

func (c *Command) Execute(ctx context.Context, env *Env, target any) ExitStatus {
	c.initFlagSet()
	if c.Flags != nil {
		c.Flags(c.flags, target)
	}

	var help bool
	c.flags.BoolVar(&help, "h", false, "")
	c.flags.BoolVar(&help, "help", false, "")

	if err := c.flags.Parse(env.Args[1:]); err != nil {
		return c.error(env, err)
	}

	if help {
		return c.error(env, flag.ErrHelp)
	}

	setFlags := map[string]bool{}
	c.flags.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	var flagErr error
	c.flags.VisitAll(func(f *flag.Flag) {
		if flagErr == nil {
			if _, wasSet := setFlags[f.Name]; !wasSet {
				if varName, hasVar := c.Vars[f.Name]; hasVar {
					if val := env.Vars[varName]; val != "" {
						err := f.Value.Set(env.Vars[varName])
						if err != nil {
							if fv, ok := f.Value.(boolFlag); ok && fv.IsBoolFlag() {
								flagErr = fmt.Errorf("invalid boolean value %q for var $%s: %v", val, varName, err)
							} else {
								flagErr = fmt.Errorf("invalid value %q for var $%s: %v", val, varName, err)
							}
						}
					}

				}
			}
		}
	})

	if flagErr != nil {
		return c.error(env, flagErr)
	}

	env.Args = c.flags.Args()

	if c.Action != nil {
		return c.Action(ctx, env, target)
	}
	if len(env.Args) == 0 {
		return c.error(env, ErrMissingCommand)
	}
	for _, cmd := range c.Commands {
		if cmd.Name == env.Args[0] {
			return cmd.Execute(ctx, env, target)
		}
	}
	return c.error(env, ErrUnknownCommand)
}
