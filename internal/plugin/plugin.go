package plugin

import (
	"net/http"

	"github.com/nidhogg1024/hverg/internal/config"
)

// Context 是每个请求流经插件链时的上下文对象。
// 它封装了标准的 HTTP 请求/响应，并提供跨插件的 Key-Value 存储。
type Context struct {
	Writer  http.ResponseWriter
	Request *http.Request

	// Aborted 表示当前请求是否应停止在插件链中继续传播
	Aborted bool

	// keys 存储跨插件传递的数据，例如鉴权插件解析出的 UserID 可以被后续插件读取。
	// 延迟初始化，只在第一次 Set 时分配内存。
	keys map[string]interface{}
}

// Set 在上下文中存储一个 Key-Value 对，供后续插件读取。
func (c *Context) Set(key string, value interface{}) {
	if c.keys == nil {
		c.keys = make(map[string]interface{})
	}
	c.keys[key] = value
}

// Get 从上下文中获取指定 key 的值。
func (c *Context) Get(key string) (interface{}, bool) {
	if c.keys == nil {
		return nil, false
	}
	val, ok := c.keys[key]
	return val, ok
}

// GetString 从上下文中获取指定 key 的字符串值，不存在或类型不匹配时返回空字符串。
func (c *Context) GetString(key string) string {
	val, ok := c.Get(key)
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}

// Abort 中止插件链并写入 HTTP 状态码。
func (c *Context) Abort(code int) {
	c.Aborted = true
	c.Writer.WriteHeader(code)
}

// AbortWithStatusJSON 中止插件链并写入 JSON 响应。
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
