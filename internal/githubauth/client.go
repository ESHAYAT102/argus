package githubauth

import (
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

type Device struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type Client struct {
	clientID string
	http     *http.Client
	webURL   string
	apiURL   string
}

func New(clientID string) (*Client, error) {
	if clientID == "" {
		return nil, errors.New("GITHUB_CLIENT_ID is required")
	}
	return &Client{clientID: clientID, http: &http.Client{Timeout: 15 * time.Second}, webURL: "https://github.com", apiURL: "https://api.github.com"}, nil
}

func (client *Client) Start(ctx context.Context) (Device, error) {
	values := url.Values{"client_id": {client.clientID}}
	var device Device
	if err := client.form(ctx, client.webURL+"/login/device/code", values, &device); err != nil {
		return Device{}, err
	}
	return device, nil
}

// Poll returns pending=true while the user has not completed authorization.
func (client *Client) Poll(ctx context.Context, deviceCode string) (token string, pending bool, err error) {
	values := url.Values{
		"client_id":   {client.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	var response struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := client.form(ctx, client.webURL+"/login/oauth/access_token", values, &response); err != nil {
		return "", false, err
	}
	switch response.Error {
	case "authorization_pending", "slow_down":
		return "", true, nil
	case "":
		if response.AccessToken == "" {
			return "", false, errors.New("GitHub returned no access token")
		}
		return response.AccessToken, false, nil
	default:
		if response.Description == "" {
			response.Description = response.Error
		}
		return "", false, errors.New(response.Description)
	}
}

func (client *Client) User(ctx context.Context, token string) (User, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.apiURL+"/user", nil)
	if err != nil {
		return User{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.http.Do(request)
	if err != nil {
		return User{}, fmt.Errorf("fetch GitHub user: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("fetch GitHub user: %s", response.Status)
	}
	var user User
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&user); err != nil {
		return User{}, err
	}
	if user.ID == 0 || user.Login == "" {
		return User{}, errors.New("GitHub returned an incomplete user")
	}
	return user, nil
}

func (client *Client) form(ctx context.Context, endpoint string, values url.Values, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("contact GitHub: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub authentication: %s", response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}
