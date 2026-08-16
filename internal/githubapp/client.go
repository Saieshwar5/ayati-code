package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	githubURL = "https://github.com"
	apiURL    = "https://api.github.com"
	maxBody   = 4 << 20
)

type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

type CreateRepositoryInput struct {
	Name        string
	Description string
	Private     bool
}

type APIError struct {
	StatusCode int
	Status     string
}

func (e APIError) Error() string { return "GitHub returned " + e.Status }

type Branch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

type Client struct {
	clientID, clientSecret, redirectURL string
	githubURL, apiURL                   string
	httpClient                          *http.Client
}

func New(clientID, clientSecret, redirectURL string) (*Client, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" || strings.TrimSpace(redirectURL) == "" {
		return nil, errors.New("GitHub client ID, client secret, and callback URL are required")
	}
	return &Client{
		clientID: strings.TrimSpace(clientID), clientSecret: strings.TrimSpace(clientSecret),
		redirectURL: strings.TrimSpace(redirectURL), githubURL: githubURL, apiURL: apiURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) AuthorizeURL(state string) string {
	values := url.Values{
		"client_id":    {c.clientID},
		"redirect_uri": {c.redirectURL},
		"state":        {state},
	}
	return c.githubURL + "/login/oauth/authorize?" + values.Encode()
}

func (c *Client) Exchange(ctx context.Context, code string) (string, error) {
	values := url.Values{
		"client_id": {c.clientID}, "client_secret": {c.clientSecret},
		"code": {strings.TrimSpace(code)}, "redirect_uri": {c.redirectURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.githubURL+"/login/oauth/access_token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	if err := c.do(request, &response); err != nil {
		return "", fmt.Errorf("exchange GitHub authorization: %w", err)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", fmt.Errorf("GitHub authorization returned no token: %s", response.Error)
	}
	return response.AccessToken, nil
}

func (c *Client) CurrentUser(ctx context.Context, token string) (User, error) {
	var user User
	if err := c.api(ctx, token, http.MethodGet, "/user", nil, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (c *Client) Repositories(ctx context.Context, token string) ([]Repository, error) {
	var installations struct {
		Installations []struct {
			ID int64 `json:"id"`
		} `json:"installations"`
	}
	if err := c.api(ctx, token, http.MethodGet, "/user/installations?per_page=100", nil, &installations); err != nil {
		return nil, err
	}
	byID := make(map[int64]Repository)
	for _, installation := range installations.Installations {
		var page struct {
			Repositories []Repository `json:"repositories"`
		}
		path := "/user/installations/" + strconv.FormatInt(installation.ID, 10) + "/repositories?per_page=100"
		if err := c.api(ctx, token, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		for _, repository := range page.Repositories {
			byID[repository.ID] = repository
		}
	}
	values := make([]Repository, 0, len(byID))
	for _, repository := range byID {
		values = append(values, repository)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].FullName < values[j].FullName })
	return values, nil
}

func (c *Client) CreateRepository(
	ctx context.Context, token string, input CreateRepositoryInput,
) (Repository, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateRepositoryName(input.Name); err != nil {
		return Repository{}, err
	}
	if len(input.Description) > 350 {
		return Repository{}, errors.New("repository description must be 350 characters or fewer")
	}
	request := map[string]any{
		"name": input.Name, "description": input.Description,
		"private": input.Private, "auto_init": true,
	}
	var repository Repository
	if err := c.api(ctx, token, http.MethodPost, "/user/repos", request, &repository); err != nil {
		return Repository{}, fmt.Errorf("create GitHub repository: %w", err)
	}
	if strings.TrimSpace(repository.FullName) == "" || strings.TrimSpace(repository.CloneURL) == "" ||
		strings.TrimSpace(repository.DefaultBranch) == "" {
		return Repository{}, errors.New("GitHub created a repository without complete clone information")
	}
	return repository, nil
}

func (c *Client) Branches(ctx context.Context, token, repository string) ([]Branch, error) {
	path, err := repositoryPath(repository)
	if err != nil {
		return nil, err
	}
	var branches []Branch
	if err := c.api(ctx, token, http.MethodGet, "/repos/"+path+"/branches?per_page=100", nil, &branches); err != nil {
		return nil, err
	}
	return branches, nil
}

func (c *Client) CreatePullRequest(
	ctx context.Context, token, repository, base, head, title, body string,
) (PullRequest, error) {
	path, err := repositoryPath(repository)
	if err != nil {
		return PullRequest{}, err
	}
	request := map[string]any{
		"base": base, "head": head, "title": strings.TrimSpace(title),
		"body": strings.TrimSpace(body), "draft": true,
	}
	var pull PullRequest
	err = c.api(ctx, token, http.MethodPost, "/repos/"+path+"/pulls", request, &pull)
	return pull, err
}

func (c *Client) api(ctx context.Context, token, method, path string, body, output any) error {
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, input)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return c.do(request, output)
}

func (c *Client) do(request *http.Request, output any) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(data) > maxBody {
		return errors.New("GitHub response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return APIError{StatusCode: response.StatusCode, Status: response.Status}
	}
	if len(data) == 0 || output == nil {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func validateRepositoryName(value string) error {
	if value == "" || len(value) > 100 {
		return errors.New("repository name must be between 1 and 100 characters")
	}
	if value == "." || value == ".." {
		return errors.New("repository name is invalid")
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("-_.", character) {
			continue
		}
		return errors.New("repository name may contain only letters, numbers, periods, hyphens, and underscores")
	}
	return nil
}

func repositoryPath(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("GitHub repository must be owner/name")
	}
	return url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}
