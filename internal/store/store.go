package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/argus-env/argus/internal/secrets"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db     *pgxpool.Pool
	cipher *secrets.Cipher
}

func New(db *pgxpool.Pool, cipher *secrets.Cipher) *Store { return &Store{db: db, cipher: cipher} }

func (store *Store) CreateSession(ctx context.Context, githubID int64, login string) (string, error) {
	var userID string
	err := store.db.QueryRow(ctx, `
		INSERT INTO users (github_id, github_login) VALUES ($1, $2)
		ON CONFLICT (github_id) DO UPDATE SET github_login = EXCLUDED.github_login, updated_at = now()
		RETURNING id`, githubID, login).Scan(&userID)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "arg_" + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	_, err = store.db.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, hash[:], time.Now().Add(30*24*time.Hour))
	return token, err
}

func (store *Store) Authenticate(ctx context.Context, token string) (string, error) {
	hash := sha256.Sum256([]byte(token))
	var userID string
	err := store.db.QueryRow(ctx, `SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > now()`, hash[:]).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (store *Store) CurrentUser(ctx context.Context, userID string) (api.User, error) {
	var user api.User
	err := store.db.QueryRow(ctx, `SELECT id, github_login FROM users WHERE id=$1`, userID).Scan(&user.ID, &user.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, ErrNotFound
	}
	return user, err
}

func (store *Store) Logout(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	_, err := store.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash[:])
	return err
}

func (store *Store) InitProject(ctx context.Context, userID string, request api.InitProjectRequest) (api.Project, api.Environment, error) {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return api.Project{}, api.Environment{}, err
	}
	defer tx.Rollback(ctx)
	var projectID string
	err = tx.QueryRow(ctx, `INSERT INTO projects (owner_id, name, repository) VALUES ($1,$2,NULLIF($3,'')) RETURNING id`, userID, request.Name, request.Repository).Scan(&projectID)
	if err != nil {
		return api.Project{}, api.Environment{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_members (project_id,user_id,role) VALUES ($1,$2,'owner')`, projectID, userID); err != nil {
		return api.Project{}, api.Environment{}, err
	}
	var environmentID string
	if err = tx.QueryRow(ctx, `INSERT INTO environments (project_id,name) VALUES ($1,$2) RETURNING id`, projectID, request.Environment).Scan(&environmentID); err != nil {
		return api.Project{}, api.Environment{}, err
	}
	for name, value := range request.Variables {
		existed, putErr := store.putVariable(ctx, tx, userID, environmentID, name, value)
		if putErr != nil {
			err = putErr
			return api.Project{}, api.Environment{}, err
		}
		action := "variable.added"
		if existed {
			action = "variable.changed"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,variable_name) VALUES($1,$2,$3,$4,$5)`, projectID, environmentID, userID, action, name); err != nil {
			return api.Project{}, api.Environment{}, err
		}
	}
	metadata, _ := json.Marshal(map[string]any{"variables": len(request.Variables)})
	if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,metadata) VALUES($1,$2,$3,'project.initialized',$4)`, projectID, environmentID, userID, metadata); err != nil {
		return api.Project{}, api.Environment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return api.Project{}, api.Environment{}, err
	}
	return api.Project{ID: projectID, Name: request.Name, Repository: request.Repository}, api.Environment{ID: environmentID, ProjectID: projectID, Name: request.Environment}, nil
}

func (store *Store) EnvironmentExists(ctx context.Context, userID, projectID, name string) (bool, error) {
	var exists bool
	err := store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments e JOIN project_members m ON m.project_id=e.project_id WHERE e.project_id=$1 AND e.name=$2 AND m.user_id=$3)`, projectID, name, userID).Scan(&exists)
	return exists, err
}

func (store *Store) Push(ctx context.Context, userID, projectID, environment string, values map[string]string) (api.Environment, error) {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return api.Environment{}, err
	}
	defer tx.Rollback(ctx)
	if err = authorize(ctx, tx, userID, projectID); err != nil {
		return api.Environment{}, err
	}
	var existed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM environments WHERE project_id=$1 AND name=$2)`, projectID, environment).Scan(&existed); err != nil {
		return api.Environment{}, err
	}
	var environmentID string
	err = tx.QueryRow(ctx, `INSERT INTO environments(project_id,name) VALUES($1,$2) ON CONFLICT(project_id,name) DO UPDATE SET updated_at=now() RETURNING id`, projectID, environment).Scan(&environmentID)
	if err != nil {
		return api.Environment{}, err
	}
	if !existed {
		if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action) VALUES($1,$2,$3,'environment.created')`, projectID, environmentID, userID); err != nil {
			return api.Environment{}, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT name FROM variables WHERE environment_id=$1`, environmentID)
	if err != nil {
		return api.Environment{}, err
	}
	remoteNames := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return api.Environment{}, err
		}
		remoteNames = append(remoteNames, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return api.Environment{}, err
	}
	rows.Close()
	for _, name := range remoteNames {
		if _, keep := values[name]; keep {
			continue
		}
		if _, err = tx.Exec(ctx, `DELETE FROM variables WHERE environment_id=$1 AND name=$2`, environmentID, name); err != nil {
			return api.Environment{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,variable_name) VALUES($1,$2,$3,'variable.removed',$4)`, projectID, environmentID, userID, name); err != nil {
			return api.Environment{}, err
		}
	}
	for name, value := range values {
		existed, putErr := store.putVariable(ctx, tx, userID, environmentID, name, value)
		if putErr != nil {
			return api.Environment{}, putErr
		}
		action := "variable.added"
		if existed {
			action = "variable.changed"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,variable_name) VALUES($1,$2,$3,$4,$5)`, projectID, environmentID, userID, action, name); err != nil {
			return api.Environment{}, err
		}
	}
	metadata, _ := json.Marshal(map[string]any{"variables": len(values)})
	if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,metadata) VALUES($1,$2,$3,'environment.pushed',$4)`, projectID, environmentID, userID, metadata); err != nil {
		return api.Environment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return api.Environment{}, err
	}
	return api.Environment{ID: environmentID, ProjectID: projectID, Name: environment}, nil
}

func (store *Store) Get(ctx context.Context, userID, projectID, environment string, recordActivity bool) (map[string]string, error) {
	rows, err := store.db.Query(ctx, `SELECT e.id,v.name,v.encrypted_value,v.nonce FROM environments e JOIN project_members m ON m.project_id=e.project_id LEFT JOIN variables v ON v.environment_id=e.id WHERE e.project_id=$1 AND e.name=$2 AND m.user_id=$3`, projectID, environment, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]string{}
	found := false
	for rows.Next() {
		found = true
		var environmentID string
		var name *string
		var encrypted, nonce []byte
		if err := rows.Scan(&environmentID, &name, &encrypted, &nonce); err != nil {
			return nil, err
		}
		if name == nil {
			continue
		}
		plain, err := store.cipher.Decrypt(encrypted, nonce, []byte(environmentID+":"+*name))
		if err != nil {
			return nil, err
		}
		values[*name] = string(plain)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	if recordActivity {
		_, err = store.db.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action) SELECT $1,id,$3,'environment.pulled' FROM environments WHERE project_id=$1 AND name=$2`, projectID, environment, userID)
	}
	return values, err
}

func (store *Store) Set(ctx context.Context, userID, projectID, environment, name, value string) error {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var environmentID string
	err = tx.QueryRow(ctx, `SELECT e.id FROM environments e JOIN project_members m ON m.project_id=e.project_id WHERE e.project_id=$1 AND e.name=$2 AND m.user_id=$3`, projectID, environment, userID).Scan(&environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	existed, err := store.putVariable(ctx, tx, userID, environmentID, name, value)
	if err != nil {
		return err
	}
	action := "variable.added"
	if existed {
		action = "variable.changed"
	}
	_, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,variable_name) VALUES($1,$2,$3,$4,$5)`, projectID, environmentID, userID, action, name)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *Store) DeleteVariable(ctx context.Context, userID, projectID, environment, name string) error {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var environmentID string
	err = tx.QueryRow(ctx, `SELECT e.id FROM environments e JOIN project_members m ON m.project_id=e.project_id WHERE e.project_id=$1 AND e.name=$2 AND m.user_id=$3`, projectID, environment, userID).Scan(&environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM variables WHERE environment_id=$1 AND name=$2`, environmentID, name)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,variable_name) VALUES($1,$2,$3,'variable.removed',$4)`, projectID, environmentID, userID, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *Store) List(ctx context.Context, userID string) ([]api.Project, error) {
	rows, err := store.db.Query(ctx, `SELECT p.id,p.name,COALESCE(p.repository,''),e.id,e.name FROM projects p JOIN project_members m ON m.project_id=p.id LEFT JOIN environments e ON e.project_id=p.id WHERE m.user_id=$1 ORDER BY lower(p.name),lower(e.name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*api.Project{}
	order := []string{}
	for rows.Next() {
		var projectID, name, repo string
		var environmentID, environmentName *string
		if err := rows.Scan(&projectID, &name, &repo, &environmentID, &environmentName); err != nil {
			return nil, err
		}
		item := byID[projectID]
		if item == nil {
			item = &api.Project{ID: projectID, Name: name, Repository: repo}
			byID[projectID] = item
			order = append(order, projectID)
		}
		if environmentID != nil {
			item.Environments = append(item.Environments, api.Environment{ID: *environmentID, ProjectID: projectID, Name: *environmentName})
		}
	}
	projects := make([]api.Project, 0, len(order))
	for _, id := range order {
		projects = append(projects, *byID[id])
	}
	return projects, rows.Err()
}

func (store *Store) History(ctx context.Context, userID, projectID string) ([]api.Activity, error) {
	query := `SELECT a.id,a.action,u.github_login,COALESCE(e.name,a.metadata->>'environment',''),COALESCE(a.variable_name,''),a.created_at FROM activity_events a JOIN project_members m ON m.project_id=a.project_id JOIN users u ON u.id=a.actor_id LEFT JOIN environments e ON e.id=a.environment_id WHERE m.user_id=$1`
	args := []any{userID}
	if projectID != "" {
		query += " AND a.project_id=$2"
		args = append(args, projectID)
	}
	query += " ORDER BY a.created_at DESC"
	rows, err := store.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []api.Activity{}
	for rows.Next() {
		var event api.Activity
		if err := rows.Scan(&event.ID, &event.Action, &event.Actor, &event.Environment, &event.Variable, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (store *Store) RemoveEnvironment(ctx context.Context, userID, projectID, environment string) error {
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = authorize(ctx, tx, userID, projectID); err != nil {
		return err
	}
	var environmentID string
	err = tx.QueryRow(ctx, `SELECT id FROM environments WHERE project_id=$1 AND name=$2`, projectID, environment).Scan(&environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO activity_events(project_id,environment_id,actor_id,action,metadata) VALUES($1,$2,$3,'environment.removed',jsonb_build_object('environment',$4::text))`, projectID, environmentID, userID, environment); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM environments WHERE id=$1`, environmentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *Store) DestroyProject(ctx context.Context, userID, projectID string) error {
	result, err := store.db.Exec(ctx, `DELETE FROM projects WHERE id=$1 AND owner_id=$2`, projectID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *Store) putVariable(ctx context.Context, tx pgx.Tx, userID, environmentID, name, value string) (bool, error) {
	var existed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM variables WHERE environment_id=$1 AND name=$2)`, environmentID, name).Scan(&existed); err != nil {
		return false, err
	}
	encrypted, nonce, err := store.cipher.Encrypt([]byte(value), []byte(environmentID+":"+name))
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO variables(environment_id,name,encrypted_value,nonce,updated_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(environment_id,name) DO UPDATE SET encrypted_value=EXCLUDED.encrypted_value,nonce=EXCLUDED.nonce,updated_by=EXCLUDED.updated_by,updated_at=now()`, environmentID, name, encrypted, nonce, userID)
	return existed, err
}

func authorize(ctx context.Context, tx pgx.Tx, userID, projectID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, userID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func SortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
