package config

import (
	"github.com/go-playground/validator/v10"
)

type Config struct {
	TaskScheduler struct {
		Port         int    `yaml:"port"`
		Name         string `yaml:"name"`
		PollInterval int    `yaml:"poll_interval"`
	} `yaml:"task-scheduler"`

	RedisClient struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		DB       int    `yaml:"db"`
		Password string `yaml:"password"`
	} `yaml:"redis_client"`
}

// Validate performs validation on the Config struct.
func (c Config) Validate() error {
	validate := validator.New()
	return validate.Struct(c)
}
