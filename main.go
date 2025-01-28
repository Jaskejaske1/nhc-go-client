// main.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
)

const (
	appName    = "Niko Home Control Client"
	appVersion = "0.1.0"
)

func main() {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Setup logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmsgprefix)
	log.SetPrefix("[NHC] ")

	// Print app header
	printHeader()

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		if err == ErrNoIPAddress {
			// No IP configured, let's create a new config
			config = &NikoConfig{
				Port:    DefaultPort,
				Timeout: DefaultTimeout,
			}

			fmt.Print("No configuration found. Enter Niko Home Control IP address: ")
			var ip string
			fmt.Scanln(&ip)

			if ip == "" {
				log.Fatal("IP address is required")
			}

			config.IP = ip

			if err := config.SaveConfig(); err != nil {
				log.Fatalf("Failed to save config: %v", err)
			}
			fmt.Printf("Configuration saved to %s\n\n", mustGetConfigPath())
		} else {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Handle alias commands if present
	if len(os.Args) > 1 && os.Args[1] == "alias" {
		if err := handleAliasCommand(os.Args[2:], config); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Create client with loaded config
	client, err := NewClient(Config{
		IP:      config.IP,
		Port:    config.Port,
		Timeout: config.Timeout,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Enable debug logging if environment variable is set
	if os.Getenv("NHC_DEBUG") != "" {
		client.SetLogLevel(LogLevelDebug)
	}

	// Parse and handle commands
	command, args := parseCommand()
	if command != "" {
		if err := handleCommand(command, args, config, client); err != nil {
			log.Fatal(err)
		}
		return
	}

	// If no command provided, continue with the existing status display
	// Setup tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Get and print system info
	sysInfo, err := client.GetSystemInfo()
	if err != nil {
		log.Fatal(err)
	}
	printSystemInfo(sysInfo)

	// Get and print locations
	locations, err := client.GetLocations()
	if err != nil {
		log.Fatal(err)
	}
	printLocations(w, locations)

	// Get and print actions
	actions, err := client.GetActions()
	if err != nil {
		log.Fatal(err)
	}
	printActions(w, actions, locations) // Pass locations to printActions

	// Get and print thermostats
	thermostats, err := client.GetThermostats()
	if err != nil {
		log.Fatal(err)
	}
	printThermostats(w, thermostats)

	// Flush the tabwriter
	w.Flush()

	// Only wait for signals if running in daemon mode
	if os.Getenv("NHC_DAEMON") == "1" {
		fmt.Println("\nRunning in daemon mode. Press Ctrl+C to exit...")
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
	}
}

// Helper function to get config path without error
func mustGetConfigPath() string {
	path, err := GetConfigPath()
	if err != nil {
		return "~/.config/nhc-go-client/config.json"
	}
	return path
}

func printHeader() {
	fmt.Printf("%s v%s\n", appName, appVersion)
	fmt.Println(strings.Repeat("-", 40))
}

func printSystemInfo(info map[string]interface{}) {
	fmt.Println("System Information:")
	fmt.Println(strings.Repeat("-", 20))

	// Sort keys for consistent output
	var keys []string
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		value := info[k]
		if k == "time" {
			if timeStr, ok := value.(string); ok {
				value = formatSystemTime(timeStr)
			}
		}
		fmt.Printf("%-15s: %v\n", k, value)
	}
	fmt.Println()
}

func printLocations(w *tabwriter.Writer, locations []Location) {
	fmt.Println("Locations:")
	fmt.Println(strings.Repeat("-", 20))

	fmt.Fprintln(w, "ID\tName")
	fmt.Fprintln(w, strings.Repeat("-", 20))

	// Sort locations by ID
	sort.Slice(locations, func(i, j int) bool {
		return locations[i].ID < locations[j].ID
	})

	for _, loc := range locations {
		if loc.Name == "" {
			loc.Name = "(default)"
		}
		fmt.Fprintf(w, "%d\t%s\n", loc.ID, loc.Name)
	}
	fmt.Fprintln(w)
	w.Flush()
}

func printActions(w *tabwriter.Writer, actions []Action, locations []Location, config *NikoConfig) {
	fmt.Println("Actions:")
	fmt.Println(strings.Repeat("-", 20))

	fmt.Fprintln(w, "ID\tName\tLocation\tType\tState\tAlias")
	fmt.Fprintln(w, strings.Repeat("-", 60))

	// Create location map for quick lookups
	locationMap := make(map[int]string)
	for _, loc := range locations {
		name := loc.Name
		if name == "" {
			name = "(default)"
		}
		locationMap[loc.ID] = name
	}

	// Group actions by type
	typeGroups := make(map[ActionType][]Action)
	for _, action := range actions {
		typeGroups[action.Type] = append(typeGroups[action.Type], action)
	}

	// Sort within each group by location, then name
	for _, actions := range typeGroups {
		sort.Slice(actions, func(i, j int) bool {
			if actions[i].Location != actions[j].Location {
				return actions[i].Location < actions[j].Location
			}
			return actions[i].Name < actions[j].Name
		})
	}

	// Print actions by type
	for _, actionType := range []ActionType{ActionTypeLight, ActionTypeScene, ActionTypeSocket} {
		if actions, ok := typeGroups[actionType]; ok {
			fmt.Fprintf(w, "--- %s ---\n", actionType)
			for _, action := range actions {
				state := "Off"
				if action.IsOn() {
					if action.Type == ActionTypeLight {
						state = fmt.Sprintf("On (%d%%)", int(float64(action.Value1)/255*100))
					} else {
						state = "On"
					}
				}
				locationName := locationMap[action.Location]

				// Get alias if exists
				alias := config.GetDeviceAlias(string(action.Type), action.ID)
				aliasStr := ""
				if alias != "" {
					aliasStr = fmt.Sprintf("(%s)", alias)
				}

				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
					action.ID,
					action.Name,
					locationName,
					action.Type,
					state,
					aliasStr)
			}
		}
	}
}

func printThermostats(w *tabwriter.Writer, thermostats []Thermostat) {
	if len(thermostats) == 0 {
		return
	}

	fmt.Println("Thermostats:")
	fmt.Println(strings.Repeat("-", 20))

	fmt.Fprintln(w, "ID\tName\tMeasured\tSetpoint\tMode\tEco")
	fmt.Fprintln(w, strings.Repeat("-", 50))

	for _, t := range thermostats {
		fmt.Fprintf(w, "%d\t%s\t%.1f°C\t%.1f°C\t%d\t%v\n",
			t.ID,
			t.Name,
			t.GetFormattedTemperature(t.Measured),
			t.GetFormattedTemperature(t.Setpoint),
			t.Mode,
			t.Ecosave,
		)
	}
	fmt.Fprintln(w)
	w.Flush()
}

func formatSystemTime(timeStr string) string {
	// Expected format: "YYYYMMDDHHMMSS"
	if len(timeStr) != 14 {
		return timeStr
	}
	return fmt.Sprintf("%s-%s-%s %s:%s:%s",
		timeStr[0:4],   // YYYY
		timeStr[4:6],   // MM
		timeStr[6:8],   // DD
		timeStr[8:10],  // HH
		timeStr[10:12], // MM
		timeStr[12:14], // SS
	)
}

func parseCommand() (command string, args []string) {
	if len(os.Args) < 2 {
		return "", nil
	}
	return os.Args[1], os.Args[2:]
}

func handleCommand(command string, args []string, config *NikoConfig, client *Client) error {
	switch command {
	case "alias":
		return handleAliasCommand(args, config)
	case "light", "scene", "socket":
		return handleDeviceCommand(command, args, config, client)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func handleDeviceCommand(deviceType string, args []string, config *NikoConfig, client *Client) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: nhc %s <on|off> <id|alias>", deviceType)
	}

	action := args[0]
	idOrAlias := args[1]

	// Resolve ID from either direct ID or alias
	id, err := resolveID(config, deviceType, idOrAlias)
	if err != nil {
		return err
	}

	switch action {
	case "on":
		return client.TurnOn(id)
	case "off":
		return client.TurnOff(id)
	default:
		return fmt.Errorf("unknown action: %s (use 'on' or 'off')", action)
	}
}
