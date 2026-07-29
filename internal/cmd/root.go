package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"

	"github.com/jaredpalmer/mogcli/internal/authclient"
	"github.com/jaredpalmer/mogcli/internal/errfmt"
	"github.com/jaredpalmer/mogcli/internal/outfmt"
	"github.com/jaredpalmer/mogcli/internal/ui"
)

type RootFlags struct {
	Profile           string `name:"use-profile" group:"inherited-flags" help:"Profile name override for API commands" default:"${profile}"`
	Client            string `group:"inherited-flags" help:"Logical client registration name" default:"${client}"`
	EnableCommands    string `group:"inherited-flags" help:"Comma-separated list of enabled top-level commands; required in managed automation" default:"${enabled_commands}"`
	EnableActions     string `group:"inherited-flags" help:"Comma-separated list of enabled command actions; required in managed automation (for example mail.list,teams.channel-send)" default:"${enabled_actions}"`
	ManagedAutomation bool   `name:"managed-automation" group:"inherited-flags" help:"Fail closed unless command and action allowlists are non-empty (also MOG_MANAGED_AUTOMATION=true)"`
	JSON              bool   `group:"inherited-flags" help:"Output JSON to stdout (best for scripting)" default:"${json}"`
	Plain             bool   `group:"inherited-flags" help:"Output stable, parseable text to stdout (TSV)" default:"${plain}"`
	Force             bool   `group:"inherited-flags" help:"Skip confirmations for destructive commands"`
	NoInput           bool   `group:"inherited-flags" help:"Never prompt; fail instead (useful for CI)"`
	Verbose           bool   `group:"inherited-flags" help:"Enable verbose logging"`
}

type CLI struct {
	RootFlags `embed:""`

	Version kong.VersionFlag `group:"inherited-flags" help:"Print version and exit"`

	Auth       AuthCmd               `cmd:"" help:"Authenticate and manage profiles"`
	Mail       MailCmd               `cmd:"" aliases:"email" help:"Manage Outlook mail"`
	Calendar   CalendarCmd           `cmd:"" help:"Manage Outlook calendar events"`
	Contacts   ContactsCmd           `cmd:"" help:"Manage Outlook contacts"`
	Groups     GroupsCmd             `cmd:"" help:"Manage Microsoft 365 Groups (enterprise only)"`
	Teams      TeamsCmd              `cmd:"" help:"Manage Microsoft Teams"`
	Tasks      TasksCmd              `cmd:"" help:"Manage Microsoft To Do tasks"`
	OneDrive   OneDriveCmd           `cmd:"" name:"onedrive" help:"Manage OneDrive files and folders"`
	Config     ConfigCmd             `cmd:"" help:"View and manage configuration"`
	VersionCmd VersionCmd            `cmd:"" name:"version" help:"Print version"`
	Completion CompletionCmd         `cmd:"" help:"Generate shell completion scripts"`
	Complete   CompletionInternalCmd `cmd:"" name:"__complete" hidden:"" help:"Internal completion helper"`
}

type exitPanic struct{ code int }

func Execute(args []string) (err error) {
	parser, cli, err := newParser(helpDescription())
	if err != nil {
		return err
	}
	if len(args) == 0 {
		args = []string{"--help"}
	}

	// Kong exits by invoking the configured kong.Exit callback.
	// We translate that callback into a panic carrying only an exit code,
	// then recover here so Execute can return a regular Go error value.
	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		parsedErr := wrapParseError(err)
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(parsedErr))
		return parsedErr
	}

	managedMode, err := managedAutomationMode(cli.ManagedAutomation)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}

	if err = enforceEnabledCommands(kctx, cli.EnableCommands, managedMode); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}
	if err = enforceEnabledActions(kctx, cli.EnableActions, managedMode); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
		return err
	}

	logLevel := slog.LevelWarn
	if cli.Verbose {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	mode, err := outfmt.FromFlags(cli.JSON, cli.Plain)
	if err != nil {
		return newUsageError(err)
	}

	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, mode)
	ctx = authclient.WithClient(ctx, cli.Client)
	ctx = withRootFlags(ctx, &cli.RootFlags)

	u := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	ctx = ui.WithUI(ctx, u)

	kctx.BindTo(ctx, (*context.Context)(nil))
	kctx.Bind(&cli.RootFlags)

	err = kctx.Run()
	if err == nil {
		return nil
	}

	if uiPrinter := ui.FromContext(ctx); uiPrinter != nil {
		uiPrinter.Err().Error(errfmt.Format(err))
		return err
	}

	_, _ = fmt.Fprintln(os.Stderr, errfmt.Format(err))
	return err
}

func wrapParseError(err error) error {
	if err == nil {
		return nil
	}
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		return &ExitError{Code: 2, Err: parseErr}
	}
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newParser(description string) (*kong.Kong, *CLI, error) {
	envMode := outfmt.FromEnv()
	vars := kong.Vars{
		"profile":          envOr("MOG_PROFILE", ""),
		"client":           envOr("MOG_CLIENT", ""),
		"enabled_commands": envOr("MOG_ENABLE_COMMANDS", ""),
		"enabled_actions":  envOr("MOG_ENABLE_ACTIONS", ""),
		"json":             boolString(envMode.JSON),
		"plain":            boolString(envMode.Plain),
		"version":          VersionString(),
	}

	cli := &CLI{}
	parser, err := kong.New(
		cli,
		kong.Name("mog"),
		kong.Description(description),
		kong.ConfigureHelp(helpOptions()),
		kong.Help(helpPrinter),
		kong.Vars(vars),
		kong.Groups{"inherited-flags": "Inherited Flags:"},
		kong.Writers(os.Stdout, os.Stderr),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return nil, nil, err
	}

	return parser, cli, nil
}

func baseDescription() string {
	return "Microsoft Graph CLI for Outlook Mail/Calendar/Contacts/Groups/Teams/Tasks/OneDrive"
}

func helpDescription() string {
	return baseDescription()
}

func newUsageError(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: 2, Err: err}
}
