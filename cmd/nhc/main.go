// main.go
package main

import (
	"flag"
	"fmt"
	"log"
	. "nhc-go-client"
	"nhc-go-client/internal/curve"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
)

const (
	appName    = "Niko Home Control Client"
	appVersion = "0.1.0"
)

func main() {
	command, args, options, err := parseCommand()
	if err != nil {
		log.Fatal(err)
	}
	if command == "help" || command == "-h" || command == "--help" {
		printUsage()
		return
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Setup logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmsgprefix)
	log.SetPrefix("[NHC] ")

	// Print app header
	printHeader()

	// Load configuration
	config, err := LoadConfig(options...)
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
	if command == "alias" {
		if err := handleAliasCommand(args, config); err != nil {
			log.Fatal(err)
		}
		return
	}
	if command == "status" {
		command = ""
	}

	// Create client with loaded config
	client, err := NewClient(Config{
		IP:              config.IP,
		Port:            config.Port,
		Timeout:         config.Timeout,
		BrightnessCurve: mustBrightnessMapper(config),
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
	printActions(w, actions, locations, config)

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

func mustBrightnessMapper(config *NikoConfig) curve.Mapper {
	mapper, err := config.Brightness.Mapper()
	if err != nil {
		log.Fatalf("invalid brightness curve: %v", err)
	}
	return mapper
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
						state = fmt.Sprintf("On (%d%%)", action.Value1)
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

func parseCommand() (string, []string, []ConfigOption, error) {
	flags := flag.NewFlagSet("nhc", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {}
	ip := flags.String("ip", "", "Niko Home Control IP address")
	port := flags.Int("port", 0, "Niko Home Control TCP port")
	timeout := flags.Duration("timeout", 0, "request timeout, for example 5s")
	brightnessCurve := flags.String("brightness-curve", "", "brightness curve: linear, gamma, or lookup")
	gamma := flags.Float64("brightness-gamma", 0, "gamma exponent for the gamma brightness curve")
	debug := flags.Bool("debug", false, "enable debug logging")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return "help", nil, nil, nil
		}
		return "", nil, nil, err
	}

	var options []ConfigOption
	if *brightnessCurve != "" {
		options = append(options, WithBrightnessCurve(*brightnessCurve))
	}
	if *gamma != 0 {
		options = append(options, WithBrightnessGamma(*gamma))
	}
	if *ip != "" {
		options = append(options, WithIP(*ip))
	}
	if *port != 0 {
		options = append(options, WithPort(*port))
	}
	if *timeout != 0 {
		options = append(options, WithTimeout(*timeout))
	}
	if *debug {
		os.Setenv("NHC_DEBUG", "1")
	}

	args := flags.Args()
	if len(args) == 0 {
		return "", nil, options, nil
	}
	return args[0], args[1:], options, nil
}

func printUsage() {
	fmt.Println("Niko Home Control Client")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  nhc [flags]                 Show system status")
	fmt.Println("  nhc [flags] status          Show system status")
	fmt.Println("  nhc [flags] light on <id|alias> [brightness]")
	fmt.Println("  nhc [flags] light off <id|alias>")
	fmt.Println("  nhc [flags] scene on <id|alias>")
	fmt.Println("  nhc [flags] socket off <id|alias>")
	fmt.Println("  nhc alias add <type> <id> <alias>")
	fmt.Println("  nhc alias remove <type> <alias>")
	fmt.Println("  nhc alias list [type]")
	fmt.Println("  nhc macro list")
	fmt.Println("  nhc macro validate <name>")
	fmt.Println("  nhc macro run <name>")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -ip string       override the configured controller IP")
	fmt.Println("  -port int        override the configured TCP port")
	fmt.Println("  -timeout duration  override the request timeout")
	fmt.Println("  -debug           enable debug logging")
}

func handleCommand(command string, args []string, config *NikoConfig, client *Client) error {
	switch command {
	case "alias":
		return handleAliasCommand(args, config)
	case "macro":
		return handleMacroCommand(args, config, client)
	case "light", "scene", "socket":
		return handleDeviceCommand(command, args, config, client)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func handleMacroCommand(args []string, config *NikoConfig, client *Client) error {
	if len(args) == 1 && args[0] == "list" {
		names := make([]string, 0, len(config.Macros))
		for name := range config.Macros {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: nhc macro <list|validate|run> [name]")
	}
	macro, ok := config.Macros[args[1]]
	if !ok {
		return fmt.Errorf("macro not found: %s", args[1])
	}
	switch args[0] {
	case "validate":
		return client.ValidateMacro(macro)
	case "run":
		return client.RunMacro(macro)
	default:
		return fmt.Errorf("unknown macro command: %s", args[0])
	}
}

func handleDeviceCommand(deviceType string, args []string, config *NikoConfig, client *Client) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("usage: nhc %s <on|off> <id|alias> [brightness]", deviceType)
	}

	action := args[0]
	idOrAlias := args[1]

	// Resolve ID from either direct ID or alias
	id, err := resolveID(config, deviceType, idOrAlias)
	if err != nil {
		return err
	}

	// Get the action to check if it exists and has the correct type
	actionObj, err := client.GetActionByID(id)
	if err != nil {
		return err
	}

	// Verify the action type matches the command
	if string(actionObj.Type) != strings.ToUpper(deviceType) {
		return fmt.Errorf("device with ID %d is a %s, not a %s", id, actionObj.Type, strings.ToUpper(deviceType))
	}

	switch action {
	case "on":
		if len(args) == 3 {
			if deviceType != "light" {
				return fmt.Errorf("brightness can only be set for lights")
			}
			brightness, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid brightness %q: %w", args[2], err)
			}
			return actionObj.TurnOn(brightness)
		}
		return actionObj.TurnOn()
	case "off":
		if len(args) == 3 {
			return fmt.Errorf("brightness can only be used with 'light on'")
		}
		return actionObj.TurnOff()
	default:
		return fmt.Errorf("unknown action: %s (use 'on' or 'off')", action)
	}
}
