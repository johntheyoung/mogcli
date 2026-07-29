package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
)

const managedAutomationEnv = "MOG_MANAGED_AUTOMATION"

func managedAutomationMode(flagEnabled bool) (bool, error) {
	raw, ok := os.LookupEnv(managedAutomationEnv)
	if !ok || strings.TrimSpace(raw) == "" {
		return flagEnabled, nil
	}

	envEnabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, usagef("%s must be a boolean (for example true or false)", managedAutomationEnv)
	}
	return flagEnabled || envEnabled, nil
}

func enforceEnabledCommands(kctx *kong.Context, enabled string, managedMode bool) error {
	enabled = strings.TrimSpace(enabled)
	if enabled == "" {
		if managedMode {
			return usage("managed automation requires a non-empty command allowlist (set MOG_ENABLE_COMMANDS or --enable-commands)")
		}
		return nil
	}
	allow := parseEnabledCommands(enabled)
	if len(allow) == 0 {
		if managedMode {
			return usage("managed automation requires a non-empty command allowlist (set MOG_ENABLE_COMMANDS or --enable-commands)")
		}
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

func enforceEnabledActions(kctx *kong.Context, enabled string, managedMode bool) error {
	enabled = strings.TrimSpace(enabled)
	if enabled == "" {
		if managedMode {
			return usage("managed automation requires a non-empty action allowlist (set MOG_ENABLE_ACTIONS or --enable-actions)")
		}
		return nil
	}
	allow := parseEnabledCommands(enabled)
	if len(allow) == 0 {
		if managedMode {
			return usage("managed automation requires a non-empty action allowlist (set MOG_ENABLE_ACTIONS or --enable-actions)")
		}
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
