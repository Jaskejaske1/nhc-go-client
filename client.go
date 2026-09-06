// client.go
package nhc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"nhc-go-client/internal/curve"
)

const (
	DefaultPort    = 8000
	DefaultTimeout = 20 * time.Second
	MaxRetries     = 3
	RetryDelay     = time.Second
	MaxActionValue = curve.MaxPercent
)

type LogLevel int

const (
	LogLevelNone LogLevel = iota
	LogLevelError
	LogLevelInfo
	LogLevelDebug
)

type ActionType string

const (
	ActionTypeLight  ActionType = "LIGHT"
	ActionTypeScene  ActionType = "SCENE"
	ActionTypeSocket ActionType = "SOCKET"
)

type Config struct {
	IP              string
	Port            int
	Timeout         time.Duration
	BrightnessCurve curve.Mapper
}

type Client struct {
	ip       string
	port     int
	timeout  time.Duration
	conn     net.Conn
	mu       sync.Mutex
	reader   *bufio.Reader
	logLevel LogLevel
	curve    curve.Mapper
}

type commandRequest struct {
	Cmd    string `json:"cmd"`
	ID     int    `json:"id,omitempty"`
	Value1 string `json:"value1,omitempty"`
}

type commandResponse struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error,omitempty"`
}

type Action struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	Value1   int        `json:"value1"`
	Type     ActionType `json:"-"`
	Location int        `json:"location"`
	RawType  int        `json:"type"`
	client   *Client
}

type MacroStep struct {
	ID    int        `json:"id"`
	Type  ActionType `json:"type"`
	Value int        `json:"value"`
}

type Macro struct {
	Steps []MacroStep `json:"steps"`
}

type Location struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Thermostat struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Location     int    `json:"location"`
	Measured     int    `json:"measured"`
	Setpoint     int    `json:"setpoint"`
	Mode         int    `json:"mode"`
	Overrule     int    `json:"overrule"`
	Overruletime int    `json:"overruletime"`
	Ecosave      bool   `json:"ecosave"`
}

func NewClient(config Config) (*Client, error) {
	if config.Port == 0 {
		config.Port = DefaultPort
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}

	client := &Client{
		ip:       config.IP,
		port:     config.Port,
		timeout:  config.Timeout,
		logLevel: LogLevelError, // Default to error logging
		curve:    config.BrightnessCurve,
	}
	if client.curve == nil {
		client.curve = curve.Linear{}
	}

	if err := client.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return client, nil
}

func (c *Client) SetLogLevel(level LogLevel) {
	c.logLevel = level
}

func (c *Client) SetBrightnessCurve(mapper curve.Mapper) {
	if mapper == nil {
		c.curve = curve.Linear{}
		return
	}
	c.curve = mapper
}

func (c *Client) mapBrightness(percent int) (int, error) {
	return c.curve.Map(percent)
}

func (c *Client) logDebug(format string, args ...interface{}) {
	if c.logLevel >= LogLevelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func (c *Client) logInfo(format string, args ...interface{}) {
	if c.logLevel >= LogLevelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

func (c *Client) logError(format string, args ...interface{}) {
	if c.logLevel >= LogLevelError {
		log.Printf("[ERROR] "+format, args...)
	}
}

func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) connectLocked() error {
	if c.conn != nil {
		c.conn.Close()
	}

	c.logDebug("Connecting to %s:%d", c.ip, c.port)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", c.ip, c.port), c.timeout)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.logInfo("Connected successfully")
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.logDebug("Closing connection")
		return c.conn.Close()
	}
	return nil
}

func validateResponse(data []byte) error {
	var resp commandResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("invalid response format: %w", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("server error: %s", resp.Error)
	}

	return nil
}

func (c *Client) sendCommand(cmd interface{}) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	data = append(data, '\r')
	c.logDebug("Sending command: %s", string(data))

	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			delay := RetryDelay * time.Duration(attempt)
			c.logInfo("Retry attempt %d after %v delay", attempt+1, delay)
			time.Sleep(delay)
			if err := c.connectLocked(); err != nil {
				lastErr = fmt.Errorf("retry %d: failed to reconnect: %w", attempt+1, err)
				continue
			}
		}

		if c.conn == nil {
			if err := c.connectLocked(); err != nil {
				lastErr = fmt.Errorf("retry %d: failed to reconnect: %w", attempt+1, err)
				continue
			}
		}

		if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
			lastErr = fmt.Errorf("retry %d: failed to set write deadline: %w", attempt+1, err)
			c.logError("Deadline error: %v", lastErr)
			continue
		}

		if _, err := c.conn.Write(data); err != nil {
			lastErr = fmt.Errorf("retry %d: failed to write command: %w", attempt+1, err)
			c.logError("Write error: %v", lastErr)
			c.conn.Close()
			c.conn = nil
			continue
		}

		if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			lastErr = fmt.Errorf("retry %d: failed to set read deadline: %w", attempt+1, err)
			c.logError("Deadline error: %v", lastErr)
			continue
		}

		response, err := c.reader.ReadBytes('\r')
		if err != nil {
			lastErr = fmt.Errorf("retry %d: failed to read response: %w", attempt+1, err)
			c.logError("Read error: %v", lastErr)
			c.conn.Close()
			c.conn = nil
			continue
		}

		c.logDebug("Received response: %s", string(response))
		return response, nil
	}

	return nil, fmt.Errorf("after %d attempts: %w", MaxRetries, lastErr)
}

func (c *Client) GetSystemInfo() (map[string]interface{}, error) {
	cmd := commandRequest{Cmd: "systeminfo"}
	data, err := c.sendCommand(cmd)
	if err != nil {
		return nil, err
	}

	if err := validateResponse(data); err != nil {
		return nil, err
	}

	var resp commandResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse system info: %w", err)
	}

	return info, nil
}

func (c *Client) GetActions() ([]Action, error) {
	cmd := commandRequest{Cmd: "listactions"}
	data, err := c.sendCommand(cmd)
	if err != nil {
		return nil, err
	}

	if err := validateResponse(data); err != nil {
		return nil, err
	}

	var resp commandResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var actions []Action
	if err := json.Unmarshal(resp.Data, &actions); err != nil {
		return nil, fmt.Errorf("failed to parse actions: %w", err)
	}

	// Set the client and determine type for each action
	for i := range actions {
		actions[i].client = c
		actions[i].Type = actions[i].DetermineType()
	}

	return actions, nil
}

func (c *Client) GetLocations() ([]Location, error) {
	cmd := commandRequest{Cmd: "listlocations"}
	data, err := c.sendCommand(cmd)
	if err != nil {
		return nil, err
	}

	if err := validateResponse(data); err != nil {
		return nil, err
	}

	var resp commandResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var locations []Location
	if err := json.Unmarshal(resp.Data, &locations); err != nil {
		return nil, fmt.Errorf("failed to parse locations: %w", err)
	}

	return locations, nil
}

func (c *Client) GetThermostats() ([]Thermostat, error) {
	cmd := commandRequest{Cmd: "listthermostat"}
	data, err := c.sendCommand(cmd)
	if err != nil {
		return nil, err
	}

	if err := validateResponse(data); err != nil {
		return nil, err
	}

	var resp commandResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var thermostats []Thermostat
	if err := json.Unmarshal(resp.Data, &thermostats); err != nil {
		return nil, fmt.Errorf("failed to parse thermostats: %w", err)
	}

	return thermostats, nil
}

func (c *Client) ExecuteAction(id int, value int) error {
	cmd := commandRequest{
		Cmd:    "executeactions",
		ID:     id,
		Value1: fmt.Sprintf("%d", value),
	}
	data, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	return validateResponse(data)
}

func (a *Action) IsOn() bool {
	return a.Value1 != 0
}

func (a *Action) DetermineType() ActionType {
	// First check the raw type from the API
	if a.RawType == 2 {
		return ActionTypeLight // Dimmable lights are type 2
	}

	// For type 1, determine based on name
	name := strings.ToLower(a.Name)
	switch {
	case strings.Contains(name, "sfeer"):
		return ActionTypeScene
	case strings.Contains(name, "stopcontact"):
		return ActionTypeSocket
	default:
		return ActionTypeLight // Regular lights and others are type 1
	}
}

func (a *Action) TurnOn(brightness ...int) error {
	value := MaxActionValue
	if len(brightness) > 0 {
		if brightness[0] < 0 || brightness[0] > MaxActionValue {
			return fmt.Errorf("brightness must be between 0 and %d, got %d", MaxActionValue, brightness[0])
		}
		value = brightness[0]
	}

	// Only allow dimming for lights
	if a.Type != ActionTypeLight && len(brightness) > 0 {
		return fmt.Errorf("brightness can only be set for lights, not for %s", a.Type)
	}
	requested := value
	if len(brightness) == 0 {
		requested = MaxActionValue
	}
	value, err := a.client.mapBrightness(requested)
	if err != nil {
		return fmt.Errorf("invalid brightness for action %d: %w", a.ID, err)
	}

	err = a.client.ExecuteAction(a.ID, value)
	if err != nil {
		return fmt.Errorf("failed to turn on action %d: %w", a.ID, err)
	}
	a.Value1 = value
	return nil
}

func (a *Action) TurnOff() error {
	err := a.client.ExecuteAction(a.ID, 0)
	if err != nil {
		return err
	}
	a.Value1 = 0
	return nil
}

func (a *Action) Toggle() error {
	if a.IsOn() {
		return a.TurnOff()
	}
	return a.TurnOn()
}

func (a *Action) Update() error {
	actions, err := a.client.GetActions()
	if err != nil {
		return err
	}

	for _, action := range actions {
		if action.ID == a.ID {
			a.Value1 = action.Value1
			a.Name = action.Name
			a.Type = action.Type
			return nil
		}
	}

	return fmt.Errorf("action with ID %d not found", a.ID)
}

func TransformTemperature(temp int) float64 {
	tempStr := fmt.Sprintf("%d", temp)
	if len(tempStr) == 3 {
		return float64(temp/10) + float64(temp%10)/10.0
	}
	return 0.0
}

func (t *Thermostat) GetFormattedTemperature(temp int) float64 {
	return TransformTemperature(temp)
}

func (t *Thermostat) Update(c *Client) error {
	thermostats, err := c.GetThermostats()
	if err != nil {
		return err
	}

	for _, th := range thermostats {
		if th.ID == t.ID {
			*t = th
			return nil
		}
	}

	return fmt.Errorf("thermostat with ID %d not found", t.ID)
}

// Add these methods to client.go
func (c *Client) TurnOn(id int) error {
	return c.ExecuteAction(id, MaxActionValue)
}

func (c *Client) TurnOff(id int) error {
	return c.ExecuteAction(id, 0)
}

func (c *Client) GetActionByID(id int) (*Action, error) {
	actions, err := c.GetActions()
	if err != nil {
		return nil, err
	}

	for _, action := range actions {
		if action.ID == id {
			return &action, nil
		}
	}
	return nil, fmt.Errorf("action with ID %d not found", id)
}

func (c *Client) RunMacro(macro Macro) error {
	if len(macro.Steps) == 0 {
		return fmt.Errorf("macro has no steps")
	}
	actionByID, err := c.validateMacro(macro)
	if err != nil {
		return err
	}

	for index, step := range macro.Steps {
		action := actionByID[step.ID]
		if step.Value == 0 {
			if err := c.TurnOff(step.ID); err != nil {
				return fmt.Errorf("macro step %d: %w", index+1, err)
			}
			continue
		}
		if action.Type == ActionTypeLight {
			action.client = c
			if err := action.TurnOn(step.Value); err != nil {
				return fmt.Errorf("macro step %d: %w", index+1, err)
			}
			continue
		}
		if err := c.ExecuteAction(step.ID, step.Value); err != nil {
			return fmt.Errorf("macro step %d: %w", index+1, err)
		}
	}
	return nil
}

func (c *Client) ValidateMacro(macro Macro) error {
	_, err := c.validateMacro(macro)
	return err
}

func (c *Client) validateMacro(macro Macro) (map[int]Action, error) {
	if len(macro.Steps) == 0 {
		return nil, fmt.Errorf("macro has no steps")
	}
	actions, err := c.GetActions()
	if err != nil {
		return nil, fmt.Errorf("load actions for macro: %w", err)
	}
	actionByID := make(map[int]Action, len(actions))
	for _, action := range actions {
		actionByID[action.ID] = action
	}

	for index, step := range macro.Steps {
		action, ok := actionByID[step.ID]
		if !ok {
			return nil, fmt.Errorf("macro step %d: action %d not found", index+1, step.ID)
		}
		if action.Type != step.Type {
			return nil, fmt.Errorf("macro step %d: action %d is %s, not %s", index+1, step.ID, action.Type, step.Type)
		}
		if step.Value < 0 || step.Value > MaxActionValue {
			return nil, fmt.Errorf("macro step %d: value must be between 0 and %d", index+1, MaxActionValue)
		}
	}
	return actionByID, nil
}
