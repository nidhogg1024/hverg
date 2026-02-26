package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads the YAML configuration file and parses it into the Config struct.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}

	return &cfg, nil
}
