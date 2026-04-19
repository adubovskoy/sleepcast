package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr     string
	DataDir  string
	TTLHours int
}

func Load() (*Config, error) {
	c := &Config{
		Addr:    getenv("SLEEPCAST_ADDR", ":5005"),
		DataDir: getenv("SLEEPCAST_DATA_DIR", "./data"),
	}
	ttl, err := strconv.Atoi(getenv("SLEEPCAST_TTL_HOURS", "168"))
	if err != nil {
		return nil, fmt.Errorf("SLEEPCAST_TTL_HOURS: %w", err)
	}
	c.TTLHours = ttl
	return c, nil
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
