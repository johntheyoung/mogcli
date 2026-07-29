---
name: mog
description: "mog CLI: safe Microsoft 365 automation via Microsoft Graph, JSON, auth, scoped reads/writes, and Teams chat/DM actions. Use when working with mogcli/mog, Outlook mail, calendar, contacts, Groups, Teams, To Do, OneDrive, Microsoft Graph auth, Hermes/OpenClaw shell automation, or agent-safe Microsoft 365 command allowlists."
---

# mog

Use `mog` when built-in Microsoft connectors are missing a feature, when shell automation needs stable JSON, or when you need to inspect local Microsoft Graph auth state before acting.

## Fast Path

```bash
command -v mog
mog --version
mog auth status
mog auth whoami 2>/dev/null || true
mog teams --help
```

`mogcli` is the repo/project name. The installed executable is usually `mog`. Do not conclude the setup is absent solely because `mogcli` is not on `PATH`.

Prefer `--json` for agent parsing. Human hints and progress should stay on stderr; stdout is for data.

For Dexterton Hermes, load the profile environment before testing outside the running gateway:

```bash
set -a
. ~/.hermes/profiles/dexterton/.env
set +a
mog auth status
```

## Safety Rules

- Do not print access tokens, refresh tokens, OAuth client secrets, tenant secrets, or keyring contents.
- Use the intended profile explicitly for API work: `MOG_PROFILE=dexterton` or `mog --use-profile dexterton ...`.
- Use `--json` for reads and dry runs that an agent will parse.
- Use `--dry-run` first where commands support it.
- Use `--enable-commands` and `--enable-actions` to narrow what an agent can do.
- For managed/headless execution, set `MOG_MANAGED_AUTOMATION=true`; it fails closed unless both `MOG_ENABLE_COMMANDS` and `MOG_ENABLE_ACTIONS` are non-empty. This mode is explicit and is not inferred from TTY or `CI` state.
- Do not send mail, create/update/delete calendar events, post Teams messages, or mutate contacts/files unless the user asked for that exact action.
- For ambiguous write requests, preview the target, body, recipients, and action first.
- Do not add `--force` unless the user explicitly asked for the destructive mutation.
- In headless/service agents, verify the service environment, not just the login shell. The running Hermes/OpenClaw process must see the same `MOG_*` values as the shell test.

Runtime command guards:

```bash
MOG_PROFILE=dexterton \
MOG_ENABLE_COMMANDS=mail,calendar,teams \
MOG_ENABLE_ACTIONS=mail.list,mail.get,calendar.list,calendar.get,teams.list,teams.channels,teams.chats \
mog teams chats --max 1 --json
```

For Dexterton Hermes, the current write allowlist may include:

```text
teams.channel-send,teams.chat-send,teams.dm-send
```

Treat those as capabilities, not permission to send without an explicit user request.

## Auth

OAuth setup is partly interactive. An agent can inspect and diagnose it, but a human normally completes browser/device consent:

```bash
mog auth status
mog auth login --profile work --audience enterprise --client-id <id> --tenant <tenant-id-or-domain> --scope-workloads mail,calendar,teams
mog auth use <profile>
mog auth logout --profile <profile>
```

Default for existing human/user OAuth reauth: preserve the existing workload intent. Before changing scopes, inspect the active profile and current task. Use command/action guards for agent safety instead of silently under-scoping durable user auth.

For Microsoft Teams chat/DM support, the enterprise delegated app registration needs these Graph delegated scopes in addition to existing Teams channel scopes:

```text
Chat.ReadBasic
Chat.Create
ChatMessage.Send
```

For Hermes/systemd setups, run diagnostics through the actual profile entrypoint after restarting the service:

```bash
dexterton --accept-hooks gateway restart
dexterton gateway status
dexterton send 'Run: mog auth status && mog teams chats --max 1 --json'
```

If a command works in the shell but fails in Hermes, fix the profile `.env`, service environment, or gateway process before reauthenticating.

## Common Reads

```bash
mog mail list --max 10 --json
mog mail get <message-id> --json
mog calendar list --from 2026-06-19 --to 2026-06-26 --max 20 --json
mog calendar get <event-id> --json
mog contacts list --max 20 --json
mog groups list --max 20 --json
mog groups members <group-id> --max 20 --json
mog teams list --max 20 --json
mog teams channels --team <team-id> --max 20 --json
mog teams chats --max 20 --json
mog tasks lists --json
mog onedrive ls --path / --json
```

Use paging tokens exactly as returned:

```bash
mog teams chats --page '<next-link>' --json
```

## Writes

Before writes, identify the profile, object id, target, and exact mutation. Prefer commands that support `--dry-run`.

```bash
mog mail send --to user@example.com --subject "Subject" --body "Message" --dry-run
mog mail archive <message-id> --dry-run
mog mail move <message-id> --folder <folder-id-or-supported-well-known-name> --dry-run
mog mail mark-read <message-id> --dry-run
mog calendar create --subject "Planning" --start "2026-06-19T10:00:00+08:00" --end "2026-06-19T10:30:00+08:00" --body "Agenda" --dry-run
mog calendar update <event-id> --subject "Updated title" --dry-run
mog calendar delete <event-id> --dry-run
mog contacts create --display-name "Jane Doe" --email jane@example.com --dry-run
mog contacts update <contact-id> --title "Director" --dry-run
mog teams channel-send --team <team-id> --channel <channel-id> --body "Message" --dry-run
mog teams chat-send --chat <chat-id> --body "Message" --dry-run
mog teams dm-send --to user@example.com --body "Message" --dry-run
mog onedrive mkdir --path /AgentTest --dry-run
mog onedrive put ./file.txt --path /AgentTest/file.txt --dry-run
```

Mail workflow mutations are single-message and require distinct allowlisted actions: `mail.archive`, `mail.move`, and `mail.mark-read`. Graph implements archive/move as copy-then-remove and returns a resulting message resource. If mog reports a move outcome as indeterminate, do **not** retry automatically; inspect the source and destination folders first because the write may already have completed.

For direct Teams messages, prefer the first-class DM command:

```bash
mog teams dm-send --to user@dexterton.com --body "Message" --dry-run
```

`dm-send` opens or reuses a one-on-one chat before sending. Its dry run deliberately does not create/open a chat or post a message.

## Discovery

Use CLI help and source rather than guessing flags:

```bash
mog --help
mog <service> --help
mog <service> <command> --help
rg -n 'type .*Cmd|runtimeCapability|MOG_ENABLE_ACTIONS' internal/cmd internal/services internal/auth
```

Repo paths:

- CLI entrypoint: `cmd/mog/`
- Command implementations: `internal/cmd/`
- Runtime/action gating: `internal/cmd/runtime.go`, `internal/cmd/enabled_commands.go`
- Microsoft Graph client: `internal/graph/`
- OAuth/profile handling: `internal/auth/`, `internal/profile/`
- Service implementations: `internal/services/`
- Teams service: `internal/services/teams/`

Verification before shipping changes:

```bash
go test ./...
git diff --check
go build -o bin/mog ./cmd/mog
./bin/mog teams --help
```
