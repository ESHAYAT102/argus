package argus

import "embed"

// Migrations contains the ordered PostgreSQL migrations used by the Argus API.
//
//go:embed migrations/*.sql
var Migrations embed.FS
