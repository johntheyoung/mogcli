package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

func enforceEnabledCommands(kctx *kong.Context, enabled string) error {
	enabled = strings.TrimSpace(enabled)
	if enabled == "" {
		return nil
	}
	allow := parseEnabledCommands(enabled)
	if len(allow) == 0 {
		return nil
	}
	if allow["*"] || allow["all"] {
		return nil
	}
	cmd := strings.Fields(kctx.Command())
	if len(cmd) == 0 {
		return nil
	}
	top := strings.ToLower(cmd[0])
	if !allow[top] {
		return usagef("command %q is not enabled (set --enable-commands to allow it)", top)
	}
	return nil
}

func enforceEnabledActions(kctx *kong.Context, enabled string) error {
	enabled = strings.TrimSpace(enabled)
	if enabled == "" {
		return nil
	}
	allow := parseEnabledCommands(enabled)
	if len(allow) == 0 {
		return nil
	}
	if allow["*"] || allow["all"] {
		return nil
	}
	action := commandAction(kctx)
	if action == "" {
		return nil
	}
	if !allow[action] {
		return usagef("action %q is not enabled (set --enable-actions to allow it)", action)
	}
	return nil
}

func parseEnabledCommands(value string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		out[part] = true
	}
	return out
}

func commandAction(kctx *kong.Context) string {
	if kctx == nil {
		return ""
	}
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(kctx.Command())))
	if len(parts) == 0 {
		return ""
	}
	actionParts := make([]string, 0, len(parts))
	for _, part := range parts {
		// Kong represents positional resource values as schema placeholders
		// such as <id>. They identify the target of an action, not a distinct
		// capability, so keep only named commands in the canonical action.
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			continue
		}
		actionParts = append(actionParts, part)
	}
	return strings.Join(actionParts, ".")
}
