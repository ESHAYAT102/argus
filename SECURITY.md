# Security

Argus stores high-value credentials. Please report vulnerabilities privately to the project owner rather than opening a public issue.

## Operational requirements

- Keep `DATABASE_URL`, `ARGUS_ENCRYPTION_KEY`, GitHub private keys, and webhook secrets in encrypted deployment secret storage.
- Use a unique 32-byte production encryption key and retain it for as long as encrypted values exist.
- Restrict the Neon database role to the Argus service.
- Never log request bodies, environment values, session tokens, or authorization headers.
- Rotate any credential that appears in source control, issue trackers, chat, screenshots, or logs.

GitHub's Device Flow uses the public `GITHUB_CLIENT_ID`; it does not require a client secret.
