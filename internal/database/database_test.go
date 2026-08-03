package database

import "testing"

func TestMaxConnections(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("ARGUS_DB_MAX_CONNS", "")
	if got := maxConnections(); got != 3 {
		t.Fatalf("Vercel max connections = %d, want 3", got)
	}
	t.Setenv("ARGUS_DB_MAX_CONNS", "2")
	if got := maxConnections(); got != 2 {
		t.Fatalf("configured max connections = %d, want 2", got)
	}
	t.Setenv("ARGUS_DB_MAX_CONNS", "invalid")
	if got := maxConnections(); got != 3 {
		t.Fatalf("invalid override = %d, want Vercel default 3", got)
	}
}
