package skylight

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	oauthClientID    = "skylight-mobile"
	oauthRedirectURI = "https://ourskylight.com/welcome"
	oauthScope       = "everything"
)

// token is the persisted credential state.
type token struct {
	Method       string    `json:"method"` // "oauth" or "legacy"
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
}

func (t *token) valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return t.Expiry.IsZero() || time.Now().Add(60*time.Second).Before(t.Expiry)
}

func (t *token) authHeader() string {
	if t.Method == "legacy" {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(t.UserID+":"+t.AccessToken))
	}
	return "Bearer " + t.AccessToken
}

func (c *Client) loadCache() {
	if c.cfg.TokenCache == "" {
		return
	}
	b, err := os.ReadFile(c.cfg.TokenCache)
	if err != nil {
		return
	}
	var t token
	if json.Unmarshal(b, &t) == nil && t.AccessToken != "" {
		c.tok = &t
	}
}

func (c *Client) saveCache() {
	if c.cfg.TokenCache == "" || c.tok == nil {
		return
	}
	b, _ := json.MarshalIndent(c.tok, "", "  ")
	if err := os.WriteFile(c.cfg.TokenCache, b, 0o600); err != nil {
		c.log.Warn("could not write token cache", "err", err)
	}
}

// authorization returns a valid Authorization header value, logging in or
// refreshing as needed. force discards the current access token first.
func (c *Client) authorization(ctx context.Context, force bool) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if force && c.tok != nil {
		c.tok.Expiry = time.Unix(1, 0) // mark expired, keep refresh token
	}
	if c.tok.valid() {
		return c.tok.authHeader(), nil
	}
	if c.tok != nil && c.tok.Method == "oauth" && c.tok.RefreshToken != "" {
		if t, err := c.oauthRefresh(ctx, c.tok.RefreshToken); err == nil {
			c.tok = t
			c.saveCache()
			return t.authHeader(), nil
		} else {
			c.log.Info("token refresh failed; logging in again", "err", err)
		}
	}
	t, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	c.tok = t
	c.saveCache()
	return t.authHeader(), nil
}

func (c *Client) login(ctx context.Context) (*token, error) {
	var errs []error
	if c.cfg.AuthMethod == "auto" || c.cfg.AuthMethod == "oauth" {
		t, err := c.oauthLogin(ctx)
		if err == nil {
			c.log.Info("logged in to Skylight", "method", "oauth")
			return t, nil
		}
		errs = append(errs, fmt.Errorf("oauth: %w", err))
	}
	if c.cfg.AuthMethod == "auto" || c.cfg.AuthMethod == "legacy" {
		t, err := c.legacyLogin(ctx)
		if err == nil {
			c.log.Info("logged in to Skylight", "method", "legacy")
			return t, nil
		}
		errs = append(errs, fmt.Errorf("legacy: %w", err))
	}
	return nil, fmt.Errorf("skylight login failed: %w", errors.Join(errs...))
}

// legacyLogin uses POST /api/sessions and Basic user_id:token auth.
func (c *Client) legacyLogin(ctx context.Context) (*token, error) {
	body, _ := json.Marshal(map[string]string{"email": c.cfg.Email, "password": c.cfg.Password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/api/sessions", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, snippet(b))
	}
	var r struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Token string `json:"token"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.Data.Attributes.Token == "" {
		return nil, fmt.Errorf("unexpected response: %s", snippet(b))
	}
	return &token{Method: "legacy", AccessToken: r.Data.Attributes.Token, UserID: r.Data.ID}, nil
}

var (
	csrfMetaRe  = regexp.MustCompile(`<meta[^>]+name="csrf-token"[^>]+content="([^"]+)"`)
	hiddenInput = regexp.MustCompile(`<input[^>]*type="hidden"[^>]*>`)
	attrNameRe  = regexp.MustCompile(`name="([^"]*)"`)
	attrValueRe = regexp.MustCompile(`value="([^"]*)"`)
)

// oauthLogin performs the browser-style Authorization Code + PKCE flow headlessly.
func (c *Client) oauthLogin(ctx context.Context) (*token, error) {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	verifier := randString(64)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randString(24)

	authURL := c.cfg.BaseURL + "/oauth/authorize?" + url.Values{
		"client_id":             {oauthClientID},
		"response_type":         {"code"},
		"scope":                 {oauthScope},
		"redirect_uri":          {oauthRedirectURI},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	// Follow redirects manually until we land on an HTML page (the login form).
	page, pageURL, err := c.followToPage(ctx, hc, authURL)
	if err != nil {
		return nil, err
	}
	m := csrfMetaRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("csrf token not found on login page %s", pageURL)
	}
	form := url.Values{}
	for _, in := range hiddenInput.FindAll(page, -1) {
		n, v := attrNameRe.FindSubmatch(in), attrValueRe.FindSubmatch(in)
		if n != nil {
			val := ""
			if v != nil {
				val = html.UnescapeString(string(v[1]))
			}
			form.Set(html.UnescapeString(string(n[1])), val)
		}
	}
	form.Set("authenticity_token", html.UnescapeString(string(m[1])))
	form.Set("email", c.cfg.Email)
	form.Set("password", c.cfg.Password)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/auth/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", pageURL)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// Follow the redirect chain until it points at the redirect URI with a code.
	next := resp.Header.Get("Location")
	for i := 0; i < 10 && next != ""; i++ {
		u, err := resp.Request.URL.Parse(next)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(u.String(), oauthRedirectURI) {
			q := u.Query()
			if q.Get("state") != state {
				return nil, errors.New("oauth state mismatch")
			}
			if e := q.Get("error"); e != "" {
				return nil, fmt.Errorf("oauth error: %s %s", e, q.Get("error_description"))
			}
			return c.oauthExchange(ctx, url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {oauthClientID},
				"code":          {q.Get("code")},
				"redirect_uri":  {oauthRedirectURI},
				"code_verifier": {verifier},
			})
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		req.Header.Set("User-Agent", browserUA)
		resp, err = hc.Do(req)
		if err != nil {
			return nil, err
		}
		resp.Body.Close()
		next = resp.Header.Get("Location")
	}
	return nil, fmt.Errorf("login did not redirect to the app (status %d) — check email/password", resp.StatusCode)
}

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

func (c *Client) followToPage(ctx context.Context, hc *http.Client, start string) ([]byte, string, error) {
	cur := start
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cur, nil)
		req.Header.Set("User-Agent", browserUA)
		req.Header.Set("Accept", "text/html")
		resp, err := hc.Do(req)
		if err != nil {
			return nil, "", err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
			u, err := resp.Request.URL.Parse(loc)
			if err != nil {
				return nil, "", err
			}
			cur = u.String()
			continue
		}
		if resp.StatusCode != 200 {
			return nil, "", fmt.Errorf("GET %s: status %d", cur, resp.StatusCode)
		}
		return b, cur, nil
	}
	return nil, "", errors.New("too many redirects")
}

func (c *Client) oauthRefresh(ctx context.Context, refresh string) (*token, error) {
	return c.oauthExchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {oauthClientID},
		"refresh_token": {refresh},
	})
}

func (c *Client) oauthExchange(ctx context.Context, form url.Values) (*token, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token endpoint status %d: %s", resp.StatusCode, snippet(b))
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.AccessToken == "" {
		return nil, fmt.Errorf("unexpected token response: %s", snippet(b))
	}
	t := &token{Method: "oauth", AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	if t.RefreshToken == "" {
		t.RefreshToken = form.Get("refresh_token")
	}
	if r.ExpiresIn > 0 {
		t.Expiry = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	return t, nil
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}
