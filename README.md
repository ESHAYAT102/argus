# Argus

Argus is a Go CLI for managing named sets of environment variables across GitHub repositories and local projects.

The repository contains both the `argus` CLI and its Neon-backed API service.

## Install

The installer clones this repository, builds `argus` with Go, copies the binary into your user binary directory, and removes the cloned repository when it is done.

If Go or Git is missing, the installer asks before installing them with the platform package manager. It supports pacman, apt, dnf, yum, zypper, apk, xbps, Homebrew, winget, Chocolatey, and Scoop.

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/ESHAYAT102/argus/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ESHAYAT102/argus/main/scripts/install.ps1 | iex
```

The binary is installed to `~/.local/bin/argus` on macOS and Linux, or `$HOME\.local\bin\argus.exe` on Windows.

## Uninstall

macOS and Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/ESHAYAT102/argus/main/scripts/uninstall.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ESHAYAT102/argus/main/scripts/uninstall.ps1 | iex
```

## Commands

```text
argus auth                         Sign in with GitHub
argus whoami                       Show the authenticated GitHub username
argus logout                       Log out
argus init [environment]           Create a project and push .env
argus push [environment]           Push .env
argus pull [environment]           Pull into .env; auto-select the only environment
argus rename <environment> <new-name> Rename an environment
argus set <variable> [value]       Update one variable
argus status                       Show local/remote synchronization status
argus diff <environment>           Compare variable names without showing values
argus delete <variable>            Delete a variable locally and remotely
argus project link <project> [env] Link this directory to an existing project
argus project rename <project> <new-name> Rename a project
argus share <project> <user>         Invite a GitHub user
argus project members <project>      List project members and roles
argus project role <project> <user> <role> Change a member's role
argus project unshare <project> <user> Remove project access
argus invites                       List pending invitations
argus invites accept <id>           Accept an invitation
argus invites decline <id>          Decline an invitation
argus list                         List projects and environments
argus history [project]            Show current or named project activity
argus remove <environment>         Remove an environment
argus destroy <project>            Destroy a project by name
```

`ls`, `activity`, and `rm` are aliases for `list`, `history`, and `remove`.

`argus auth` copies GitHub's one-time code to the clipboard, waits for Enter, and opens the verification page in the default browser. If desktop integration is unavailable, it prints manual instructions instead.

`argus status` compares `.env` with the active environment. `argus diff` shows only variable names marked as local-only, changed, or remote-only; secret values are never printed. These comparisons do not create fetch entries in project history.

`argus delete` requires confirmation and removes the variable remotely before updating `.env`. `argus project link` stores the association in the user-wide registry below, never inside the project. When a project has multiple environments and none is supplied, Argus presents an environment picker.

`argus rename <environment> <new-name>` renames an environment in the current project. `argus project rename <project> <new-name>` renames a project. Successful renames update every matching entry in the user-wide registry and are recorded in activity history.

## Sharing projects

Project owners and admins can invite collaborators by GitHub username. Invitations expire after seven days and do not grant access until the recipient explicitly accepts:

```bash
argus share portfolio octocat
argus share portfolio octocat --role viewer
argus invites
argus invites accept <invitation-id>
```

If `--role` is omitted, Argus presents a role picker. Roles are enforced by the API:

- `viewer` can list, pull, inspect differences, and view history.
- `member` can also push and change environments and variables.
- `admin` can also invite, remove, and change the roles of members and viewers.
- `owner` has complete control, including admin management and project destruction. Ownership cannot be granted, removed, or demoted through sharing commands.

Manage existing access with `argus project members`, `argus project role`, and `argus project unshare`. A recipient can link an accepted project to a directory with `argus project link`.

## Safe pulls

When `argus pull` encounters a non-empty `.env` that was not created by the previous pull—or that has since been modified—it moves the existing file to a timestamped backup:

```text
.env.backup.20260804-143052
```

Before switching environments, Argus compares the local variable names and values with the currently active remote environment. An exact match is replaced without a redundant backup; any local addition, removal, or changed value is backed up. Replacements are written to a temporary owner-only file first. If a backed-up installation fails, Argus attempts to restore the original file.

## Development

```bash
go test ./...
go run ./cmd/argus help
```

Set `ARGUS_API_URL` to point the CLI at a development API:

```bash
ARGUS_API_URL=http://localhost:8080 go run ./cmd/argus list
```

Directory associations for every project are stored in one user registry. It contains canonical directory paths, project identifiers, and active environments—never secret values.

- Linux and macOS: `${XDG_DATA_HOME:-~/.local/share}/argus/argus.toml`
- Windows: `%LOCALAPPDATA%\Argus\argus.toml`

Argus uses the current project directory to select the correct registry entry. Set `ARGUS_DATA_HOME` to override the Argus data directory for a portable installation.

```toml
version = 1

[[projects]]
directory = "/home/esh/code/portfolio"
project_id = "..."
project_name = "portfolio"
environment = "prod"
```

## Neon backend

The API uses Neon PostgreSQL through its standard pooled connection string. Copy `.env.example` to a local, untracked configuration source and provide:

```text
DATABASE_URL=postgresql://...
ARGUS_ENCRYPTION_KEY=<base64-encoded 32-byte key>
GITHUB_CLIENT_ID=Iv23ctiIuQdTod21RSVo
```

Generate a development encryption key with `openssl rand -base64 32`. Variable values are encrypted with AES-256-GCM before they reach PostgreSQL; the environment and variable identifiers are authenticated as encryption context. Never reuse a production encryption key in development.

Start the API after exporting the variables into the process environment:

```bash
go run ./cmd/argus-api
```

For local development the API also reads an untracked `.env` without overriding variables already supplied by the process environment.

The API applies each embedded migration transactionally, records it in `schema_migrations`, and exposes `GET /health`.

## API behavior

- GitHub App Device Flow creates or updates the user and returns a 30-day Argus session.
- Only SHA-256 hashes of Argus session tokens are stored.
- Project access is checked through project membership on every operation.
- Invitations are matched case-insensitively to the authenticated GitHub username, expire after seven days, and require explicit acceptance.
- Viewer, member, admin, and owner permissions are enforced by the API for every protected mutation.
- Variable values are encrypted before insertion using unique AES-GCM nonces.
- Push makes the remote environment an exact mirror of the local `.env`; remote-only keys are removed transactionally and recorded in activity history.
- Activity records variable additions and changes without recording values.
- Destroying a project is restricted to its owner and cascades through its environments and activity.

## Verification

Run the normal suite:

```bash
make check
```

Run the complete store lifecycle against an isolated PostgreSQL or Neon branch:

```bash
DATABASE_URL=postgresql://... go test -tags=integration ./internal/store
```

Build production binaries with `make build`, or build the API container with:

```bash
docker build -t argus-api .
```

## Deploying the API to Vercel

Argus includes a single Go Function in `api/index.go`. `vercel.json` routes `/health` and every `/v1/*` request to that function while preserving the route expected by the HTTP server. The function runs in Vercel's Singapore region (`sin1`) near the Neon database.

Import the GitHub repository into Vercel with these settings:

- Framework preset: **Other**
- Root directory: repository root
- Build command: leave empty
- Output directory: leave empty

Add these environment variables to Production and Preview:

```text
DATABASE_URL
ARGUS_ENCRYPTION_KEY
GITHUB_CLIENT_ID
```

Do not add `ARGUS_API_ADDRESS`; Vercel invokes the exported handler directly. The GitHub Device Flow does not require `GITHUB_CLIENT_SECRET`.

After the first deployment, add `api.argus.eshayat.com` in the Vercel project's Domains settings and configure the DNS record Vercel provides. Verify the deployment with:

```bash
curl https://api.argus.eshayat.com/health
```

Vercel cold starts reuse a pool within each warm instance, limit that pool to three Neon connections, and serialize migrations using a PostgreSQL advisory lock. Override the per-instance limit only when necessary with `ARGUS_DB_MAX_CONNS`.
