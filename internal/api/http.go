package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotFound = errors.New("resource not found")
var ErrUnauthenticated = errors.New("not authenticated")

type TokenStore interface {
	Load() (string, error)
	Save(string) error
	Clear() error
}

type HTTPClient struct {
	baseURL string
	http    *http.Client
	tokens  TokenStore
}

func NewHTTPClient(baseURL string, tokens TokenStore) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
		tokens:  tokens,
	}
}

type deviceAuth struct {
	VerificationURI string `json:"verification_uri"`
	UserCode        string `json:"user_code"`
	DeviceCode      string `json:"device_code"`
	Interval        int    `json:"interval"`
}

func (client *HTTPClient) Authenticate(ctx context.Context) error {
	var device deviceAuth
	if err := client.request(ctx, http.MethodPost, "/v1/auth/github/device", nil, &device, false); err != nil {
		return err
	}
	fmt.Printf("Open %s and enter code %s\n", device.VerificationURI, device.UserCode)
	interval := time.Duration(device.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		var session struct {
			Token   string `json:"token"`
			Pending bool   `json:"pending"`
		}
		err := client.request(ctx, http.MethodPost, "/v1/auth/github/device/poll", map[string]string{"device_code": device.DeviceCode}, &session, false)
		if err != nil {
			return err
		}
		if session.Pending {
			continue
		}
		if session.Token == "" {
			return errors.New("authentication completed without a session token")
		}
		return client.tokens.Save(session.Token)
	}
}

func (client *HTTPClient) Logout(ctx context.Context) error {
	err := client.request(ctx, http.MethodPost, "/v1/auth/logout", nil, nil, true)
	clearErr := client.tokens.Clear()
	if clearErr != nil {
		return clearErr
	}
	// Logging out must always remove the local credential. An expired remote
	// session is already logged out for practical purposes.
	if err != nil && !errors.Is(err, ErrUnauthenticated) {
		return err
	}
	return nil
}

func (client *HTTPClient) WhoAmI(ctx context.Context) (User, error) {
	var user User
	err := client.request(ctx, http.MethodGet, "/v1/auth/me", nil, &user, true)
	return user, err
}

func (client *HTTPClient) InitProject(ctx context.Context, request InitProjectRequest) (Project, Environment, error) {
	var response struct {
		Project     Project     `json:"project"`
		Environment Environment `json:"environment"`
	}
	err := client.request(ctx, http.MethodPost, "/v1/projects", request, &response, true)
	return response.Project, response.Environment, err
}

func (client *HTTPClient) Push(ctx context.Context, projectID, environment string, variables map[string]string) (Environment, error) {
	var result Environment
	path := fmt.Sprintf("/v1/projects/%s/environments/%s/push", url.PathEscape(projectID), url.PathEscape(environment))
	err := client.request(ctx, http.MethodPut, path, map[string]any{"variables": variables}, &result, true)
	return result, err
}

func (client *HTTPClient) EnvironmentExists(ctx context.Context, projectID, environment string) (bool, error) {
	path := fmt.Sprintf("/v1/projects/%s/environments/%s", url.PathEscape(projectID), url.PathEscape(environment))
	err := client.request(ctx, http.MethodGet, path, nil, nil, true)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (client *HTTPClient) Get(ctx context.Context, projectID, environment string) (map[string]string, error) {
	var response struct {
		Variables map[string]string `json:"variables"`
	}
	path := fmt.Sprintf("/v1/projects/%s/environments/%s/variables", url.PathEscape(projectID), url.PathEscape(environment))
	err := client.request(ctx, http.MethodGet, path, nil, &response, true)
	return response.Variables, err
}

func (client *HTTPClient) Set(ctx context.Context, projectID, environment, name, value string) error {
	path := fmt.Sprintf("/v1/projects/%s/environments/%s/variables/%s", url.PathEscape(projectID), url.PathEscape(environment), url.PathEscape(name))
	return client.request(ctx, http.MethodPut, path, map[string]string{"value": value}, nil, true)
}

func (client *HTTPClient) List(ctx context.Context) ([]Project, error) {
	var response struct {
		Projects []Project `json:"projects"`
	}
	err := client.request(ctx, http.MethodGet, "/v1/projects", nil, &response, true)
	return response.Projects, err
}

func (client *HTTPClient) History(ctx context.Context, projectID string) ([]Activity, error) {
	var response struct {
		Activity []Activity `json:"activity"`
	}
	path := "/v1/activity"
	if projectID != "" {
		path += "?project_id=" + url.QueryEscape(projectID)
	}
	err := client.request(ctx, http.MethodGet, path, nil, &response, true)
	return response.Activity, err
}

func (client *HTTPClient) RemoveEnvironment(ctx context.Context, projectID, environment string) error {
	path := fmt.Sprintf("/v1/projects/%s/environments/%s", url.PathEscape(projectID), url.PathEscape(environment))
	return client.request(ctx, http.MethodDelete, path, nil, nil, true)
}

func (client *HTTPClient) DestroyProject(ctx context.Context, projectID string) error {
	return client.request(ctx, http.MethodDelete, "/v1/projects/"+url.PathEscape(projectID), nil, nil, true)
}

func (client *HTTPClient) request(ctx context.Context, method, path string, input, output any, authenticated bool) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		token, err := client.tokens.Load()
		if err != nil {
			return ErrUnauthenticated
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("contact Argus: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode == http.StatusUnauthorized {
		return ErrUnauthenticated
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem)
		if problem.Error == "" {
			problem.Error = response.Status
		}
		return errors.New(problem.Error)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 10<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode Argus response: %w", err)
	}
	return nil
}
