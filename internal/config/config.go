// Package config loads the skylight-mcp YAML configuration.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Skylight SkylightConfig `yaml:"skylight"`
}

type ServerConfig struct {
	// Listen is the address the streamable HTTP server binds to, e.g. ":8080".
	Listen string `yaml:"listen"`
	// Path is the HTTP path the MCP endpoint is served on, e.g. "/mcp".
	Path string `yaml:"path"`
	// AuthToken, when set, requires clients to send "Authorization: Bearer <token>".
	AuthToken string `yaml:"auth_token"`
	// Stateless disables MCP session tracking (each request is independent).
	Stateless bool `yaml:"stateless"`
}

type SkylightConfig struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
	// FrameID is optional; it is discovered from /api/frames/calendar when empty.
	FrameID string `yaml:"frame_id"`
	// Timezone is an IANA name used for calendar queries and new events.
	Timezone string `yaml:"timezone"`
	// AuthMethod: "auto" (OAuth, then legacy), "oauth", or "legacy".
	AuthMethod string `yaml:"auth_method"`
	BaseURL    string `yaml:"base_url"`
	APIVersion string `yaml:"api_version"`
	UserAgent  string `yaml:"user_agent"`
	// TokenCache is a file where tokens are persisted between restarts ("" disables).
	TokenCache string `yaml:"token_cache"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	return &c, c.validate()
}

func (c *Config) applyDefaults() {
	def := func(s *string, v string) {
		if *s == "" {
			*s = v
		}
	}
	def(&c.Server.Listen, ":8080")
	def(&c.Server.Path, "/mcp")
	def(&c.Skylight.AuthMethod, "auto")
	def(&c.Skylight.BaseURL, "https://app.ourskylight.com")
	def(&c.Skylight.APIVersion, "2026-10-15")
	def(&c.Skylight.UserAgent, "SkylightMobile (web)")
	def(&c.Skylight.TokenCache, ".skylight-token.json")
	if v := os.Getenv("SKYLIGHT_EMAIL"); v != "" {
		c.Skylight.Email = v
	}
	if v := os.Getenv("SKYLIGHT_PASSWORD"); v != "" {
		c.Skylight.Password = v
	}
	if v := os.Getenv("SKYLIGHT_MCP_AUTH_TOKEN"); v != "" {
		c.Server.AuthToken = v
	}
}

func (c *Config) validate() error {
	if c.Skylight.Email == "" || c.Skylight.Password == "" {
		return errors.New("skylight.email and skylight.password are required")
	}
	switch c.Skylight.AuthMethod {
	case "auto", "oauth", "legacy":
	default:
		return fmt.Errorf("skylight.auth_method must be auto, oauth or legacy (got %q)", c.Skylight.AuthMethod)
	}
	return nil
}
