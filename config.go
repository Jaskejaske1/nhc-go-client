// config.go
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Errors
var (
	ErrNoIPAddress     = fmt.Errorf("no IP address configured")
	ErrInvalidIP       = fmt.Errorf("invalid IP address format")
	ErrInvalidPort     = fmt.Errorf("invalid port number")
	ErrInvalidTimeout  = fmt.Errorf("invalid timeout value")
	ErrConfigNotFound  = fmt.Errorf("configuration file not found")
	ErrConfigCorrupted = fmt.Errorf("configuration file is corrupted")
)

// Add this type for JSON loading
type configJSON struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Timeout string `json:"timeout"`
}

type NikoConfig struct {
	IP      string        `json:"ip"`
	Port    int           `json:"port"`
	Timeout time.Duration `json:"timeout"`
}

// ConfigOption represents a function that modifies the configuration
type ConfigOption func(*NikoConfig)

// WithIP returns a ConfigOption that sets the IP address
func WithIP(ip string) ConfigOption {
	return func(c *NikoConfig) {
		c.IP = ip
	}
}

// WithPort returns a ConfigOption that sets the port
func WithPort(port int) ConfigOption {
	return func(c *NikoConfig) {
		c.Port = port
	}
}

// WithTimeout returns a ConfigOption that sets the timeout
func WithTimeout(timeout time.Duration) ConfigOption {
	return func(c *NikoConfig) {
		c.Timeout = timeout
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() NikoConfig {
	return NikoConfig{
		Port:    DefaultPort,
		Timeout: DefaultTimeout,
	}
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "nhc-go-client", "config.json"), nil
}

// LoadConfig loads configuration from multiple sources in order of precedence:
// 1. Command line flags (not implemented yet)
// 2. Environment variables
// 3. Config file
// 4. Default values
func LoadConfig(opts ...ConfigOption) (*NikoConfig, error) {
	// Start with default config
	config := DefaultConfig()

	// Load from config file
	err := config.loadFromFile()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Load from environment variables
	config.loadFromEnv()

	// Apply any provided options
	for _, opt := range opts {
		opt(&config)
	}

	// Validate the configuration
	if err := config.validate(); err != nil {
		if err == ErrNoIPAddress {
			return nil, ErrNoIPAddress
		}
		return nil, err
	}

	return &config, nil
}

// loadFromFile loads configuration from a JSON file in the user's home directory
func (c *NikoConfig) loadFromFile() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Use the temporary JSON struct for loading
	var jsonConfig configJSON
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&jsonConfig); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigCorrupted, err)
	}

	// Convert the values to the actual config struct
	c.IP = jsonConfig.IP
	c.Port = jsonConfig.Port

	// Parse the timeout string into a duration
	timeout, err := time.ParseDuration(jsonConfig.Timeout)
	if err != nil {
		return fmt.Errorf("failed to parse timeout duration: %w", err)
	}
	c.Timeout = timeout

	return nil
}

// loadFromEnv loads configuration from environment variables
func (c *NikoConfig) loadFromEnv() {
	if ip := os.Getenv("NHC_IP"); ip != "" {
		c.IP = ip
	}

	if portStr := os.Getenv("NHC_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			c.Port = port
		}
	}

	if timeoutStr := os.Getenv("NHC_TIMEOUT"); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			c.Timeout = timeout
		}
	}
}

// validate checks if the configuration is valid
func (c *NikoConfig) validate() error {
	if c.IP == "" {
		return ErrNoIPAddress
	}

	// Validate IP format
	if net.ParseIP(c.IP) == nil {
		return fmt.Errorf("%w: %s", ErrInvalidIP, c.IP)
	}

	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("%w: %d", ErrInvalidPort, c.Port)
	}

	if c.Timeout < time.Second {
		return fmt.Errorf("%w: minimum 1 second required", ErrInvalidTimeout)
	}

	return nil
}

// SaveConfig saves the current configuration to a file
func (c *NikoConfig) SaveConfig() error {
	// Validate before saving
	if err := c.validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Create a temporary struct for JSON marshaling
	jsonConfig := configJSON{
		IP:      c.IP,
		Port:    c.Port,
		Timeout: c.Timeout.String(), // This will format it as "20s" instead of nanoseconds
	}

	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create temporary file
	tmpFile := configPath + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary config file: %w", err)
	}

	// Write to temporary file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonConfig); err != nil {
		file.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("failed to encode config: %w", err)
	}

	// Close the file before moving it
	if err := file.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomically replace the old config file
	if err := os.Rename(tmpFile, configPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to save config file: %w", err)
	}

	return nil
}
