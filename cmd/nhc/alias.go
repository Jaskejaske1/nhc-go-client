package main

import (
	"fmt"
	. "nhc-go-client"
	"strconv"
)

func handleAliasCommand(args []string, config *NikoConfig) error {
	if len(args) < 1 {
		return fmt.Errorf("alias command requires a subcommand: add, remove, or list")
	}

	switch args[0] {
	case "add":
		if len(args) != 4 {
			return fmt.Errorf("usage: nhc alias add <type> <id> <alias>")
		}
		deviceType := args[1]
		id, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid ID: %v", err)
		}
		alias := args[3]

		if err := config.SetDeviceAlias(deviceType, id, alias); err != nil {
			return err
		}
		return config.SaveConfig()

	case "remove":
		if len(args) != 3 {
			return fmt.Errorf("usage: nhc alias remove <type> <alias>")
		}
		deviceType := args[1]
		aliasToRemove := args[2]

		var found bool
		switch deviceType {
		case "light":
			for id, alias := range config.Aliases.Lights {
				if alias == aliasToRemove {
					delete(config.Aliases.Lights, id)
					found = true
					break
				}
			}
		case "scene":
			for id, alias := range config.Aliases.Scenes {
				if alias == aliasToRemove {
					delete(config.Aliases.Scenes, id)
					found = true
					break
				}
			}
		case "socket":
			for id, alias := range config.Aliases.Sockets {
				if alias == aliasToRemove {
					delete(config.Aliases.Sockets, id)
					found = true
					break
				}
			}
		default:
			return fmt.Errorf("invalid device type: %s", deviceType)
		}

		if !found {
			return fmt.Errorf("alias not found: %s", aliasToRemove)
		}
		return config.SaveConfig()

	case "list":
		deviceType := ""
		if len(args) > 1 {
			deviceType = args[1]
		}

		printAliases(config, deviceType)
		return nil

	default:
		return fmt.Errorf("unknown alias subcommand: %s", args[0])
	}
}

func printAliases(config *NikoConfig, deviceType string) {
	if deviceType == "" || deviceType == "light" {
		fmt.Println("Light Aliases:")
		for id, alias := range config.Aliases.Lights {
			fmt.Printf("  %d -> %s\n", id, alias)
		}
	}

	if deviceType == "" || deviceType == "scene" {
		fmt.Println("Scene Aliases:")
		for id, alias := range config.Aliases.Scenes {
			fmt.Printf("  %d -> %s\n", id, alias)
		}
	}

	if deviceType == "" || deviceType == "socket" {
		fmt.Println("Socket Aliases:")
		for id, alias := range config.Aliases.Sockets {
			fmt.Printf("  %d -> %s\n", id, alias)
		}
	}
}

// Helper function to get ID from alias or direct ID
func resolveID(config *NikoConfig, deviceType string, idOrAlias string) (int, error) {
	// First try to parse as direct ID
	if id, err := strconv.Atoi(idOrAlias); err == nil {
		return id, nil
	}

	// If not a number, look up the alias
	var id int
	found := false

	switch deviceType {
	case "light":
		for k, v := range config.Aliases.Lights {
			if v == idOrAlias {
				id = k
				found = true
				break
			}
		}
	case "scene":
		for k, v := range config.Aliases.Scenes {
			if v == idOrAlias {
				id = k
				found = true
				break
			}
		}
	case "socket":
		for k, v := range config.Aliases.Sockets {
			if v == idOrAlias {
				id = k
				found = true
				break
			}
		}
	default:
		return 0, fmt.Errorf("invalid device type: %s", deviceType)
	}

	if !found {
		return 0, fmt.Errorf("alias not found: %s", idOrAlias)
	}
	return id, nil
}
