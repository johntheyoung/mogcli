# mogcli

Unofficial agent-friendly Microsoft 365 CLI

`mogcli` is a Microsoft Graph CLI for personal Microsoft accounts (MSA) and enterprise Microsoft Entra ID accounts. It provides scriptable commands for Mail, Calendar, Contacts, Groups, Teams, Tasks, and OneDrive.

## What mogcli supports

- Multiple profiles with exactly one active profile at a time.
- Consumer and enterprise audiences.
- Delegated user auth and enterprise app-only auth.
- Stable scripting output modes: `--json` and `--plain`.
- Interactive delegated wizard (`mog auth`), advanced app-only wizard (`mog auth app`), settings editor (`mog auth update`), and non-interactive login (`mog auth login`).
- Per-command scope requests in delegated mode (progressive consent).
- `--dry-run` previews for write operations in Mail, Calendar, Teams, and OneDrive.

### Workload support matrix

| Workload | Delegated | App-only |
|---|---|---|
| Mail | Yes | Yes (enterprise, requires target user) |
| Calendar | Yes | No |
| Contacts | Yes | Yes (enterprise, requires target user) |
| Groups | Enterprise only | Enterprise only |
| Teams | Enterprise delegated only | No |
| Tasks (Microsoft To Do) | Yes | No |
| OneDrive | Yes | Yes (enterprise, requires target user) |

Notes:

- App-only mode is enterprise-only.
- Calendar and tasks are intentionally blocked in app-only mode.
- Groups are intentionally blocked for consumer profiles.
- Teams commands are intentionally delegated-only because channel and chat message sends act on behalf of the signed-in user.

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
  --scope-workloads mail,calendar,contacts,tasks,onedrive,groups,teams
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
- `mog mail folders|list|get|send|archive|move|mark-read`
- `mog calendar list|get|create|update|delete`
- `mog contacts list|get|create|update|delete`
- `mog groups list|get|members`
- `mog teams list|channels|channel-send|chats|chat-members|chat-send|chat-file-send|dm-send`
- `mog tasks lists|list|get|create|update|complete|delete`
- `mog onedrive ls|get|put|mkdir|rm`
- `mog config get|keys|set|unset|list|path`
- `mog completion <shell>`

## Usage examples

Mail:

```bash
mog mail folders --max 50
mog mail folders --include-hidden --json
mog mail list --max 50 --query "from:alerts@example.com"
mog mail list --folder inbox --max 50
mog mail list --folder <folder-id> --max 50
mog mail get <message-id>
mog mail send --to dev@contoso.com --subject "Deploy complete" --body "Finished."
mog mail send --to dev@contoso.com --subject "Re: Deploy complete" --quote <message-id>
mog mail send --to dev@contoso.com --subject "Deploy complete" --body "Finished." --dry-run
mog mail archive <message-id> --dry-run
mog mail move <message-id> --folder <folder-id-or-supported-well-known-name> --dry-run
mog mail mark-read <message-id> --dry-run
```

`mog mail folders` lists top-level folders and their IDs, parent IDs, child counts, total item counts, and unread counts. Hidden folders are excluded by default; pass `--include-hidden` to request them explicitly.

Without `--folder`, `mog mail list` preserves the mailbox-wide `/messages` behavior. Pass a Graph folder ID or well-known folder name such as `inbox` to use the folder-scoped messages endpoint. Message list responses include routing, conversation, recipient, timestamp, status, importance, flag, category, classification, attachment-presence, and web-link metadata, but never request message bodies or attachments. Mail folder/list/get reads request Outlook immutable IDs with `Prefer: IdType="ImmutableId"`.

`mog mail archive` is a narrow convenience that moves one explicit message to Graph's documented `archive` well-known folder. `mog mail move` requires one explicit message ID and one explicit destination folder ID or supported well-known name. `mog mail mark-read` sets `isRead=true` on one explicit message. Each command supports `--dry-run`, performs no mailbox mutation during a dry run, and does not require `--force`.

Graph implements message moves by creating a new copy in the destination and removing the original. A successful move returns HTTP 201 and the new message resource, including its resulting ID. mog never automatically retries a move; transport failures, server errors, and unrecognized success responses are reported as potentially indeterminate so callers can inspect mailbox state before deciding what to do next.

When the action guard is configured, folder discovery and each mail mutation must be enabled explicitly as `mail.folders`, `mail.archive`, `mail.move`, or `mail.mark-read`; enabling another mail action does not authorize them.

### Managed automation guard

Interactive CLI use remains unrestricted when no allowlists are configured. For a managed or headless process, opt in explicitly with `MOG_MANAGED_AUTOMATION=true` (or `--managed-automation`) and configure both allowlists:

```bash
MOG_MANAGED_AUTOMATION=true \
MOG_ENABLE_COMMANDS=mail \
MOG_ENABLE_ACTIONS=mail.folders,mail.list,mail.get,mail.archive,mail.move,mail.mark-read \
mog mail folders --json
```

Managed automation fails closed if either `MOG_ENABLE_COMMANDS` or `MOG_ENABLE_ACTIONS` is missing, empty, whitespace-only, or contains only empty comma-separated entries. This mode is never inferred from a TTY, `CI`, or another ambient environment variable. Use `all` or `*` explicitly if a managed process intentionally needs an unrestricted allowlist.

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

Teams:

```bash
mog teams list --max 100
mog teams channels --team <team-id> --max 100
mog teams channel-send --team <team-id> --channel <channel-id> --body "Deploy complete" --dry-run
mog teams chats --max 50
mog teams chat-members --chat <chat-id> --max 50
mog teams chat-send --chat <chat-id> --body "Deploy complete" --dry-run
mog teams chat-send --chat <chat-id> --body "Standup is ready" --mention "Jane Doe:<aad-object-id>" --dry-run
mog teams chat-file-send --chat <chat-id> --file ./report.pdf --dry-run --json
mog teams chat-file-send --chat <chat-id> --file ./report.pdf --name "Quarterly report.pdf" --body "Please review" --json
mog teams dm-send --to user@contoso.com --body "Deploy complete" --dry-run
```

For chat mentions, repeat `--mention "Display Name:<aad-object-id>"` for each Teams @mention. `chat-send` builds the Microsoft Graph HTML `<at>` tags and top-level `mentions` array; plain text bodies are escaped before mention tags are appended.

`teams chat-file-send` is an enterprise-delegated, one-on-one-only file send. The caller supplies an existing chat ID. Before uploading, mog resolves `/me`, confirms the chat resource has `chatType: oneOnOne`, and requires exactly the signed-in internal member plus one other same-tenant AAD member with an immutable `userId`. Guest, external, paged, incomplete, or ambiguous membership responses fail closed. The command never selects a recipient from an ambiguous member list.

The profile must be authorized for both the `teams` and `onedrive` delegated workloads. The operation uses the existing delegated permissions `User.Read`, `Chat.ReadBasic` (to read and verify `chatType`), `ChatMember.Read`, `Files.ReadWrite`, and `ChatMessage.Send`. It does not use `User.ReadBasic.All`, tenant-wide file/site write permissions, application permissions, anonymous links, or organization-wide links. In managed automation, enable the separate action `teams.chat-file-send`; `teams.chat-send`, `teams.dm-send`, and `onedrive.put` do not authorize it:

```bash
MOG_MANAGED_AUTOMATION=true \
MOG_ENABLE_COMMANDS=teams \
MOG_ENABLE_ACTIONS=teams.chat-file-send \
mog teams chat-file-send --chat <chat-id> --file ./report.pdf --dry-run --json
```

The default attachment name is the local basename; `--name` overrides it after strict filename validation, including rejection of Unicode formatting controls that can visually spoof names. The optional `--body` is treated as short text (maximum 4096 bytes), HTML-escaped, and sent as HTML only so Teams can receive the required `<attachment>` marker. Local files are checked with `lstat`: symlinks and non-regular files are rejected, the maximum is 50 MiB, and mog hashes the exact bytes with SHA-256 without emitting file content or the full local parent path.

Dry run is entirely local: it validates and hashes the file but acquires no token, makes no Graph call, and does not validate the remote chat or recipient. Its JSON reports the planned stages, `conflict_policy: "fail"`, direct sign-in-required read permission policy with `retain_inherited_permissions: false`, and rollback policy.

On a live run, mog uploads a uniquely and neutrally named item under its private OneDrive app folder using simple upload with conflict behavior `fail`, reads the item back by ID to verify byte count and SHA-256, and grants only the verified other member direct read access (`requireSignIn: true`, `sendInvitation: false`, `retainInheritedPermissions: false`). Before sending, mog lists the resulting permissions and fails closed unless it can prove the item has exactly the recipient's new direct read grant plus at most the signed-in user's explicit owner grant—no inherited grants, other users/groups, or sharing links of any scope. Graph can create the app folder on first access; mog treats that folder as shared managed infrastructure and never removes it during per-send rollback. Failures before a confirmed message HTTP 201 remove only the permission and drive item created by that operation. An indeterminate final-send transport failure or server-side 5xx response is never retried and preserves the backing item and permission to avoid breaking a message that may have been delivered. Successfully sent backing files also remain in OneDrive and count against the user's quota.

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
mog mail folders --user user@contoso.com --max 20
mog mail list --user user@contoso.com --max 20
mog mail list --user user@contoso.com --folder inbox --max 20
mog onedrive ls --user user@contoso.com --path /
```

## Pagination and scripting

Most list commands support `--page` to resume from a next-page token.

```bash
mog groups list --max 50 --json
mog groups list --page "<next-token-url>"
```

`--next-token` is also accepted as an alias for pagination resume flags where supported.

Mail list and folder commands return one Microsoft Graph page per invocation. `--max` sets `$top` only for the initial request; Graph can return fewer items and still provide a `next` URL. Pass that opaque URL back with `--page` to continue. A resumed request uses the endpoint, filters, hidden-folder mode, and page sizing encoded by Graph in that URL instead of reapplying initial request selectors. The presence of fewer than `--max` results never implies complete coverage.

Their JSON responses retain the existing `messages` or `folders` collection and opaque `next` URL, and also report page coverage explicitly: `hasMore` is true and `complete` is false when Graph supplied `@odata.nextLink`; otherwise `hasMore` is false and `complete` is true. These fields describe coverage of the requested list across Graph pages, not recursive child-folder traversal.

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
mog auth login --profile <profile> --audience enterprise --client-id <id> --scope-workloads mail,calendar,contacts,tasks,onedrive,teams
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
