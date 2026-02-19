# mogcli

Microsoft 365 in your terminal.

`mogcli` is a Microsoft Graph CLI for personal Microsoft accounts (MSA) and enterprise Microsoft Entra ID accounts. It provides scriptable commands for Mail, Calendar, Contacts, Groups, Tasks, and OneDrive.

## What mogcli supports

- Multiple profiles with exactly one active profile at a time.
- Consumer and enterprise audiences.
- Delegated user auth and enterprise app-only auth.
- Stable scripting output modes: `--json` and `--plain`.
- Interactive delegated wizard (`mog auth`), advanced app-only wizard (`mog auth app`), settings editor (`mog auth update`), and non-interactive login (`mog auth login`).
- Per-command scope requests in delegated mode (progressive consent).
- `--dry-run` previews for write operations in Mail, Calendar, and OneDrive.

### Workload support matrix

| Workload | Delegated | App-only |
|---|---|---|
| Mail | Yes | Yes (enterprise, requires target user) |
| Calendar | Yes | No |
| Contacts | Yes | Yes (enterprise, requires target user) |
| Groups | Enterprise only | Enterprise only |
| Tasks (Microsoft To Do) | Yes | No |
| OneDrive | Yes | Yes (enterprise, requires target user) |

Notes:

- App-only mode is enterprise-only.
- Calendar and tasks are intentionally blocked in app-only mode.
- Groups are intentionally blocked for consumer profiles.

## Install

### Option 1: Build from source

```bash
git clone https://github.com/jaredpalmer/mogcli.git
cd mogcli
go build -o bin/mog ./cmd/mog
./bin/mog --help
```

### Option 2: go install

```bash
go install github.com/jaredpalmer/mogcli/cmd/mog@latest
mog --help
```

### Option 3: Homebrew

```bash
brew tap jaredpalmer/tap
brew install jaredpalmer/tap/mogcli
mog --help
```

## Microsoft app setup prerequisites

Before login, create app registrations in Microsoft Entra:

1. Consumer app registration (for MSA audience).
2. Enterprise app registration (for work/school audience).

For delegated login:

1. Enable public client flow in app Authentication settings.
2. Add delegated Graph permissions for the workloads you plan to use.

For app-only login:

1. Use an enterprise app registration.
2. Add required application permissions.
3. Grant admin consent.
4. Create a client secret.

## Quick start

### 1) Interactive setup (recommended)

```bash
mog auth
```

This wizard configures the profile and starts delegated device-code login.

Advanced app-only interactive setup (enterprise only):

```bash
mog auth app
```

### 2) Scripted delegated login

Consumer profile:

```bash
mog auth login \
  --profile personal \
  --audience consumer \
  --client-id <consumer-client-id> \
  --scope-workloads mail,calendar,contacts,tasks,onedrive
```

Enterprise delegated profile:

```bash
mog auth login \
  --profile work \
  --audience enterprise \
  --client-id <enterprise-client-id> \
  --tenant <tenant-id-or-domain> \
  --scope-workloads mail,calendar,contacts,tasks,onedrive,groups
```

### 3) Scripted app-only login (enterprise)

```bash
export MOG_CLIENT_SECRET="<client-secret-value>"

mog auth login \
  --profile work-app \
  --audience enterprise \
  --mode app-only \
  --client-id <enterprise-client-id> \
  --tenant <tenant-id-or-domain> \
  --app-only-user user@contoso.com \
  --client-secret-env MOG_CLIENT_SECRET
```

### 4) Switch and inspect profiles

```bash
mog auth accounts
mog auth use work
mog auth whoami
```

For one-off command routing without switching active profile:

```bash
mog --use-profile work mail list --max 10
```

### 5) Update existing auth settings without full re-onboarding

```bash
mog auth update
mog auth update --profile work
```

The update flow shows current settings, lets you choose one field at a time to edit, and saves only selected changes.

## Command overview

- `mog auth`
- `mog auth app`
- `mog auth login|update|logout|accounts|use|whoami`
- `mog mail list|get|send`
- `mog calendar list|get|create|update|delete`
- `mog contacts list|get|create|update|delete`
- `mog groups list|get|members`
- `mog tasks lists|list|get|create|update|complete|delete`
- `mog onedrive ls|get|put|mkdir|rm`
- `mog config get|keys|set|unset|list|path`
- `mog completion <shell>`

## Usage examples

Mail:

```bash
mog mail list --max 50 --query "from:alerts@example.com"
mog mail get <message-id>
mog mail send --to dev@contoso.com --subject "Deploy complete" --body "Finished."
mog mail send --to dev@contoso.com --subject "Re: Deploy complete" --quote <message-id>
mog mail send --to dev@contoso.com --subject "Deploy complete" --body "Finished." --dry-run
```

Calendar:

```bash
mog calendar list --from 2026-02-12 --to 2026-02-19 --max 100
mog calendar create \
  --subject "Planning" \
  --start "2026-02-13T16:00:00-08:00" \
  --end "2026-02-13T16:30:00-08:00" \
  --body "Weekly sync"
mog calendar delete <event-id> --dry-run
```

Contacts:

```bash
mog contacts list --max 100
mog contacts create --display-name "Jane Doe" --email "jane@contoso.com"
mog contacts create \
  --display-name "Jane Doe" \
  --email "jane@contoso.com" \
  --org "Contoso" \
  --title "Program Manager" \
  --url "https://contoso.example/jane" \
  --note "Customer success lead" \
  --custom region=NA \
  --custom team=platform
mog contacts update <contact-id> --title "Director" --custom region=EMEA
```

Groups:

```bash
mog groups list --max 100
mog groups members <group-id> --max 100
```

Tasks:

```bash
mog tasks lists
mog tasks list --list <list-id> --max 100
mog tasks create --list <list-id> --title "Follow up"
mog tasks complete --list <list-id> --task <task-id>
```

OneDrive:

```bash
mog onedrive ls --path / --max 100
mog onedrive put ./report.pdf --path /Reports/report.pdf
mog onedrive get /Reports/report.pdf --out ./report.pdf
mog onedrive mkdir --path /Reports/Archive
mog onedrive rm --path /Reports/old-report.pdf
mog onedrive rm --path /Reports/old-report.pdf --dry-run
```

If `mog onedrive get` is run without `--out`, files are saved under the local `onedrive-downloads` directory in your mogcli config path.

App-only target user override (mail/contacts/onedrive):

```bash
mog mail list --user user@contoso.com --max 20
mog onedrive ls --user user@contoso.com --path /
```

## Pagination and scripting

Most list commands support `--page` to resume from a next-page token.

```bash
mog groups list --max 50 --json
mog groups list --page "<next-token-url>"
```

`--next-token` is also accepted as an alias for pagination resume flags where supported.

Output modes:

- `--json`: structured output for tooling.
- `--plain`: stable tab-separated output for shell scripts.

Examples:

```bash
mog mail list --json | jq '.messages[0]'
mog tasks list --list <list-id> --plain
```

## Configuration and secrets

Show config path:

```bash
mog config path
```

Show editable config keys:

```bash
mog config keys
```

Current keys:

- `timezone`
- `keyring_backend`

Profile metadata is stored in config. Tokens and secrets are stored via keychain/keyring backends.

`keyring_backend` supports `auto`, `keychain`, and `file`.

- `auto`: use native OS keychain when available, otherwise file backend.
- `keychain`: require native OS keychain support.
- `file`: store under the local mogcli keyring directory.

`MOG_KEYRING_PASSWORD` is treated as explicitly configured even when empty, so headless runs do not implicitly prompt for keyring passwords.

## Troubleshooting

No active profile:

```bash
mog auth accounts
mog auth use <profile>
```

Refresh delegated login:

```bash
mog auth login --profile <profile> --audience enterprise --client-id <id> --scope-workloads mail,calendar,contacts,tasks,onedrive
```

Logout and reset profile auth state:

```bash
mog auth logout --profile <profile>
```

Verbose mode:

```bash
mog --verbose mail list
```

## Development

```bash
go test ./...
go run ./cmd/mog --help
```

Additional project docs are in `docs/`.

## License

MIT
