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

type Member struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Invitation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Project   string    `json:"project"`
	Inviter   string    `json:"inviter"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
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
	Inspect(ctx context.Context, projectID, environment string) (map[string]string, error)
	Set(ctx context.Context, projectID, environment, name, value string) error
	DeleteVariable(ctx context.Context, projectID, environment, name string) error
	List(ctx context.Context) ([]Project, error)
	History(ctx context.Context, projectID string) ([]Activity, error)
	RemoveEnvironment(ctx context.Context, projectID, environment string) error
	RenameEnvironment(ctx context.Context, projectID, environment, newName string) error
	DestroyProject(ctx context.Context, projectID string) error
	RenameProject(ctx context.Context, projectID, newName string) error
	ShareProject(ctx context.Context, projectID, username, role string) (Invitation, error)
	ShareProjects(ctx context.Context, projectID string, usernames []string, role string) ([]Invitation, error)
	Members(ctx context.Context, projectID string) ([]Member, error)
	UpdateMemberRole(ctx context.Context, projectID, username, role string) error
	RemoveMember(ctx context.Context, projectID, username string) error
	Invitations(ctx context.Context) ([]Invitation, error)
	AcceptInvitation(ctx context.Context, invitationID string) error
	DeclineInvitation(ctx context.Context, invitationID string) error
}
