package plugin

import (
	"net/http"

	"github.com/nidhogg1024/hverg/internal/config"
)

// Context provides contextual information for a plugin during execution.
// It wraps standard HTTP request and response writer.
type Context struct {
	Writer  http.ResponseWriter
	Request *http.Request

	// Aborted indicates if the current request should stop propagating through the plugin chain.
	// This is typically set to true by auth or rate limiting plugins when they fail.
	Aborted bool
}

// Abort stops the plugin chain and optionally writes a status code.
func (c *Context) Abort(code int) {
	c.Aborted = true
	c.Writer.WriteHeader(code)
}

// AbortWithStatusJSON aborts and writes JSON response
func (c *Context) AbortWithStatusJSON(code int, jsonBytes []byte) {
	c.Aborted = true
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	c.Writer.Write(jsonBytes)
}

// Plugin defines the interface that all Hverg plugins must implement.
type Plugin interface {
	// Name returns the name of the plugin.
	Name() string
	// Handle executes the plugin logic. It returns an error if something goes wrong.
	Handle(ctx *Context) error
}

// Factory defines a function that creates a new instance of a Plugin given its configuration.
type Factory func(cfg map[string]interface{}) (Plugin, error)

var registry = make(map[string]Factory)

// Register registers a new plugin factory by name.
func Register(name string, factory Factory) {
	registry[name] = factory
}

// Get creates a new plugin instance by name.
func Get(name string, cfg map[string]interface{}) (Plugin, error) {
	if factory, ok := registry[name]; ok {
		return factory(cfg)
	}
	return nil, nil // Or return a specific "plugin not found" error
}

// Chain represents a sequence of plugins configured for a specific route.
type Chain struct {
	Plugins []Plugin
}

// NewChain creates a plugin chain from route configuration.
func NewChain(routeCfg config.RouteConfig) (*Chain, error) {
	chain := &Chain{}
	for _, pCfg := range routeCfg.Plugins {
		if pCfg.Config == nil {
			pCfg.Config = make(map[string]interface{})
		}
		// Inject the route backend so plugins (like transcoder) know where to route
		pCfg.Config["_route_backend"] = routeCfg.Backend

		p, err := Get(pCfg.Name, pCfg.Config)
		if err != nil {
			return nil, err
		}
		if p != nil {
			chain.Plugins = append(chain.Plugins, p)
		}
	}
	return chain, nil
}

// Execute runs the plugin chain. If any plugin aborts the context or returns an error,
// the execution stops.
func (c *Chain) Execute(ctx *Context) error {
	for _, p := range c.Plugins {
		err := p.Handle(ctx)
		if err != nil {
			return err
		}
		if ctx.Aborted {
			break
		}
	}
	return nil
}
