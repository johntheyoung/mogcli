# mogcli Microsoft Port Plan (Revision 2)

Date: 2026-02-19
Status: implementation in progress (auth wizard, progressive delegated consent, and selective app-only routing implemented)
Audience: Codex agents implementing `mogcli`

## Implementation status update (2026-02-19)

Implemented in codebase:

1. Command-level capability matrix enforcement in `internal/cmd/runtime.go` for all current workload commands.
2. Interactive default `mog auth` delegated wizard with workload-group delegated scope selection, plus advanced app-only wizard via `mog auth app`.
3. Interactive `mog auth update` flow to review current profile settings and edit selected fields without full re-onboarding.
4. Scripted `mog auth login` delegated bootstrap with required `--scope-workloads`.
5. Profile metadata persistence for delegated bootstrap workloads and app-only default target user.
6. Progressive delegated consent via per-operation minimal scopes (no broad default command scopes).
7. Delegated token restore hardening: cached scope coverage checks, refresh on missing scope, account/tenant consistency validation, and cache purge on mismatch.
8. App-only endpoint routing for mail/contacts/onedrive using `/users/{id}` with `--user` override and profile fallback.
9. Explicit fail-fast app-only rejection for calendar/tasks with deterministic user-facing guidance.
10. Task mutation mutability error normalization in `internal/services/tasks/service.go` for built-in/well-known Microsoft To Do list constraints.
11. `--dry-run` previews for write operations in mail, calendar, and onedrive command surfaces.
12. OneDrive local path hardening (`SafeExpandPath`), metadata-rich delete confirmation, and progress output for local file transfers.
13. Keyring backend hardening: `auto|keychain|file` resolution, native macOS keychain support, and explicit empty `MOG_KEYRING_PASSWORD` handling for headless contexts.
14. Calendar update patch safety: attendee updates are split from reminder-field patches to avoid Graph validation conflicts.
15. Contacts create/update now supports richer fields (`org`, `title`, `url`, `note`, `custom`) with deterministic custom-field ordering in command output.
16. Mail send supports quoted replies via `--quote <message-id>` to include source message context automatically.

## 0. Critique of the prior plan

The prior plan was directionally correct but not execution-ready for autonomous implementation.

Gaps that would slow or derail implementation:

1. It did not include a file-level copy/port manifest from `gogcli` into `mogcli`, so an agent would still need to rediscover source/destination mapping.
2. It did not define concrete profile/token data models for the now-confirmed split consumer/enterprise app registrations.
3. It did not encode a command-level API matrix (command -> Graph endpoint -> scope -> account-type constraints), which is critical for non-1:1 behavior.
4. It lacked explicit acceptance tests per phase and per workload.
5. It had minimal citations; it referenced ideas but not the exact source files/docs to anchor implementation choices.
6. It did not include a parallel work breakdown for subagents.
7. It did not include failure-mode handling requirements (throttling limits, auth mismatch, unsupported combinations).
8. It left unresolved how to operationalize app-only as a phase-2 capability while keeping phase-1 delegated flows clean.

This revised plan resolves those gaps.

## 1. Locked decisions

Already decided with user:

1. Use separate app registrations/client IDs for consumer and enterprise profiles.
2. Allow login to both audiences, but exactly one active profile at a time.
3. App-only support is workload-scoped: enabled for mail/contacts/onedrive with explicit target-user routing; calendar/tasks remain delegated-only.
4. Use unstyled plain-text terminal output only; remove `--color` and `MOG_COLOR`.

Reference: current project state and sources inventory [L1], [L2].

## 2. Source-backed architecture baseline

`mogcli` is currently an empty implementation shell (`README.md` only), so architecture can be established cleanly [L1].

`gogcli` provides a strong reusable CLI shell:

1. Entry/exit lifecycle and parse/error/exit-code flow [G1], [G2], [G16].
2. Help rendering with terminal-aware formatting [G3].
3. Output mode (`--json`, `--plain`) and serialization helpers [G4].
4. UI printer plumbing adapted to unstyled plain-text output [G5].
5. Error normalization patterns [G6].
6. Config directory/path plumbing and atomic writes [G7].
7. Client/account mapping logic that can be adapted into profile routing [G8].
8. Keyring-first secure secret storage with Linux/macOS handling [G9].
9. Retry/backoff/circuit-breaker transport primitives [G10].
10. Reusable test harness patterns for CLI and environment isolation [G16].

Google-specific pieces that must be replaced:

1. OAuth flow and browser callback implementation [G11], [G13].
2. Service/scope registry and Google service semantics [G12].
3. Google credential JSON parsing model [G14].
4. Google API client factories and service account flow [G11], [G14].

Cross-reference repos provide M365 implementation patterns:

1. Central request wrapper + OData pagination + connection switching [P3], [P4], [P6].
2. Token persistence pitfalls (plain JSON caches) [P5].
3. Per-connection cache, account mismatch invalidation, and safe auth-error sanitization [MCP1], [MCP2].
4. Graph hooks (timezone/signature) and page-cap pagination [MCP2], [MCP3].

## 3. Target architecture for `mogcli`

Language/runtime: Go, preserving `gogcli` shell patterns [G1], [G2], [G17].

Proposed package layout:

1. `cmd/mog/main.go`
2. `internal/cmd/*`
3. `internal/outfmt/*`
4. `internal/ui/*`
5. `internal/errfmt/*`
6. `internal/config/*`
7. `internal/secrets/*`
8. `internal/profile/*`
9. `internal/auth/*`
10. `internal/graph/*`
11. `internal/services/mail/*`
12. `internal/services/calendar/*`
13. `internal/services/contacts/*`
14. `internal/services/groups/*`
15. `internal/services/tasks/*`
16. `internal/services/onedrive/*`

Design invariants:

1. One active profile at a time.
2. Separate consumer/enterprise registrations and token/cache isolation.
3. Delegated-only in phase 1.
4. App-only added in phase 2 behind explicit command/profile mode.
5. Every command declares required scope(s) and unsupported audience/mode combinations.

## 4. Profile and auth model (implementation spec)

### 4.1 Profile model

Store profile metadata in config (not secrets), and tokens in keyring-backed secret store.

`profiles` record (suggested):

1. `name`: string
2. `audience`: `consumer|enterprise`
3. `client_id`: string
4. `authority`: `consumers|organizations|common|<tenant-id-or-domain>`
5. `tenant_id`: string
6. `account_id`: string (home account id / oid)
7. `username`: string
8. `auth_mode`: `delegated|app_only`
9. `active`: bool (single `true` across all profiles)

Use per-profile token cache key namespace:

1. `mog:token:<profile-name>:<account-id>`
2. `mog:refresh:<profile-name>:<account-id>`
3. `mog:cache:<profile-name>` (serialized token cache if needed)

### 4.2 Auth commands

Phase-1 command surface:

1. `mog auth` (interactive delegated wizard) and `mog auth app` (interactive enterprise app-only wizard).
2. `mog auth update [--profile <name>]` to review current settings and edit selected fields.
3. `mog auth login --profile <name> --audience consumer|enterprise --client-id <id> --scope-workloads <csv> [--tenant <tenant>] [--authority ...]`
4. `mog auth login --mode app-only --app-only-user <upn-or-id> ...` for profile-level app-only target-user defaults.
5. `mog auth logout [--profile <name>|--all]`
6. `mog auth accounts`
7. `mog auth use <profile-name>`
8. `mog auth whoami`

Phase-2 additions:

1. `mog auth login --mode app-only --client-id ... --tenant ... --client-secret|--cert ...`

### 4.3 Authority rules

1. `/common`, `/organizations`, `/consumers` authority behavior follows Microsoft identity platform guidance [MS1].
2. Device code flow is default for CLI interaction [MS2].
3. Consumer profile must use consumer-compatible authority and client registration.
4. Enterprise app-only only enabled for enterprise profiles (phase 2).

## 5. Workload matrix (commands, Graph endpoints, scopes, constraints)

Phase-1 delegated MVP scope:

| Workload | Core commands | Primary Graph endpoints | Minimum delegated scopes | Constraints |
|---|---|---|---|---|
| Mail | `list`, `get`, `send` | `/me/messages`, `/me/messages/{id}`, `/me/sendMail` | `Mail.Read`, `Mail.Send` | `sendMail` defaults to saving in Sent Items [MS3], [MS4] |
| Calendar | `list`, `get`, `create`, `update`, `delete` | `/me/events`, `/me/events/{id}`, `/me/calendars/{id}/events` | `Calendars.Read`, `Calendars.ReadWrite` | Group calendar and account-type caveats apply [MS5] |
| Contacts | `list`, `get`, `create`, `update`, `delete` | `/me/contacts`, `/me/contacts/{id}` | `Contacts.Read`, `Contacts.ReadWrite` | None beyond audience gating [MS6] |
| Groups | `list`, `get`, `members` | `/groups`, `/groups/{id}`, `/groups/{id}/members` | `GroupMember.Read.All` or `Group.Read.All` | Not supported for personal Microsoft accounts [MS7] |
| Tasks (To Do) | `lists`, `list`, `get`, `create`, `update`, `complete`, `delete` | `/me/todo/lists`, `/me/todo/lists/{id}/tasks` | `Tasks.Read`, `Tasks.ReadWrite` | Built-in lists have mutability limits [MS9] |
| OneDrive | `ls`, `get`, `put`, `mkdir`, `rm` | `/me/drive`, `/me/drive/root/children`, `/drives/{id}/items/...` | `Files.Read`, `Files.ReadWrite` | Some drive metadata endpoints are delegated-only [MS10], [MS11] |

Current app-only routing/constraints implemented:

1. Mail, Contacts, and OneDrive support app-only via `/users/{id}` endpoint routing.
2. Target user resolution order is command `--user` override, then profile `app_only_user`; missing target returns explicit user-facing error.
3. Calendar and Tasks reject app-only mode with deterministic fail-fast guidance.
4. Groups behavior is unchanged (non-`/me`, enterprise-gated).

## 6. Port manifest: copy vs adapt vs rewrite

### 6.1 Copy mostly unchanged (shell)

| Source (`opensrc/repos/github.com/steipete/gogcli`) | Destination | Action |
|---|---|---|
| `cmd/gog/main.go` | `cmd/mog/main.go` | Rename imports/binary identifiers |
| `internal/cmd/root.go` | `internal/cmd/root.go` | Keep parser flow; replace command tree/auth binding |
| `internal/cmd/help_printer.go` | `internal/cmd/help_printer.go` | Keep with `GOG_*` -> `MOG_*` env changes |
| `internal/cmd/usage.go`, `internal/cmd/exit.go` | same paths | Keep |
| `internal/outfmt/outfmt.go` | same path | Keep with env rename |
| `internal/ui/ui.go` | same path | Keep |
| `internal/cmd/output_helpers.go`, `internal/cmd/confirm.go`, `internal/cmd/flags_output.go` | same paths | Keep |
| `internal/input/prompt.go`, `internal/input/readline.go` | same path | Keep |
| `internal/config/config.go`, `internal/config/paths.go`, `internal/config/clients.go`, `internal/config/aliases.go` | same path | Keep structure, adjust schema/names |
| `internal/secrets/*` | same path | Keep with key names/env rename |
| `internal/googleapi/transport.go`, `retry_constants.go`, `circuitbreaker.go` | `internal/graph/*` | Move/rename package, keep logic |
| `internal/cmd/testutil_test.go`, `internal/cmd/testmain_test.go` | same path | Keep and retarget |

### 6.2 Rewrite for Microsoft

| Source area | Destination | Action |
|---|---|---|
| `internal/googleauth/*` | `internal/auth/*` | Replace with Microsoft identity/device-code/auth profile flows |
| `internal/googleapi/client.go` and service constructors | `internal/graph/client.go` + `internal/services/*` | Replace with Graph HTTP client + per-workload services |
| Google command implementations (`gmail`, `calendar`, `contacts`, `groups`, `tasks`, `drive`) | `internal/cmd/*` + `internal/services/*` | Port command UX shape and output, replace API calls |
| Google credentials parsing in `internal/config/credentials.go` | `internal/config/registrations.go` | Replace with app registration/profile schema |
| Google-specific error mapping in `internal/errfmt/errfmt.go` | same path | Replace with AADSTS/Graph-safe mapping |

References for source behavior [G2], [G10], [G11], [G12], [G14], [G15].

## 7. Phase plan with concrete tasks and exit criteria

### Phase 0 - Bootstrap and shell import

Objective: make `mog` executable with global flags, help, output modes, and config/secret foundations.

Tasks:

1. Initialize Go module and base build files (adapt `Makefile`, optional `.goreleaser.yaml`) [G17].
2. Copy shell components from manifest section 6.1.
3. Rename environment/config keys (`GOG_*` -> `MOG_*`) and app name path defaults (`gogcli` -> `mogcli`) [G2], [G4], [G7], [G9].
4. Reduce command tree to placeholders: `auth`, `mail`, `calendar`, `contacts`, `groups`, `tasks`, `onedrive`, `config`, `version`, `completion`.

Exit criteria:

1. `go test ./...` passes for imported shell tests.
2. `mog --help` renders with build/config info.
3. `mog --json version` and `mog --plain version` produce expected mode behavior.

### Phase 1 - Delegated auth and profile system

Objective: robust login and profile switching for consumer and enterprise.

Tasks:

1. Implement `internal/profile` store with single-active-profile invariant.
2. Implement `internal/auth` delegated device-code login flow.
3. Implement account mismatch protection and cache invalidation semantics inspired by `m365-mcp-suite` [MCP1], [MCP2].
4. Implement `auth login/update/logout/status/use` commands (with `accounts/whoami` compatibility aliases).
5. Implement secure token persistence through `internal/secrets`.

Exit criteria:

1. Can login to one consumer and one enterprise profile with separate client IDs.
2. `mog auth use <profile>` toggles active profile and updates subsequent commands.
3. Restarting CLI supports silent token reuse where valid.
4. Wrong-account token restoration is detected and rejected.

### Phase 2 - Graph client platform

Objective: shared Graph transport, pagination, retries, and error normalization.

Tasks:

1. Build `internal/graph/client.go` with:
- auth header injection
- retry/backoff with max attempt caps
- respect `Retry-After`
- circuit breaker support
2. Build OData pagination helper (`@odata.nextLink`) with optional hard cap.
3. Add per-request hooks framework (timezone header for calendar, optional mail body preprocess), inspired by `graph-tools.ts` and `mm/server.py` [MCP2], [MCP3].
4. Add Graph/AADSTS error sanitizer in `internal/errfmt` [MCP2].

Exit criteria:

1. Throttled test paths eventually succeed or fail with bounded retries.
2. Pagination helper validated against mocked multi-page responses.
3. Error output is actionable and does not leak sensitive tenant details.

### Phase 3 - Workload MVP (delegated only)

Objective: implement target six workloads end-to-end.

Execution order (parallelizable by subagents):

1. OneDrive
2. Mail
3. Calendar
4. Contacts
5. Tasks
6. Groups

Per-workload requirements:

1. `list` and `get` required.
2. At least one mutating command.
3. Table/plain/json output parity.
4. `--max`/`--page` paging UX with next page hints.
5. `--page` accepts raw Graph `@odata.nextLink`; `--next-token` is an alias for the same value.
6. Required scopes listed in help.

Exit criteria:

1. Command set from section 5 works against mocked Graph.
2. Live smoke tests pass for MSA delegated (where supported) and Entra delegated.
3. Unsupported audience combinations fail fast with clear messages.

### Phase 4 - Enterprise hardening and app-only (phase 2 commitment)

Objective: add app-only mode for supported endpoints without regressing delegated flows.

Tasks:

1. Add app-only profile auth mode based on client-credentials guidance [MS12].
2. Add command routing rules for app-only path substitutions (`/me/...` -> explicit user/drive forms where needed).
3. Implement permission matrix guardrails.
4. Provide admin consent guidance docs/command output.

Exit criteria:

1. App-only scenarios succeed for supported workloads.
2. Unsupported app-only workloads fail with deterministic, cited messages.
3. Delegated workflows unaffected.

### Phase 5 - Migration UX and parity docs

Objective: make migration from `gogcli` predictable.

Tasks:

1. Add alias mapping where it improves discoverability.
2. Publish `docs/migration-gog-to-mog.md` with command examples.
3. Add troubleshooting doc for auth, scope consent, and tenant/account mismatch.

Exit criteria:

1. Migration guide covers top workflows for all six workloads.
2. End-to-end smoke matrix documented and reproducible.

## 8. Subagent work packages (recommended)

Use these in parallel where possible:

1. Shell porter agent:
- owns phase 0 file import/renames
- outputs compile-ready CLI shell and foundational tests
2. Auth/profile agent:
- owns phase 1 profile schema, token cache integration, auth commands
- owns active-profile invariant enforcement
3. Graph platform agent:
- owns phase 2 transport/pagination/retry/error mapping/hook framework
4. Workload agents (up to 3 parallel):
- Agent A: OneDrive + Mail
- Agent B: Calendar + Contacts
- Agent C: Tasks + Groups
5. QA/docs agent:
- owns phase 5 migration docs, test matrix, and command help scope audit

## 9. Testing strategy and matrix

### 9.1 Test types

1. Unit tests:
- parser/flags/help behavior
- profile selection and active-profile invariant
- scope resolution and audience gating
- retry/pagination/error mapping
2. Contract tests:
- HTTP mocks for Graph responses and errors
- pagination/throttle/error edge cases
3. Live integration tests (opt-in):
- MSA delegated profile
- Entra delegated profile
- Entra app-only profile (phase 4+)

### 9.2 Required scenario matrix

| Scenario | Mail | Calendar | Contacts | Groups | Tasks | OneDrive |
|---|---|---|---|---|---|---|
| MSA delegated | yes | yes | yes | no (expected fail) | yes | yes |
| Entra delegated | yes | yes | yes | yes | yes | yes |
| Entra app-only (phase 4+) | partial | partial | partial | yes | partial | partial |

All expected failures must assert explicit error text and exit codes.

## 10. Risk register and mitigations

1. Risk: token/account cross-contamination across profiles.
Mitigation: isolate cache by profile+account, enforce mismatch invalidation [MCP1], [MCP2].

2. Risk: infinite retry loops under 429/503.
Mitigation: bounded retries and jitter; avoid recursive unbounded strategy seen in references [P3].

3. Risk: plaintext token persistence.
Mitigation: keyring-first secret storage with encrypted file fallback patterns [G9], [P5], [MCP5].

4. Risk: incorrect audience handling for groups and app-only paths.
Mitigation: explicit capability matrix in command metadata [MS5], [MS7], [MS8], [MS9], [MS10].

5. Risk: missing parity due broad command surface differences.
Mitigation: phase-scoped command surface and migration guide before expansion [G15].

## 11. Remaining open decisions

These are still open and should be resolved before phase-0 implementation begins:

1. Login UX preference: device-code only, or also support localhost browser callback in phase 1.
2. Command naming finalization: keep noun-first (`mog mail list`) or align more tightly with `gogcli` patterns.

## 12. Citation index

### Local codebase citations

- [L1] `README.md:1`
- [L2] `opensrc/sources.json:1`
- [G1] `opensrc/repos/github.com/steipete/gogcli/cmd/gog/main.go:9`
- [G2] `opensrc/repos/github.com/steipete/gogcli/internal/cmd/root.go:26`
- [G3] `opensrc/repos/github.com/steipete/gogcli/internal/cmd/help_printer.go:16`
- [G4] `opensrc/repos/github.com/steipete/gogcli/internal/outfmt/outfmt.go:12`
- [G5] `opensrc/repos/github.com/steipete/gogcli/internal/ui/ui.go:13`
- [G6] `opensrc/repos/github.com/steipete/gogcli/internal/errfmt/errfmt.go:17`
- [G7] `opensrc/repos/github.com/steipete/gogcli/internal/config/paths.go:12`, `opensrc/repos/github.com/steipete/gogcli/internal/config/config.go:12`
- [G8] `opensrc/repos/github.com/steipete/gogcli/internal/config/clients.go:12`, `opensrc/repos/github.com/steipete/gogcli/internal/authclient/authclient.go:36`
- [G9] `opensrc/repos/github.com/steipete/gogcli/internal/secrets/store.go:18`
- [G10] `opensrc/repos/github.com/steipete/gogcli/internal/googleapi/transport.go:14`, `opensrc/repos/github.com/steipete/gogcli/internal/googleapi/retry_constants.go:5`, `opensrc/repos/github.com/steipete/gogcli/internal/googleapi/circuitbreaker.go:9`
- [G11] `opensrc/repos/github.com/steipete/gogcli/internal/googleauth/oauth_flow.go:25`
- [G12] `opensrc/repos/github.com/steipete/gogcli/internal/googleauth/service.go:10`
- [G13] `opensrc/repos/github.com/steipete/gogcli/internal/googleauth/accounts_server.go:33`
- [G14] `opensrc/repos/github.com/steipete/gogcli/internal/config/credentials.go:20`
- [G15] `opensrc/repos/github.com/steipete/gogcli/internal/cmd/calendar.go:13`, `opensrc/repos/github.com/steipete/gogcli/internal/cmd/drive.go:52`, `opensrc/repos/github.com/steipete/gogcli/internal/cmd/contacts.go:15`, `opensrc/repos/github.com/steipete/gogcli/internal/cmd/tasks.go:9`, `opensrc/repos/github.com/steipete/gogcli/internal/cmd/groups.go:26`
- [G16] `opensrc/repos/github.com/steipete/gogcli/internal/cmd/testutil_test.go:130`, `opensrc/repos/github.com/steipete/gogcli/internal/cmd/testmain_test.go:9`
- [G17] `opensrc/repos/github.com/steipete/gogcli/go.mod:1`, `opensrc/repos/github.com/steipete/gogcli/Makefile:1`, `opensrc/repos/github.com/steipete/gogcli/.goreleaser.yaml:1`
- [P1] `opensrc/repos/github.com/pnp/cli-microsoft365/src/Auth.ts:47`
- [P2] `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/commands/login.ts:13`
- [P3] `opensrc/repos/github.com/pnp/cli-microsoft365/src/request.ts:14`
- [P4] `opensrc/repos/github.com/pnp/cli-microsoft365/src/utils/odata.ts:21`
- [P5] `opensrc/repos/github.com/pnp/cli-microsoft365/src/auth/FileTokenStorage.ts:6`, `opensrc/repos/github.com/pnp/cli-microsoft365/docs/docs/concepts/persisting-connection.mdx:46`
- [P6] `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/connection/commands/connection-use.ts:17`, `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/connection/commands/connection-list.ts:10`
- [P7] `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/outlook/commands.ts:1`, `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/onedrive/commands.ts:1`, `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/todo/commands.ts:1`, `opensrc/repos/github.com/pnp/cli-microsoft365/src/m365/entra/commands.ts:1`
- [MCP1] `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/mm/server.py:105`
- [MCP2] `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/mm/server.py:184`, `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/mm/server.py:216`, `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/mm/server.py:450`
- [MCP3] `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/_archived/graph/src/graph-tools.ts:170`, `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/_archived/graph/src/graph-tools.ts:226`
- [MCP4] `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/session-pool/session_pool.py:145`
- [MCP5] `opensrc/repos/github.com/ForITLLC/m365-mcp-suite/_archived/graph/src/auth.ts:8`

### Microsoft documentation citations

- [MS1] https://learn.microsoft.com/en-us/entra/identity-platform/msal-client-application-configuration
- [MS2] https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-device-code
- [MS3] https://learn.microsoft.com/en-us/graph/api/user-list-messages?view=graph-rest-1.0
- [MS4] https://learn.microsoft.com/en-us/graph/api/user-sendmail?view=graph-rest-1.0
- [MS5] https://learn.microsoft.com/en-us/graph/api/calendar-list-events?view=graph-rest-1.0
- [MS6] https://learn.microsoft.com/en-us/graph/api/contact-get?view=graph-rest-1.0
- [MS7] https://learn.microsoft.com/en-us/graph/api/group-list?view=graph-rest-1.0
- [MS8] https://learn.microsoft.com/en-us/graph/api/todo-list-lists?view=graph-rest-1.0
- [MS9] https://learn.microsoft.com/en-us/graph/api/resources/todotasklist?view=graph-rest-1.0
- [MS10] https://learn.microsoft.com/en-us/graph/api/driveitem-list-children?view=graph-rest-1.0
- [MS11] https://learn.microsoft.com/en-us/graph/api/drive-get?view=graph-rest-1.0
- [MS12] https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow
