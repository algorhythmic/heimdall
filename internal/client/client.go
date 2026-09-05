// Package client accesses only the daemon's scoped API. It never opens its database.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MaxResponse = 512 << 10

type Credential struct {
	Version int           `json:"version"`
	DataDir string        `json:"data_dir"`
	Token   string        `json:"token"`
	Issue   authz.Request `json:"issue"`
}

func ReadCredential(path string) (Credential, error) {
	var c Credential
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return c, err
	}
	if len(raw) > 16384 {
		return c, fmt.Errorf("credential file too large")
	}
	if err = model.StrictJSON(raw, &c); err != nil {
		return c, err
	}
	if c.Version != 1 || !filepath.IsAbs(c.DataDir) || !model.TokenHashPattern.MatchString(c.Token) || c.Issue.Grant == nil || c.Issue.Grant.TokenHash != authz.HashToken(c.Token) {
		return c, fmt.Errorf("invalid credential file")
	}
	return c, nil
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Details json.RawMessage
}

func (e *APIError) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message) }

type Client struct{ credential Credential }

func New(path string) (*Client, error) {
	c, err := ReadCredential(path)
	if err != nil {
		return nil, err
	}
	return &Client{c}, nil
}
func (c *Client) Call(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/client/") {
		return nil, fmt.Errorf("scoped client route required")
	}
	raw, err := os.ReadFile(filepath.Join(c.credential.DataDir, "client-endpoint.json"))
	if err != nil {
		return nil, &APIError{Code: "daemon_unavailable", Message: "public daemon endpoint unavailable"}
	}
	var ep struct {
		URL   string `json:"url"`
		Token string `json:"token"`
		PID   int    `json:"pid"`
	}
	if len(raw) > 4096 || model.StrictJSON(raw, &ep) != nil {
		return nil, fmt.Errorf("invalid public daemon endpoint")
	}
	port := strings.TrimPrefix(ep.URL, "http://127.0.0.1:")
	n, err := strconv.Atoi(port)
	if !strings.HasPrefix(ep.URL, "http://127.0.0.1:") || err != nil || n < 1 || n > 65535 || ep.Token != "" {
		return nil, fmt.Errorf("invalid public daemon endpoint")
	}
	var reader io.Reader
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if len(raw) > 64<<10 {
			return nil, fmt.Errorf("request exceeds 64 KiB")
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, ep.URL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.credential.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	httpClient := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer httpClient.CloseIdleConnections()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &APIError{Code: "daemon_unavailable", Message: "daemon call failed; retry writes only with the same request ID"}
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, MaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxResponse {
		return nil, fmt.Errorf("daemon response exceeds 512 KiB")
	}
	if resp.StatusCode != 200 {
		var details struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &details)
		if details.Code == "" {
			details.Code = "daemon_error"
		}
		return nil, &APIError{resp.StatusCode, details.Code, details.Error, raw}
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid daemon response")
	}
	return json.RawMessage(bytes.TrimSpace(raw)), nil
}
