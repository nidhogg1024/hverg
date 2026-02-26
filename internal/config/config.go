package config

// Config represents the root configuration of the Hverg Gateway
type Config struct {
	Server ServerConfig  `yaml:"server"`
	Routes []RouteConfig `yaml:"routes"`
}

// ServerConfig configures the gateway listener
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// RouteConfig represents a single API route mapping
type RouteConfig struct {
	Path    string         `yaml:"path"`    // e.g., /api/v1/users/{id}
	Method  string         `yaml:"method"`  // e.g., GET, POST, or empty for all
	Backend string         `yaml:"backend"` // e.g., http://user-service:8080 or grpc://order-service:9090
	Plugins []PluginConfig `yaml:"plugins"`
}

// PluginConfig represents the configuration for a plugin on a route
type PluginConfig struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}
