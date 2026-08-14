package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port int                  `yaml:"port"`
	Apps map[string]AppConfig `yaml:"apps"`
}

type AppConfig struct {
	Path          string `yaml:"path"`
	Branch        string `yaml:"branch"`         // Default: "main"
	WebhookSecret string `yaml:"webhook_secret"`
	DeployCmd     string `yaml:"deploy_cmd"`     // Default: "docker compose up -d --build"
}

// Load reads and parses a YAML configuration file.
func Load(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", filepath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	// Set default values
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}

	if cfg.Apps == nil {
		cfg.Apps = make(map[string]AppConfig)
	}

	for name, app := range cfg.Apps {
		if app.Branch == "" {
			app.Branch = "main"
		}
		if app.DeployCmd == "" {
			app.DeployCmd = "docker compose up -d --build"
		}
		cfg.Apps[name] = app
	}

	return &cfg, nil
}
