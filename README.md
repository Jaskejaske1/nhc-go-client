# Niko Home Control v1 client

A small command-line client for controlling legacy Niko Home Control v1 installations over the controller's TCP API.

The client can display controller status, list devices, control lights/scenes/sockets, and store friendly aliases for frequently used actions.

## Requirements

- Niko Home Control v1 controller reachable over TCP
- Go 1.23 or newer to build from source
- Network access to the controller, normally on port `8000`

The controller must be reachable from the machine running the client. This project does not use the Niko cloud service and does not support Niko Home Control II.

## Build and run

Run directly from the repository:

```text
go run ./cmd/nhc -ip 192.168.1.20
```

Build the CLI executable:

```text
go build -o nhc.exe ./cmd/nhc
```

On Windows, this creates `nhc.exe` when an `.exe` output name is preferred:

```text
go build -o nhc.exe ./cmd/nhc
```

Build the browser dashboard:

```text
go build -o nhc-gui.exe ./cmd/nhc-gui
```

## First run

Give the controller address with `-ip` for a one-off command:

```text
nhc -ip 192.168.1.20
```

When no address is configured, running the client without `-ip` prompts for the address and saves the resulting configuration to:

```text
~/.config/nhc-go-client/config.json
```

On Windows, `~` means the current user's home directory. The path is still `.config/nhc-go-client/config.json` beneath that directory.

## Command reference

### Show status

With no command, the client prints system information, locations, actions, and thermostats:

```text
nhc
nhc status
```

The explicit `status` form is recommended for scripts and future CLI compatibility.

### Control a device

Device types are `light`, `scene`, and `socket`. The action is `on` or `off`.

```text
nhc light on 12
nhc light on 12 35
nhc light off 12
nhc scene on 34
nhc socket off 56
```

The final argument can be a numeric action ID or an alias. The client reads the current action list before changing a device and rejects a command when the ID belongs to a different device type.

Action values use the controller's 0-100 scale. A light `on` command uses `100` for full brightness; dimming values are also represented as percentages.

The client can apply a configured brightness curve before sending the controller value. The default `linear` curve preserves the raw percentage behavior.

For a dimmable light, add a brightness percentage from `0` to `100`:

```text
nhc light on 13 35
nhc light on kitchen 60
```

`light off` turns the action off. Turning a light on without a percentage uses full brightness.

### Custom scenes

Existing scenes configured in the Niko controller can be triggered with the same command:

```text
nhc scene on 19
nhc scene on movie-night
```

This client cannot currently create or edit controller-native scenes. The v1 command surface implemented here exposes listing and executing actions, but no scene-definition operation. A local macro feature could run a named sequence of existing actions, but it would not appear as a native scene in the Niko controller.

Local macros are stored in the client configuration and execute existing actions sequentially:

```text
nhc macro list
nhc macro validate movie-night
nhc macro run movie-night
```

Macros must be defined in the `macros` configuration object. Each step uses an action ID, its expected type, and a value from `0` to `100`:

```json
{
  "macros": {
    "movie-night": {
      "steps": [
        { "id": 13, "type": "LIGHT", "value": 35 },
        { "id": 7, "type": "SOCKET", "value": 0 },
        { "id": 19, "type": "SCENE", "value": 100 }
      ]
    }
  }
}
```

Macros are local client routines, not controller-native scenes. They validate action IDs and types against a fresh controller snapshot before executing.

### Manage aliases

Add an alias:

```text
nhc alias add light 12 kitchen
nhc alias add scene 34 movie-night
nhc alias add socket 56 coffee-machine
```

Use an alias in a command:

```text
nhc light on kitchen
nhc scene on movie-night
```

List all aliases or one device type:

```text
nhc alias list
nhc alias list light
```

Remove an alias by its name:

```text
nhc alias remove light kitchen
```

Aliases are local names stored in the configuration file. They are not written to the Niko controller.

### Help and diagnostics

```text
nhc help
nhc -h
nhc -debug
```

`-debug` enables connection, command, retry, and response logging. The same setting can be enabled with `NHC_DEBUG=1`.

## Configuration

Configuration values are applied in this order, with later sources overriding earlier ones:

1. Built-in defaults
2. Configuration file
3. Environment variables
4. Command-line flags

The defaults are:

| Setting | Default |
| --- | --- |
| Port | `8000` |
| Timeout | `20s` |
| Retry attempts | `3` |
| Retry delay | `1s`, increasing per attempt |

### Command-line flags

```text
nhc -ip 192.168.1.20 -port 8000 -timeout 5s
```

Available flags:

| Flag | Description |
| --- | --- |
| `-ip` | Controller IPv4 or IPv6 address |
| `-port` | Controller TCP port |
| `-timeout` | Request timeout such as `5s` or `1m` |
| `-debug` | Enable debug logging |
| `-brightness-curve` | `linear`, `gamma`, or `lookup` |
| `-brightness-gamma` | Gamma exponent when using `gamma` |

Flags must appear before the command, for example `nhc -ip 192.168.1.20 light on 12`.

### Environment variables

```text
NHC_IP=192.168.1.20
NHC_PORT=8000
NHC_TIMEOUT=5s
NHC_DEBUG=1
NHC_BRIGHTNESS_CURVE=gamma
NHC_BRIGHTNESS_GAMMA=2.0
```

On PowerShell:

```powershell
$env:NHC_IP = "192.168.1.20"
$env:NHC_TIMEOUT = "5s"
nhc light on 12
```

### Configuration file

The file uses JSON. A minimal configuration is:

```json
{
  "ip": "192.168.1.20",
  "port": 8000,
  "timeout": "20s",
  "brightness": {
    "type": "gamma",
    "gamma": 2.0
  }
}
```

Aliases can be included in the same file:

```json
{
  "ip": "192.168.1.20",
  "port": 8000,
  "timeout": "20s",
  "aliases": {
    "lights": {
      "12": "kitchen"
    },
    "scenes": {
      "34": "movie-night"
    },
    "sockets": {
      "56": "coffee-machine"
    }
  }
}
```

`example_config.json` contains a minimal example without aliases. The client creates the configuration directory when saving the first configuration.

For a measured calibration table, use `lookup` with points from user-facing percentage to controller output:

```json
{
  "brightness": {
    "type": "lookup",
    "points": [
      { "input": 0, "output": 0 },
      { "input": 25, "output": 8 },
      { "input": 50, "output": 25 },
      { "input": 75, "output": 55 },
      { "input": 100, "output": 100 }
    ]
  }
}
```

The points must start and end at `0` and `100`, and outputs must be monotonic.
These curves correct the requested controller level; choosing a physically accurate curve still requires measuring your specific dimmer and light at fixed levels.

## GUI dashboard

Start the local browser dashboard after building `nhc-gui.exe`:

```text
nhc-gui.exe
```

Open `http://127.0.0.1:8765`. Choose another bind address with `-addr`, for example `nhc-gui.exe -addr 127.0.0.1:9000`.

The dashboard groups actions by room, shows current action state, provides brightness sliders for lights, and refreshes a cached controller snapshot every five seconds. Browser requests return the latest snapshot immediately instead of waiting for three controller queries. It is intentionally bound to localhost by default.

## Source layout

```text
client.go              reusable Niko v1 client package
config.go              shared configuration and persistence
internal/curve         brightness curve and calibration logic
cmd/nhc                command-line executable
cmd/nhc-gui            browser dashboard executable
```

The root package is now importable as `nhc-go-client`; both executables use the same client and configuration code.

## Troubleshooting

### No IP address configured

Use `-ip`, set `NHC_IP`, or run the client interactively once so it can save the address:

```text
nhc -ip 192.168.1.20
```

### Invalid configuration

The IP must be a numeric IP address, the port must be between `1` and `65535`, and the timeout must be at least one second. Check the JSON duration format, for example `20s`, not `20`.

### Connection failures

Confirm that the controller is powered on and reachable, then verify the port:

```text
nhc -ip 192.168.1.20 -port 8000 -timeout 5s -debug
```

The client retries failed reads and writes and reconnects between attempts. It does not discover controllers automatically.

### Unknown action or alias

Run `nhc` to inspect the action IDs reported by the controller, or run `nhc alias list` to inspect local aliases. Aliases are case-sensitive and are scoped to their device type.

## Development

Run formatting, tests, and static checks from the repository root:

```text
gofmt -w *.go
go test ./...
go vet ./...
```

The repository contains curve unit tests but no automated device-integration tests, so commands that change controller state should be verified against a reachable test or production controller with care.

## Implementation notes

The client communicates directly with the Niko Home Control v1 TCP endpoint using JSON commands terminated by a carriage return. The current repository is an executable Go package (`package main`); the exported client types and methods are organized in `client.go`, but the project is not currently published as an importable Go library.
