package rogen

import (
	"fmt"
	"strings"
)

type cliArgs struct {
	help         bool
	initConfig   bool
	watch        bool
	version      bool
	verbatim     *bool
	config       string
	modes        []string
	environments []string
	template     string
	output       string
	build        string
	sources      []string
}

const helpText = `
Rogen - A tool for feature-based folder structures with Rojo.

Usage:
  rogen [command] [options]

Commands:
  init                  Generate a default .rogen.json config file
  watch                 Watch the source directories and regenerate automatically
  help                  Show this help menu
  version               Print the rogen version

Options:
  -h, --help            Show this help menu
  -v, --version         Print the rogen version
  -i, --init            Generate a default .rogen.json config file
  -w, --watch           Watch the source directories for changes and regenerate automatically
  -c, --config <path>   Specify a custom Rogen config file path
  -m, --mode <mode>     Specify a mode to run (repeatable)
  -e, --env <env>       Activate an environment (repeatable)
  -s, --source <path>   Override the directory containing uncompiled code (repeatable)
  -t, --template <path> Specify a JSON file containing the base Rojo tree
  -b, --build <path>    Override the directory where compiled code lands
  -o, --output <path>   Override the generated Rojo project file
  -k, --verbatim        Preserve routing affixes except exact .server/.client suffixes

The legacy --keepRouteNames spelling remains accepted.
Paths inside the config file resolve relative to the config file's directory.
The build directory and template $path values resolve relative to the generated
output file, exactly as Rojo itself interprets them.
`

func printHelp() {
	fmt.Print(helpText)
}

// parseCliArgs parses command line arguments. Both "--flag value" and
// "--flag=value" forms are accepted; unknown flags and commands are errors.
func parseCliArgs(args []string) (*cliArgs, error) {
	parsed := &cliArgs{}
	commandSeen := false

	canonical := map[string]string{
		"h": "help", "help": "help",
		"v": "version", "version": "version",
		"i": "init", "init": "init",
		"w": "watch", "watch": "watch",
		"k": "verbatim", "verbatim": "verbatim", "keepRouteNames": "verbatim",
		"c": "config", "config": "config",
		"m": "mode", "mode": "mode",
		"e": "env", "env": "env",
		"s": "source", "source": "source",
		"t": "template", "template": "template",
		"b": "build", "build": "build",
		"o": "output", "output": "output",
	}
	takesValue := map[string]bool{
		"config": true, "mode": true, "env": true, "source": true,
		"template": true, "build": true, "output": true,
	}

	setCommand := func(command string) error {
		if commandSeen {
			return fmt.Errorf("unexpected command %q (only one command is allowed)", command)
		}
		commandSeen = true
		switch strings.ToLower(command) {
		case "init":
			parsed.initConfig = true
		case "watch":
			parsed.watch = true
		case "help":
			parsed.help = true
		case "version":
			parsed.version = true
		default:
			return fmt.Errorf("unknown command %q (see --help)", command)
		}
		return nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			if err := setCommand(arg); err != nil {
				return nil, err
			}
			continue
		}

		name := strings.TrimLeft(arg, "-")
		value := ""
		hasInlineValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value = name[eq+1:]
			name = name[:eq]
			hasInlineValue = true
		}

		flag, ok := canonical[name]
		if !ok {
			return nil, fmt.Errorf("unknown option %q (see --help)", arg)
		}

		if takesValue[flag] {
			if !hasInlineValue {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
					return nil, fmt.Errorf("option %q requires a value", arg)
				}
				i++
				value = args[i]
			}
			if value == "" {
				return nil, fmt.Errorf("option %q requires a non-empty value", arg)
			}
		} else if hasInlineValue {
			return nil, fmt.Errorf("option %q does not take a value", arg)
		}

		switch flag {
		case "help":
			parsed.help = true
		case "version":
			parsed.version = true
		case "init":
			parsed.initConfig = true
		case "watch":
			parsed.watch = true
		case "verbatim":
			yes := true
			parsed.verbatim = &yes
		case "config":
			parsed.config = value
		case "mode":
			parsed.modes = append(parsed.modes, value)
		case "env":
			parsed.environments = append(parsed.environments, value)
		case "source":
			parsed.sources = append(parsed.sources, value)
		case "template":
			parsed.template = value
		case "build":
			parsed.build = value
		case "output":
			parsed.output = value
		}
	}

	return parsed, nil
}
