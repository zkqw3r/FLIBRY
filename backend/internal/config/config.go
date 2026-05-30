package config

import (
	"errors"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort     string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	TorProxyURL    string
	TorFlibustaURL string
	StoragePath    string
	SessionSecret  string
	TemplatesPath  string
}

func Load() (*Config, error) {
	// Ignore error: .env is optional in production as variables are injected via Docker
	if err := godotenv.Load(); err != nil {
		log.Println("[INFO] .env file not found. Using system environment variables.")
	}

	return &Config{
		ServerPort:     os.Getenv("PORT"),
		DBHost:         os.Getenv("DB_HOST"),
		DBPort:         os.Getenv("DB_PORT"),
		DBUser:         os.Getenv("DB_USER"),
		DBPassword:     os.Getenv("DB_PASSWORD"),
		DBName:         os.Getenv("DB_NAME"),
		DBSSLMode:      os.Getenv("DB_SSLMODE"),
		TorProxyURL:    os.Getenv("TOR_PROXY"),
		TorFlibustaURL: os.Getenv("FLIBUSTA_URL"),
		StoragePath:    os.Getenv("STORAGE_PATH"),
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		TemplatesPath:  os.Getenv("TEMPLATES_PATH"),
	}, nil
}

func (c *Config) Validate() error {
	if c.ServerPort == "" {
		return errors.New("environment variable PORT is not set")
	}
	if c.DBHost == "" {
		return errors.New("environment variable DB_HOST is not set")
	}
	if c.DBPort == "" {
		return errors.New("environment variable DB_PORT is not set")
	}
	if c.DBUser == "" {
		return errors.New("environment variable DB_USER is not set")
	}
	if c.DBPassword == "" {
		return errors.New("environment variable DB_PASSWORD is not set")
	}
	if c.DBName == "" {
		return errors.New("environment variable DB_NAME is not set")
	}
	if c.DBSSLMode == "" {
		return errors.New("environment variable DB_SSLMODE is not set")
	}
	if c.TorProxyURL == "" {
		return errors.New("environment variable TOR_PROXY is not set")
	}
	if c.TorFlibustaURL == "" {
		return errors.New("environment variable FLIBUSTA_URL is not set")
	}
	if c.StoragePath == "" {
		return errors.New("environment variable STORAGE_PATH is not set")
	}
	if c.SessionSecret == "" {
		return errors.New("environment variable SESSION_SECRET is not set")
	}
	if c.TemplatesPath == "" {
		return errors.New("environment variable TEMPLATES_PATH is not set")
	}

	return nil
}
