package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/argus-env/argus/internal/dotenv"
	"github.com/argus-env/argus/internal/githubauth"
	"github.com/argus-env/argus/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const userKey contextKey = "user"

type Server struct {
	database *pgxpool.Pool
	store    *store.Store
	github   *githubauth.Client
	started  time.Time
}

func New(database *pgxpool.Pool, data *store.Store, github *githubauth.Client) http.Handler {
	server := &Server{database: database, store: data, github: github, started: time.Now()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("POST /v1/auth/github/device", server.startDevice)
	mux.HandleFunc("POST /v1/auth/github/device/poll", server.pollDevice)
	mux.Handle("GET /v1/auth/me", server.auth(http.HandlerFunc(server.me)))
	mux.Handle("POST /v1/auth/logout", server.auth(http.HandlerFunc(server.logout)))
	mux.Handle("POST /v1/projects", server.auth(http.HandlerFunc(server.initProject)))
	mux.Handle("GET /v1/projects", server.auth(http.HandlerFunc(server.listProjects)))
	mux.Handle("DELETE /v1/projects/{project}", server.auth(http.HandlerFunc(server.destroyProject)))
	mux.Handle("GET /v1/projects/{project}/environments/{environment}", server.auth(http.HandlerFunc(server.environmentExists)))
	mux.Handle("PUT /v1/projects/{project}/environments/{environment}/push", server.auth(http.HandlerFunc(server.push)))
	mux.Handle("GET /v1/projects/{project}/environments/{environment}/variables", server.auth(http.HandlerFunc(server.getVariables)))
	mux.Handle("PUT /v1/projects/{project}/environments/{environment}/variables/{variable}", server.auth(http.HandlerFunc(server.setVariable)))
	mux.Handle("DELETE /v1/projects/{project}/environments/{environment}/variables/{variable}", server.auth(http.HandlerFunc(server.deleteVariable)))
	mux.Handle("DELETE /v1/projects/{project}/environments/{environment}", server.auth(http.HandlerFunc(server.removeEnvironment)))
	mux.Handle("GET /v1/activity", server.auth(http.HandlerFunc(server.history)))
	return recoverer(securityHeaders(mux))
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := server.database.Ping(ctx); err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "uptime_seconds": int(time.Since(server.started).Seconds())})
}

func (server *Server) startDevice(writer http.ResponseWriter, request *http.Request) {
	device, err := server.github.Start(request.Context())
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, device)
}

func (server *Server) pollDevice(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		DeviceCode string `json:"device_code"`
	}
	if err := decode(request, &input); err != nil || input.DeviceCode == "" {
		writeError(writer, http.StatusBadRequest, "device_code is required")
		return
	}
	token, pending, err := server.github.Poll(request.Context(), input.DeviceCode)
	if err != nil {
		problem(writer, err)
		return
	}
	if pending {
		writeJSON(writer, http.StatusOK, map[string]bool{"pending": true})
		return
	}
	user, err := server.github.User(request.Context(), token)
	if err != nil {
		problem(writer, err)
		return
	}
	session, err := server.store.CreateSession(request.Context(), user.ID, user.Login)
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"token": session, "pending": false})
}

func (server *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if err := server.store.Logout(request.Context(), bearer(request)); err != nil {
		problem(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) me(writer http.ResponseWriter, request *http.Request) {
	user, err := server.store.CurrentUser(request.Context(), userID(request))
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, user)
}

func (server *Server) initProject(writer http.ResponseWriter, request *http.Request) {
	var input api.InitProjectRequest
	if err := decode(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateProject(input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	project, environment, err := server.store.InitProject(request.Context(), userID(request), input)
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"project": project, "environment": environment})
}

func (server *Server) listProjects(writer http.ResponseWriter, request *http.Request) {
	projects, err := server.store.List(request.Context(), userID(request))
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"projects": projects})
}

func (server *Server) environmentExists(writer http.ResponseWriter, request *http.Request) {
	exists, err := server.store.EnvironmentExists(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"))
	if err != nil {
		problem(writer, err)
		return
	}
	if !exists {
		writeError(writer, http.StatusNotFound, "environment not found")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) push(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Variables map[string]string `json:"variables"`
	}
	if err := decode(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if input.Variables == nil {
		writeError(writer, http.StatusBadRequest, "variables are required")
		return
	}
	for name := range input.Variables {
		if err := dotenv.ValidateName(name); err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
	}
	environment, err := server.store.Push(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"), input.Variables)
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, environment)
}

func (server *Server) getVariables(writer http.ResponseWriter, request *http.Request) {
	recordActivity := request.URL.Query().Get("record_activity") != "false"
	values, err := server.store.Get(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"), recordActivity)
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"variables": values})
}

func (server *Server) deleteVariable(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("variable")
	if err := dotenv.ValidateName(name); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := server.store.DeleteVariable(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"), name); err != nil {
		problem(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) setVariable(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("variable")
	if err := dotenv.ValidateName(name); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		Value string `json:"value"`
	}
	if err := decode(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	err := server.store.Set(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"), name, input.Value)
	if err != nil {
		problem(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) history(writer http.ResponseWriter, request *http.Request) {
	events, err := server.store.History(request.Context(), userID(request), request.URL.Query().Get("project_id"))
	if err != nil {
		problem(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"activity": events})
}

func (server *Server) removeEnvironment(writer http.ResponseWriter, request *http.Request) {
	err := server.store.RemoveEnvironment(request.Context(), userID(request), request.PathValue("project"), request.PathValue("environment"))
	if err != nil {
		problem(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) destroyProject(writer http.ResponseWriter, request *http.Request) {
	err := server.store.DestroyProject(request.Context(), userID(request), request.PathValue("project"))
	if err != nil {
		problem(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := bearer(request)
		if token == "" {
			writeError(writer, http.StatusUnauthorized, "authentication required")
			return
		}
		user, err := server.store.Authenticate(request.Context(), token)
		if err != nil {
			writeError(writer, http.StatusUnauthorized, "invalid or expired session; run `argus auth`")
			return
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), userKey, user)))
	})
}

func bearer(request *http.Request) string {
	header := request.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
func userID(request *http.Request) string {
	value, _ := request.Context().Value(userKey).(string)
	return value
}

func validateProject(input api.InitProjectRequest) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("project name is required")
	}
	if len(input.Name) > 100 {
		return errors.New("project name must be 100 characters or fewer")
	}
	if strings.TrimSpace(input.Environment) == "" {
		return errors.New("environment is required")
	}
	if len(input.Environment) > 100 {
		return errors.New("environment name must be 100 characters or fewer")
	}
	if input.Variables == nil {
		return errors.New("variables are required")
	}
	for name := range input.Variables {
		if err := dotenv.ValidateName(name); err != nil {
			return err
		}
	}
	return nil
}
func decode(request *http.Request, output any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 10<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func problem(writer http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(writer, http.StatusNotFound, "resource not found")
		return
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		writeError(writer, http.StatusConflict, "a project or environment with that name already exists")
		return
	}
	writeError(writer, http.StatusInternalServerError, "internal server error")
}
func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(writer, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
