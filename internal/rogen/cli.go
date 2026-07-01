package rogen

import (
	"fmt"
	"strings"
)

type cliArgs struct {
	help           bool
	initConfig     bool
	watch          bool
	version        bool
	keepRouteNames *bool
	config         string
	mode           string
	template       string
	output         string
	build          string
	sources        []string
}

const helpText = `
Rogen - A tool for feature-based folder structures with Rojo.

Usage:
  rogen [options]

Options:
  -h, --help            Show this help menu
  -v, --version         Print the rogen version
  -i, --init            Generate a default .rogen.json config file
  -w, --watch           Watch the source directories for changes and regenerate automatically
  -c, --config <path>   Specify a custom Rogen config file path
  -m, --mode <mode>     Specify the mode to run (luau, ts, darklua, or a custom mode)
  -s, --source <path>   Override the directory containing your uncompiled code (repeatable)
  -t, --template <path> Specify a path to a JSON file containing your base Rojo tree
  -b, --build <path>    Override the directory where your compiled/transpiled code lands
  -o, --output <path>   Override the name and destination of the final generated Rojo project file
  -k, --keepRouteNames  Do not strip routing prefixes or suffixes (e.g., server, client) from names

Paths inside the config file resolve relative to the config file's directory.
The build directory and template $path values resolve relative to the generated
output file, exactly as Rojo itself interprets them.
`

func printHelp() {
	fmt.Print(helpText)
}

// parseCliArgs parses command line arguments. Both "--flag value" and
// "--flag=value" forms are accepted; unknown flags are an error.
func parseCliArgs(args []string) (*cliArgs, error) {
	parsed := &cliArgs{}

	canonical := map[string]string{
		"h": "help", "help": "help",
		"v": "version", "version": "version",
		"i": "init", "init": "init",
		"w": "watch", "watch": "watch",
		"k": "keepRouteNames", "keepRouteNames": "keepRouteNames",
		"c": "config", "config": "config",
		"m": "mode", "mode": "mode",
		"s": "source", "source": "source",
		"t": "template", "template": "template",
		"b": "build", "build": "build",
		"o": "output", "output": "output",
	}
	takesValue := map[string]bool{
		"config": true, "mode": true, "source": true,
		"template": true, "build": true, "output": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("unexpected argument %q (rogen takes options only; see --help)", arg)
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
				if i+1 >= len(args) {
					return nil, fmt.Errorf("option %q requires a value", arg)
				}
				i++
				value = args[i]
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
		case "keepRouteNames":
			yes := true
			parsed.keepRouteNames = &yes
		case "config":
			parsed.config = value
		case "mode":
			parsed.mode = value
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
