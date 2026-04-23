// Package config loads and validates homecast's YAML configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Listen string `yaml:"listen" json:"listen"`
}

type AirConnect struct {
	BinaryPath  string `yaml:"binary_path" json:"binaryPath"`
	LogPath     string `yaml:"log_path" json:"logPath"`
	AutoRestart bool   `yaml:"auto_restart" json:"autoRestart"`
}

type Device struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}

type Config struct {
	Server     Server     `yaml:"server" json:"server"`
	AirConnect AirConnect `yaml:"airconnect" json:"airconnect"`
	Devices    []Device   `yaml:"devices" json:"devices"`
}

func Default() *Config {
	return &Config{
		Server: Server{Listen: "0.0.0.0:8080"},
		AirConnect: AirConnect{
			BinaryPath:  "/usr/local/lib/homecast/aircast",
			LogPath:     "/var/log/homecast/aircast.log",
			AutoRestart: true,
		},
		Devices: []Device{},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".homecast-config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

func (c *Config) Validate() error {
	if c.Server.Listen == "" {
		return errors.New("server.listen must not be empty")
	}
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		return fmt.Errorf("server.listen must be host:port: %w", err)
	}
	if c.AirConnect.BinaryPath == "" {
		return errors.New("airconnect.binary_path must not be empty")
	}
	seen := make(map[string]struct{}, len(c.Devices))
	for i, d := range c.Devices {
		if d.ID == "" {
			return fmt.Errorf("devices[%d].id must not be empty", i)
		}
		if d.Name == "" {
			return fmt.Errorf("devices[%d].name must not be empty", i)
		}
		if _, dup := seen[d.ID]; dup {
			return fmt.Errorf("devices[%d].id %q is duplicated", i, d.ID)
		}
		seen[d.ID] = struct{}{}
	}
	return nil
}
