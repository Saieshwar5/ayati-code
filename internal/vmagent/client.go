package vmagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

// Client is the controller-side data-plane client for vmagent.
type Client struct {
	endpoint  string
	authToken string
	client    *http.Client
}

// NewClient builds a vmagent client for a dedicated microVM endpoint.
func NewClient(endpoint, authToken string) (*Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("vmagent endpoint is required")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	return &Client{endpoint: strings.TrimSuffix(endpoint, "/"), authToken: authToken, client: &http.Client{}}, nil
}

// Exec posts one shell command and decodes the bounded ShellResult.
func (c *Client) Exec(ctx context.Context, command string) (exec.ShellResult, error) {
	var result exec.ShellResult
	body, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return result, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/exec", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	c.addAuth(request)
	response, err := c.client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return result, fmt.Errorf("vmagent exec status %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

// Bootstrap uploads a gzip tar stream.
func (c *Client) Bootstrap(ctx context.Context, reader io.Reader) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/bootstrap", reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/gzip")
	c.addAuth(request)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("vmagent bootstrap status %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// FetchTar returns the working tree as a gzip tar reader.
func (c *Client) FetchTar(ctx context.Context) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/v1/tar", nil)
	if err != nil {
		return nil, err
	}
	c.addAuth(request)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("vmagent tar status %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	return response.Body, nil
}

func (c *Client) addAuth(request *http.Request) {
	if c.authToken != "" {
		request.Header.Set("X-aws-proxy-auth", c.authToken)
	}
}
