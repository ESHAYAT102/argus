package api

import (
	"context"
	"time"
)

type Project struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Repository   string        `json:"repository,omitempty"`
	Environments []Environment `json:"environments,omitempty"`
}

type Environment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

type Activity struct {
	ID          string    `json:"id"`
	Action      string    `json:"action"`
	Actor       string    `json:"actor"`
	Environment string    `json:"environment,omitempty"`
	Variable    string    `json:"variable,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type InitProjectRequest struct {
	Name        string            `json:"name"`
	Repository  string            `json:"repository,omitempty"`
	Environment string            `json:"environment"`
	Variables   map[string]string `json:"variables"`
}

type Client interface {
	Authenticate(ctx context.Context) error
	WhoAmI(ctx context.Context) (User, error)
	Logout(ctx context.Context) error
	InitProject(ctx context.Context, request InitProjectRequest) (Project, Environment, error)
	Push(ctx context.Context, projectID, environment string, variables map[string]string) (Environment, error)
	EnvironmentExists(ctx context.Context, projectID, environment string) (bool, error)
	Get(ctx context.Context, projectID, environment string) (map[string]string, error)
	Set(ctx context.Context, projectID, environment, name, value string) error
	List(ctx context.Context) ([]Project, error)
	History(ctx context.Context, projectID string) ([]Activity, error)
	RemoveEnvironment(ctx context.Context, projectID, environment string) error
	DestroyProject(ctx context.Context, projectID string) error
}
