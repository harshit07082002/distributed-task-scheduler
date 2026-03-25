package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

const CONFIG_PATH = "configs/config.yaml"

var (
	configOnce sync.Once
	config     *Config
	configErr  error
)

func Initialize() error {
	if err := loadConfig(); err != nil {
		return err
	}

	return nil
}

func loadConfig() error {
	configOnce.Do(func() {
		cfg, err := readConfig()
		config, configErr = cfg, err
	})
	return configErr
}

func readConfig() (*Config, error) {
	configFile, err := os.ReadFile(CONFIG_PATH)
	if err != nil {
		return nil, fmt.Errorf("config read error: %w", err)
	}

	var cfg Config
	if err = yaml.Unmarshal(configFile, &cfg); err != nil {
		return nil, fmt.Errorf("config unmarshal error: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	return &cfg, nil
}

func GetConfig() *Config {
	return config
}
