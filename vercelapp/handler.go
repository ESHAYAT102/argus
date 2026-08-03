// Package vercelapp adapts the Argus HTTP server to Vercel's Go runtime.
package vercelapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/argus-env/argus/internal/database"
	"github.com/argus-env/argus/internal/githubauth"
	"github.com/argus-env/argus/internal/secrets"
	"github.com/argus-env/argus/internal/server"
	"github.com/argus-env/argus/internal/store"
)

var runtime struct {
	sync.Mutex
	handler http.Handler
}

// Handler serves a Vercel invocation. Rewrites pass the original public route
// through __argus_path because the function itself is mounted at /api.
func Handler(writer http.ResponseWriter, request *http.Request) {
	handler, err := application(request.Context())
	if err != nil {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(writer).Encode(map[string]string{"error": "Argus is temporarily unavailable"})
		return
	}

	path, err := routedPath(request)
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	request.URL.Path = path
	query := request.URL.Query()
	query.Del("__argus_path")
	request.URL.RawQuery = query.Encode()
	handler.ServeHTTP(writer, request)
}

// application initializes once per warm Vercel function instance. Failed
// initialization is retried on the next invocation rather than poisoning the
// instance permanently.
func application(ctx context.Context) (http.Handler, error) {
	runtime.Lock()
	defer runtime.Unlock()
	if runtime.handler != nil {
		return runtime.handler, nil
	}

	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	cipher, err := secrets.New(os.Getenv("ARGUS_ENCRYPTION_KEY"))
	if err != nil {
		pool.Close()
		return nil, err
	}
	github, err := githubauth.New(os.Getenv("GITHUB_CLIENT_ID"))
	if err != nil {
		pool.Close()
		return nil, err
	}

	data := store.New(pool, cipher)
	runtime.handler = server.New(pool, data, github)
	return runtime.handler, nil
}

func routedPath(request *http.Request) (string, error) {
	path := strings.TrimSpace(request.URL.Query().Get("__argus_path"))
	if path == "" {
		return "", errors.New("missing routed path")
	}
	path = "/" + strings.TrimLeft(path, "/")
	if path != "/health" && !strings.HasPrefix(path, "/v1/") {
		return "", errors.New("invalid routed path")
	}
	return path, nil
}
