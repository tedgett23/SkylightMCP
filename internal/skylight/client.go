// Package skylight is a client for the (unofficial) Skylight Calendar API.
package skylight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"skylight-mcp/internal/config"
)

type Client struct {
	cfg  config.SkylightConfig
	http *http.Client
	log  *slog.Logger

	authMu sync.Mutex
	tok    *token

	frameMu  sync.Mutex
	frameID  string
	timezone string
}

func New(cfg config.SkylightConfig, log *slog.Logger) *Client {
	c := &Client{
		cfg:      cfg,
		http:     &http.Client{Timeout: 60 * time.Second},
		log:      log,
		frameID:  cfg.FrameID,
		timezone: cfg.Timezone,
	}
	c.loadCache()
	return c
}

// APIError is returned for non-2xx API responses.
type APIError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("skylight %s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Do performs an authenticated API request. path is relative to the base URL
// (e.g. "/api/user"). body is JSON-encoded unless nil. A 401 triggers one re-login.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	u := c.cfg.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	for attempt := 0; attempt < 2; attempt++ {
		auth, err := c.authorization(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", auth)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("Skylight-Api-Version", c.cfg.APIVersion)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.log.Info("got 401, re-authenticating", "path", path)
			continue
		}
		if resp.StatusCode >= 300 {
			return nil, &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: snippet(b)}
		}
		if len(bytes.TrimSpace(b)) == 0 {
			return json.RawMessage(`{"success":true}`), nil
		}
		return b, nil
	}
	return nil, fmt.Errorf("skylight %s %s: unauthorized after re-login", method, path)
}

// FramePath returns "/api/frames/<frameID><suffix>", discovering the frame if needed.
func (c *Client) FramePath(ctx context.Context, suffix string) (string, error) {
	id, err := c.FrameID(ctx)
	if err != nil {
		return "", err
	}
	return "/api/frames/" + id + suffix, nil
}

// FrameID returns the configured frame, or discovers the primary calendar frame.
func (c *Client) FrameID(ctx context.Context) (string, error) {
	c.frameMu.Lock()
	defer c.frameMu.Unlock()
	if c.frameID != "" {
		return c.frameID, nil
	}
	type frameResp struct {
		Data json.RawMessage `json:"data"`
	}
	var id, tz string
	pick := func(raw json.RawMessage) {
		var one struct {
			ID         string         `json:"id"`
			Attributes map[string]any `json:"attributes"`
		}
		var many []struct {
			ID         string         `json:"id"`
			Attributes map[string]any `json:"attributes"`
		}
		if json.Unmarshal(raw, &one) == nil && one.ID != "" {
			id = one.ID
			tz, _ = one.Attributes["timezone"].(string)
		} else if json.Unmarshal(raw, &many) == nil && len(many) > 0 {
			id = many[0].ID
			tz, _ = many[0].Attributes["timezone"].(string)
		}
	}
	for _, p := range []string{"/api/frames/calendar", "/api/frames"} {
		raw, err := c.Do(ctx, http.MethodGet, p, nil, nil)
		if err != nil {
			c.log.Warn("frame discovery failed", "path", p, "err", err)
			continue
		}
		var fr frameResp
		if json.Unmarshal(raw, &fr) == nil {
			pick(fr.Data)
		}
		if id != "" {
			break
		}
	}
	if id == "" {
		return "", fmt.Errorf("could not discover frame id; set skylight.frame_id in config")
	}
	c.frameID = id
	if c.timezone == "" && tz != "" {
		c.timezone = tz
	}
	c.log.Info("using frame", "frame_id", id)
	return id, nil
}

// Timezone returns the configured timezone, the frame's timezone, or "UTC".
func (c *Client) Timezone(ctx context.Context) string {
	if c.timezone == "" {
		_, _ = c.FrameID(ctx)
	}
	if c.timezone == "" {
		return "UTC"
	}
	return c.timezone
}

// CleanPath validates a caller-supplied API path for the raw request tool.
func CleanPath(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, "/api/") || strings.Contains(p, "..") || strings.ContainsAny(p, "?#") {
		return "", fmt.Errorf("path must start with /api/ and contain no query string (use the query argument)")
	}
	return p, nil
}
