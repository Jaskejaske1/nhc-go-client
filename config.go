// config.go
package nhc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"nhc-go-client/internal/curve"
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

type DeviceAliases struct {
	Lights  map[int]string `json:"lights,omitempty"`
	Scenes  map[int]string `json:"scenes,omitempty"`
	Sockets map[int]string `json:"sockets,omitempty"`
}

type BrightnessSettings struct {
	Type   string        `json:"type,omitempty"`
	Gamma  float64       `json:"gamma,omitempty"`
	Points []curve.Point `json:"points,omitempty"`
}

func DefaultBrightnessSettings() BrightnessSettings {
	return BrightnessSettings{Type: "linear"}
}

func (s BrightnessSettings) Mapper() (curve.Mapper, error) {
	switch s.Type {
	case "", "linear":
		return curve.Linear{}, nil
	case "gamma":
		return curve.NewGamma(s.Gamma)
	case "lookup":
		return curve.NewLookup(s.Points)
	default:
		return nil, fmt.Errorf("unknown brightness curve: %s", s.Type)
	}
}

// Add this type for JSON loading
type configJSON struct {
	IP         string             `json:"ip"`
	Port       int                `json:"port"`
	Timeout    string             `json:"timeout"`
	Aliases    DeviceAliases      `json:"aliases,omitempty"`
	Brightness BrightnessSettings `json:"brightness,omitempty"`
	Macros     map[string]Macro   `json:"macros,omitempty"`
}

type NikoConfig struct {
	IP         string             `json:"ip"`
	Port       int                `json:"port"`
	Timeout    time.Duration      `json:"timeout"`
	Aliases    DeviceAliases      `json:"aliases,omitempty"`
	Brightness BrightnessSettings `json:"brightness,omitempty"`
	Macros     map[string]Macro   `json:"macros,omitempty"`
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

func WithBrightnessCurve(curveType string) ConfigOption {
	return func(c *NikoConfig) {
		c.Brightness.Type = curveType
	}
}

func WithBrightnessGamma(gamma float64) ConfigOption {
	return func(c *NikoConfig) {
		c.Brightness.Type = "gamma"
		c.Brightness.Gamma = gamma
	}
}

// DefaultConfig returns the default configuration
func DefaultConfig() NikoConfig {
	return NikoConfig{
		Port:    DefaultPort,
		Timeout: DefaultTimeout,
		Aliases: DeviceAliases{
			Lights:  make(map[int]string),
			Scenes:  make(map[int]string),
			Sockets: make(map[int]string),
		},
		Brightness: DefaultBrightnessSettings(),
		Macros:     make(map[string]Macro),
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
	c.Aliases = jsonConfig.Aliases
	if jsonConfig.Macros != nil {
		c.Macros = jsonConfig.Macros
	}
	if jsonConfig.Brightness.Type != "" {
		c.Brightness = jsonConfig.Brightness
	}

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

	if curveType := os.Getenv("NHC_BRIGHTNESS_CURVE"); curveType != "" {
		c.Brightness.Type = curveType
	}

	if gammaStr := os.Getenv("NHC_BRIGHTNESS_GAMMA"); gammaStr != "" {
		if gamma, err := strconv.ParseFloat(gammaStr, 64); err == nil {
			c.Brightness.Type = "gamma"
			c.Brightness.Gamma = gamma
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

	if _, err := c.Brightness.Mapper(); err != nil {
		return fmt.Errorf("invalid brightness curve: %w", err)
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
		IP:         c.IP,
		Port:       c.Port,
		Timeout:    c.Timeout.String(),
		Aliases:    c.Aliases,
		Brightness: c.Brightness,
		Macros:     c.Macros,
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

// A helper method for NikoConfig
func (c *NikoConfig) GetDeviceAlias(deviceType string, id int) string {
	switch deviceType {
	case "light":
		if alias, ok := c.Aliases.Lights[id]; ok {
			return alias
		}
	case "scene":
		if alias, ok := c.Aliases.Scenes[id]; ok {
			return alias
		}
	case "socket":
		if alias, ok := c.Aliases.Sockets[id]; ok {
			return alias
		}
	}
	return "" // Return empty string if no alias found
}

// A helper method to set aliases
func (c *NikoConfig) SetDeviceAlias(deviceType string, id int, alias string) error {
	switch deviceType {
	case "light":
		if c.Aliases.Lights == nil {
			c.Aliases.Lights = make(map[int]string)
		}
		c.Aliases.Lights[id] = alias
	case "scene":
		if c.Aliases.Scenes == nil {
			c.Aliases.Scenes = make(map[int]string)
		}
		c.Aliases.Scenes[id] = alias
	case "socket":
		if c.Aliases.Sockets == nil {
			c.Aliases.Sockets = make(map[int]string)
		}
		c.Aliases.Sockets[id] = alias
	default:
		return fmt.Errorf("invalid device type: %s", deviceType)
	}
	return nil
}
