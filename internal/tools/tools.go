// Package tools registers the Skylight MCP tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"skylight-mcp/internal/skylight"
)

type kind int

const (
	readOnly kind = iota
	write
	destructive
)

type reg struct {
	s *mcp.Server
	c *skylight.Client
}

// Register adds every tool to the server.
func Register(s *mcp.Server, c *skylight.Client) {
	r := &reg{s: s, c: c}
	r.general()
	r.calendar()
	r.profiles()
	r.chores()
	r.lists()
	r.rewards()
	r.meals()
	r.misc()
}

// add registers a tool whose handler returns any JSON-able value.
func add[In any](r *reg, name, desc string, k kind, fn func(ctx context.Context, in In) (any, error)) {
	f, t := false, true
	ann := &mcp.ToolAnnotations{OpenWorldHint: &t}
	switch k {
	case readOnly:
		ann.ReadOnlyHint = true
	case write:
		ann.DestructiveHint = &f
	case destructive:
		ann.DestructiveHint = &t
	}
	mcp.AddTool(r.s, &mcp.Tool{Name: name, Description: desc, Annotations: ann},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			out, err := fn(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			b, err := json.Marshal(out)
			if err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
		})
}

// call performs a frame-scoped request and returns the simplified response.
func (r *reg) call(ctx context.Context, method, suffix string, q url.Values, body any) (any, error) {
	p, err := r.c.FramePath(ctx, suffix)
	if err != nil {
		return nil, err
	}
	raw, err := r.c.Do(ctx, method, p, q, body)
	if err != nil {
		return nil, err
	}
	return skylight.Simplify(raw), nil
}

// global performs a non-frame request.
func (r *reg) global(ctx context.Context, method, path string, q url.Values, body any) (any, error) {
	raw, err := r.c.Do(ctx, method, path, q, body)
	if err != nil {
		return nil, err
	}
	return skylight.Simplify(raw), nil
}

// ---- body helpers ----

type body map[string]any

func put[T any](b body, k string, v *T) {
	if v != nil {
		b[k] = *v
	}
}

func putStr(b body, k, v string) {
	if v != "" {
		b[k] = v
	}
}

func putSlice[T any](b body, k string, v []T) {
	if v != nil {
		b[k] = v
	}
}

func (b body) merge(extra map[string]any) body {
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func seg(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/?#") || id == "." || id == ".." {
		return "", fmt.Errorf("invalid id %q", id)
	}
	return url.PathEscape(id), nil
}

func segs(ids ...string) ([]string, error) {
	out := make([]string, len(ids))
	for i, id := range ids {
		s, err := seg(id)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// filterByRel keeps only items whose relationship rel references id.
func filterByRel(v any, id string, rels ...string) any {
	m, ok := v.(map[string]any)
	if !ok || id == "" {
		return v
	}
	items, ok := m["data"].([]any)
	if !ok {
		return v
	}
	refID := func(x any) string {
		if xm, ok := x.(map[string]any); ok {
			s, _ := xm["id"].(string)
			return s
		}
		return ""
	}
	var keep []any
	for _, it := range items {
		im, _ := it.(map[string]any)
		match := false
		for _, rel := range rels {
			switch rv := im[rel].(type) {
			case []any:
				for _, x := range rv {
					if refID(x) == id {
						match = true
					}
				}
			default:
				if refID(rv) == id {
					match = true
				}
			}
		}
		if match {
			keep = append(keep, it)
		}
	}
	if keep == nil {
		keep = []any{}
	}
	m["data"] = keep
	m["count"] = len(keep)
	return m
}

// ---- general tools ----

type noInput struct{}

type rawIn struct {
	Method string            `json:"method" jsonschema:"HTTP method: GET, POST, PUT, PATCH or DELETE"`
	Path   string            `json:"path" jsonschema:"API path starting with /api/. The placeholder {frame_id} is replaced with the configured frame id, e.g. /api/frames/{frame_id}/nudges"`
	Query  map[string]string `json:"query,omitempty" jsonschema:"query string parameters"`
	Body   any               `json:"body,omitempty" jsonschema:"JSON request body"`
	Raw    bool              `json:"raw,omitempty" jsonschema:"return the unmodified JSON:API response instead of the simplified form"`
}

func (r *reg) general() {
	add(r, "skylight_overview",
		"Get a one-call overview of the Skylight household: frame details, family profiles (categories), lists, and synced source calendars. Use this first to learn the ids other tools need.",
		readOnly, func(ctx context.Context, _ noInput) (any, error) {
			out := map[string]any{}
			fid, err := r.c.FrameID(ctx)
			if err != nil {
				return nil, err
			}
			out["frame_id"] = fid
			out["timezone"] = r.c.Timezone(ctx)
			for k, suffix := range map[string]string{
				"frame": "", "profiles": "/categories", "lists": "/lists", "source_calendars": "/source_calendars",
			} {
				v, err := r.call(ctx, http.MethodGet, suffix, nil, nil)
				if err != nil {
					out[k] = map[string]string{"error": err.Error()}
					continue
				}
				out[k] = v
			}
			return out, nil
		})

	add(r, "skylight_api_request",
		"Escape hatch: send an arbitrary authenticated request to the Skylight API. Use for endpoints without a dedicated tool (e.g. /api/frames/{frame_id}/nudges, /api/frames/{frame_id}/household_config, /api/frames/{frame_id}/albums). Responses are simplified from JSON:API unless raw=true.",
		destructive, func(ctx context.Context, in rawIn) (any, error) {
			m := strings.ToUpper(strings.TrimSpace(in.Method))
			switch m {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				return nil, fmt.Errorf("unsupported method %q", in.Method)
			}
			p := in.Path
			if strings.Contains(p, "{frame_id}") {
				fid, err := r.c.FrameID(ctx)
				if err != nil {
					return nil, err
				}
				p = strings.ReplaceAll(p, "{frame_id}", fid)
			}
			p, err := skylight.CleanPath(p)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			for k, v := range in.Query {
				q.Set(k, v)
			}
			raw, err := r.c.Do(ctx, m, p, q, in.Body)
			if err != nil {
				return nil, err
			}
			if in.Raw {
				return raw, nil
			}
			return skylight.Simplify(raw), nil
		})

	add(r, "get_user", "Get the logged-in Skylight user's account profile.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.global(ctx, http.MethodGet, "/api/user", nil, nil)
		})
	add(r, "get_frame", "Get details of the Skylight frame (household).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) { return r.call(ctx, http.MethodGet, "", nil, nil) })
	add(r, "get_plus_access", "Get Skylight Plus subscription status.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.global(ctx, http.MethodGet, "/api/plus_access", nil, nil)
		})
	add(r, "list_colors", "List the colors available for profiles, lists and meal categories (name + hex).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.global(ctx, http.MethodGet, "/api/colors", nil, nil)
		})
	add(r, "list_avatars", "List avatars that can be assigned to family profiles (id, name, image_url).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.global(ctx, http.MethodGet, "/api/avatars", nil, nil)
		})
}
