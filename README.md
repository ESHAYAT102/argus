# Argus

Argus is a Go CLI for managing named sets of environment variables across GitHub repositories and local projects.

The repository contains both the `argus` CLI and its Neon-backed API service.

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
