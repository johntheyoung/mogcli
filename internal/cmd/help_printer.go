package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
)

func helpOptions() kong.HelpOptions {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("MOG_HELP")))
	return kong.HelpOptions{NoExpandSubcommands: mode != "full"}
}

func helpPrinter(options kong.HelpOptions, ctx *kong.Context) error {
	origStdout := ctx.Stdout
	origStderr := ctx.Stderr

	width := guessColumns(origStdout)

	oldCols, hadCols := os.LookupEnv("COLUMNS")
	_ = os.Setenv("COLUMNS", strconv.Itoa(width))
	defer func() {
		if hadCols {
			_ = os.Setenv("COLUMNS", oldCols)
		} else {
			_ = os.Unsetenv("COLUMNS")
		}
	}()

	buf := bytes.NewBuffer(nil)
	ctx.Stdout = buf
	ctx.Stderr = origStderr
	defer func() { ctx.Stdout = origStdout }()

	if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
		return err
	}

	out := rewriteCommandSummaries(buf.String(), ctx.Selected())
	out = rewriteHelpLayout(out, ctx.Model.Name, ctx.Selected(), newHelpTheme(origStdout))
	_, err := io.WriteString(origStdout, out)
	return err
}

func rewriteCommandSummaries(out string, selected *kong.Node) string {
	if selected == nil {
		return out
	}
	prefix := selected.Path() + " "
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, prefix) && strings.HasPrefix(line, "  ") {
			indent := line[:len(line)-len(trimmed)]
			lines[i] = indent + strings.TrimPrefix(trimmed, prefix)
		}
	}
	return strings.Join(lines, "\n")
}

type helpTheme struct {
	color bool
}

func newHelpTheme(w io.Writer) helpTheme {
	file, ok := w.(*os.File)
	return helpTheme{
		color: ok && isTerminal(file) && strings.TrimSpace(os.Getenv("NO_COLOR")) == "",
	}
}

func (t helpTheme) heading(value string) string {
	if !t.color {
		return value
	}
	return "\x1b[1;36m" + value + "\x1b[0m"
}

func rewriteHelpLayout(out string, appName string, selected *kong.Node, theme helpTheme) string {
	trimmed := strings.TrimRight(out, "\n")
	if trimmed == "" {
		return out
	}

	lines := strings.Split(trimmed, "\n")
	formatted := make([]string, 0, len(lines)+16)
	for _, line := range lines {
		lineTrimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(lineTrimmed, "Usage:"):
			usage := strings.TrimSpace(strings.TrimPrefix(lineTrimmed, "Usage:"))
			formatted = appendSectionHeading(formatted, theme.heading("USAGE"))
			formatted = append(formatted, "  "+usage)
		case strings.EqualFold(lineTrimmed, "Arguments:"):
			formatted = appendSectionHeading(formatted, theme.heading("ARGUMENTS"))
		case strings.EqualFold(lineTrimmed, "Aliases:"):
			formatted = appendSectionHeading(formatted, theme.heading("ALIASES"))
		case strings.EqualFold(lineTrimmed, "Commands:"):
			formatted = appendSectionHeading(formatted, theme.heading("COMMANDS"))
		case strings.EqualFold(lineTrimmed, "Flags:"):
			formatted = appendSectionHeading(formatted, theme.heading("FLAGS"))
		case strings.EqualFold(lineTrimmed, "Inherited Flags:"):
			formatted = appendSectionHeading(formatted, theme.heading("INHERITED FLAGS"))
		case strings.HasPrefix(lineTrimmed, `Run "`) && strings.Contains(lineTrimmed, `" for more information`):
			continue
		default:
			formatted = append(formatted, line)
		}
	}

	formatted = trimTrailingBlankLines(formatted)

	selectedPath := canonicalCommandPath(selected)

	if examples := helpExamplesFor(selectedPath); len(examples) > 0 {
		formatted = appendSectionHeading(formatted, theme.heading("EXAMPLES"))
		for _, example := range examples {
			formatted = append(formatted, "  "+example)
		}
	}

	learnMore := helpLearnMoreLines(appName, selectedPath)
	if len(learnMore) > 0 {
		formatted = appendSectionHeading(formatted, theme.heading("LEARN MORE"))
		for _, line := range learnMore {
			formatted = append(formatted, "  "+line)
		}
	}

	formatted = collapseBlankLines(formatted)
	formatted = trimTrailingBlankLines(formatted)
	return strings.Join(formatted, "\n") + "\n"
}

func appendSectionHeading(lines []string, heading string) []string {
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, heading)
}

func collapseBlankLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && lastBlank {
			continue
		}
		out = append(out, line)
		lastBlank = blank
	}
	return out
}

func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func helpExamplesFor(selectedPath string) []string {
	if strings.TrimSpace(selectedPath) == "" {
		return []string{
			"$ mog auth",
			"$ mog mail list --max 10",
			"$ mog onedrive ls --path /",
		}
	}

	switch selectedPath {
	case "auth login":
		return []string{
			"$ mog auth login",
			"$ mog auth login --profile work --audience enterprise --client-id <id> --scope-workloads mail,calendar,teams",
		}
	case "auth update":
		return []string{
			"$ mog auth update",
			"$ mog auth update --profile work",
		}
	case "auth app":
		return []string{
			"$ mog auth app",
			"$ mog auth login --profile work-app --audience enterprise --mode app-only --client-id <id> --tenant <tenant> --client-secret-env MOG_CLIENT_SECRET",
		}
	case "mail list":
		return []string{
			"$ mog mail list --max 20",
			"$ mog mail list --folder inbox --max 20",
			"$ mog mail list --query \"from:alerts@example.com\"",
		}
	case "mail folders":
		return []string{
			"$ mog mail folders --max 50",
			"$ mog mail folders --include-hidden --json",
		}
	case "mail send":
		return []string{
			"$ mog mail send --to alex@example.com --subject \"Hello\" --body \"Hi there\"",
		}
	case "calendar list":
		return []string{
			"$ mog calendar list --from 2026-02-12 --to 2026-02-19",
		}
	case "onedrive ls":
		return []string{
			"$ mog onedrive ls --path /Documents --max 50",
		}
	}

	return nil
}

func helpLearnMoreLines(appName string, selectedPath string) []string {
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "mog"
	}

	commandHelp := fmt.Sprintf(`Use "%s <command> --help" for more information about a command.`, name)
	if strings.TrimSpace(selectedPath) != "" {
		commandHelp = fmt.Sprintf(`Use "%s %s --help" for more information about this command.`, name, selectedPath)
	}

	return []string{
		commandHelp,
		"Read the README at https://github.com/jaredpalmer/mogcli#readme",
	}
}

func canonicalCommandPath(selected *kong.Node) string {
	if selected == nil {
		return ""
	}

	parts := make([]string, 0, 4)
	for node := selected; node != nil; node = node.Parent {
		if node.Parent == nil {
			break
		}
		if name := strings.TrimSpace(node.Name); name != "" {
			parts = append(parts, name)
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " ")
}

func guessColumns(w io.Writer) int {
	if colsStr := os.Getenv("COLUMNS"); colsStr != "" {
		if cols, err := strconv.Atoi(colsStr); err == nil {
			return cols
		}
	}
	_ = w
	return 80
}
