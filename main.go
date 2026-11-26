// Command skylight-mcp serves the Skylight Calendar API as an MCP server over
// streamable HTTP.
package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"skylight-mcp/internal/config"
	"skylight-mcp/internal/skylight"
	"skylight-mcp/internal/tools"
)

var version = "0.1.0"

const instructions = `Tools for a family's Skylight Calendar frame.
Terminology: "categories" in the Skylight API are family member profiles; profile_id / profile_ids arguments take category ids.
Call skylight_overview first to learn profile, list and calendar ids. Dates are YYYY-MM-DD; timed events use ISO 8601 with a UTC offset.
Recurring event instances have ids like <masterId>-<epochSeconds>; use apply_to (this, this_and_future, all) when changing them.
For anything without a dedicated tool, use skylight_api_request.`

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	check := flag.Bool("check", false, "log in, print the frame id, and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	client := skylight.New(cfg.Skylight, log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *check {
		id, err := client.FrameID(ctx)
		if err != nil {
			log.Error("check failed", "err", err)
			os.Exit(1)
		}
		log.Info("check ok", "frame_id", id, "timezone", client.Timezone(ctx))
		return
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "skylight-mcp", Version: version},
		&mcp.ServerOptions{Instructions: instructions})
	tools.Register(server, client)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: cfg.Server.Stateless})

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.Path, requireToken(cfg.Server.AuthToken, handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	if cfg.Server.AuthToken == "" {
		log.Warn("server.auth_token is empty: the MCP endpoint is unauthenticated")
	}
	log.Info("skylight-mcp listening", "addr", cfg.Server.Listen, "path", cfg.Server.Path)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func requireToken(tok string, next http.Handler) http.Handler {
	if tok == "" {
		return next
	}
	want := []byte("Bearer " + tok)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
