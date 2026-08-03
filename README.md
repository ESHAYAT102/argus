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
argus logout                       Log out
argus init [environment]           Create a project and push .env
argus sync [environment]           Push .env
argus get <environment>            Fetch into .env
argus set <variable> [value]       Update one variable
argus list                         List projects and environments
argus history                      Show activity
argus remove <environment>         Remove an environment
argus destroy [project]            Destroy a project
```

`ls`, `activity`, and `rm` are aliases for `list`, `history`, and `remove`.

## Safe fetches

When `argus get` encounters a non-empty `.env`, it moves the existing file to a timestamped backup before installing the fetched environment:

```text
.env.backup.20260804-143052
```

The replacement is written to a temporary owner-only file first. If installation fails, Argus attempts to restore the original file.

## Development

```bash
go test ./...
go run ./cmd/argus help
```

Set `ARGUS_API_URL` to point the CLI at a development API:

```bash
ARGUS_API_URL=http://localhost:8080 go run ./cmd/argus list
```

Local directory associations are stored in `.argus.toml`. It contains project identifiers and the active environment, never secret values.

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
- Variable values are encrypted before insertion using unique AES-GCM nonces.
- Sync is non-destructive: keys missing from the local `.env` are retained remotely.
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
