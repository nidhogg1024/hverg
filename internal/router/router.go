package router

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/nidhogg1024/hverg/internal/config"
	"github.com/nidhogg1024/hverg/internal/plugin"
	"github.com/nidhogg1024/hverg/internal/proxy"
)

// Engine is the core HTTP router engine of Hverg.
type Engine struct {
	mux *http.ServeMux
}

// NewEngine creates a new routing engine based on the given configuration.
func NewEngine(cfg *config.Config) (*Engine, error) {
	mux := http.NewServeMux()

	for _, routeCfg := range cfg.Routes {
		// 1. Build the plugin chain for this route
		chain, err := plugin.NewChain(routeCfg)
		if err != nil {
			slog.Error("Failed to build plugin chain", "route", routeCfg.Path, "err", err)
			return nil, err
		}

		// 2. Build the backend proxy for this route
		var backendProxy http.Handler

		if strings.HasPrefix(routeCfg.Backend, "http://") || strings.HasPrefix(routeCfg.Backend, "https://") {
			backendProxy, err = proxy.NewReverseProxy(routeCfg.Backend)
			if err != nil {
				slog.Error("Failed to create reverse proxy", "backend", routeCfg.Backend, "err", err)
				return nil, err
			}
		} else if strings.HasPrefix(routeCfg.Backend, "grpc://") {
			// gRPC backend handling will be hooked in via the transcoder plugin.
			// But if the transcoder plugin handles it, we still need a fallback or let transcoder handle the entire response.
			// For now, if it's grpc, we assume the transcoder plugin takes over and aborts the chain
			// by writing the response directly.
			backendProxy = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// If a request reaches here for a gRPC backend, it means transcoder plugin didn't handle it.
				http.Error(w, "gRPC backend requires transcoder plugin", http.StatusInternalServerError)
			})
		} else {
			// Mock or other protocols
			backendProxy = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Unsupported backend protocol", http.StatusBadGateway)
			})
		}

		// 3. Register the route with http.ServeMux
		// In Go 1.22+, ServeMux supports method + path matching (e.g., "GET /api/v1/users")
		pattern := routeCfg.Path
		if routeCfg.Method != "" {
			pattern = routeCfg.Method + " " + routeCfg.Path
		}

		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ctx := &plugin.Context{
				Writer:  w,
				Request: r,
				Aborted: false,
			}

			// Execute pre-routing / pre-proxy plugins
			err := chain.Execute(ctx)
			if err != nil {
				slog.Error("Plugin chain execution failed", "path", r.URL.Path, "err", err)
				if !ctx.Aborted {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}

			// If a plugin aborted the request (e.g., auth failed, or transcoder already responded), do not proxy.
			if ctx.Aborted {
				return
			}

			// Proxy to the backend
			backendProxy.ServeHTTP(w, r)
		})

		slog.Info("Route registered", "pattern", pattern, "backend", routeCfg.Backend, "plugins_count", len(chain.Plugins))
	}

	return &Engine{
		mux: mux,
	}, nil
}

// ServeHTTP implements http.Handler.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mux.ServeHTTP(w, r)
}
