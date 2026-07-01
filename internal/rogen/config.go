package rogen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode is one output pipeline: where compiled code lands and which project
// file to generate. Both values are Rojo-facing and resolve relative to the
// generated output file's directory (build) or the config anchor (output).
type Mode struct {
	Output string
	Build  string
}

// Config is a parsed and validated .rogen.json.
type Config struct {
	// Anchor is the directory all config-relative paths resolve against:
	// the config file's directory, or the cwd when no config file exists.
	Anchor string

	Sources        []string
	KeepRouteNames *bool
	Aliases        map[string]string
	Modes          map[string]Mode
	ModeOrder      []string
	// Template is either a map[string]any (inline tree) or a string path
	// relative to Anchor; nil when unset.
	Template any
}

type environment struct {
	isTsProject      bool
	isDarkluaProject bool
}

var nonModeKeys = map[string]bool{
	"source":         true,
	"template":       true,
	"aliases":        true,
	"keepRouteNames": true,
}

// resolveConfigPath finds the config file: an explicit --config path
// (cwd-relative), a .rogen.json in the cwd, or any *.rogen.json in the cwd.
func resolveConfigPath(customPathArg string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if customPathArg != "" {
		resolved := absJoin(cwd, customPathArg)
		if !fileExists(resolved) {
			return "", fmt.Errorf("specified config file not found: %s", customPathArg)
		}
		return resolved, nil
	}

	defaultPath := filepath.Join(cwd, ".rogen.json")
	if fileExists(defaultPath) {
		return defaultPath, nil
	}

	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "", nil
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rogen.json") {
			return filepath.Join(cwd, entry.Name()), nil
		}
	}

	return "", nil
}

// loadConfig parses and validates the config file at configPath. An empty
// configPath yields the built-in defaults anchored at the cwd.
func loadConfig(configPath string) (cfg *Config, hasConfig bool, err error) {
	if configPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, false, err
		}
		cfg, err := parseConfig([]byte(defaultConfigJSON), cwd)
		return cfg, false, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false, err
	}
	cfg, err = parseConfig(data, filepath.Dir(configPath))
	if err != nil {
		return nil, false, fmt.Errorf("in %s: %w", configPath, err)
	}
	return cfg, true, nil
}

func parseConfig(data []byte, anchor string) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("configuration is not valid JSON: %w", err)
	}

	cfg := &Config{
		Anchor:  anchor,
		Aliases: map[string]string{},
		Modes:   map[string]Mode{},
	}

	for _, key := range topLevelKeyOrder(data) {
		value := raw[key]
		switch key {
		case "source":
			var single string
			if err := json.Unmarshal(value, &single); err == nil {
				cfg.Sources = []string{single}
				continue
			}
			var many []string
			if err := json.Unmarshal(value, &many); err != nil {
				return nil, fmt.Errorf("configuration error: 'source' must be a string or an array of strings")
			}
			cfg.Sources = many
		case "keepRouteNames":
			var flag bool
			if err := json.Unmarshal(value, &flag); err != nil {
				return nil, fmt.Errorf("configuration error: 'keepRouteNames' must be a boolean")
			}
			cfg.KeepRouteNames = &flag
		case "aliases":
			if err := json.Unmarshal(value, &cfg.Aliases); err != nil {
				return nil, fmt.Errorf("configuration error: 'aliases' must map keywords to service names")
			}
		case "template":
			var asString string
			if err := json.Unmarshal(value, &asString); err == nil {
				cfg.Template = asString
				continue
			}
			tree, err := decodeJSONValue(value)
			if err != nil {
				return nil, err
			}
			if _, ok := tree.(map[string]any); !ok {
				return nil, fmt.Errorf("configuration error: 'template' must be an inline object or a string path to a JSON file")
			}
			cfg.Template = tree
		case "keepSuffixes":
			return nil, fmt.Errorf("configuration error: 'keepSuffixes' was renamed to 'keepRouteNames'")
		default:
			var mode struct {
				Output *string `json:"output"`
				Build  *string `json:"build"`
			}
			if err := json.Unmarshal(value, &mode); err != nil || bytes.HasPrefix(bytes.TrimSpace(value), []byte("[")) {
				return nil, fmt.Errorf("configuration error: key %q must be a valid object defining a mode", key)
			}
			if mode.Output == nil || *mode.Output == "" {
				return nil, fmt.Errorf("configuration error: mode %q is missing a valid \"output\" string", key)
			}
			if mode.Build == nil || *mode.Build == "" {
				return nil, fmt.Errorf("configuration error: mode %q is missing a valid \"build\" string", key)
			}
			cfg.Modes[key] = Mode{Output: *mode.Output, Build: *mode.Build}
			cfg.ModeOrder = append(cfg.ModeOrder, key)
		}
	}

	if len(cfg.Sources) == 0 {
		cfg.Sources = []string{"src"}
	}

	return cfg, nil
}

// topLevelKeyOrder returns the top-level object keys in file order so that
// multi-mode configs execute their modes deterministically as written.
func topLevelKeyOrder(data []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var keys []string
	depth := 0
	expectKey := false

	for {
		token, err := decoder.Token()
		if err != nil {
			return keys
		}
		switch t := token.(type) {
		case json.Delim:
			switch t {
			case '{':
				depth++
				expectKey = depth == 1
			case '}':
				depth--
				expectKey = false
			case '[', ']':
				expectKey = false
			}
		case string:
			if depth == 1 && expectKey {
				keys = append(keys, t)
				// The next token is this key's value; skip it entirely.
				var skipped json.RawMessage
				if err := decoder.Decode(&skipped); err != nil {
					return keys
				}
			}
		}
	}
}

// getEnvironment determines the project language. An explicit --mode is
// authoritative; otherwise the language markers next to the config anchor
// decide (tsconfig.json, .darklua.json/.darklua.json5).
func getEnvironment(anchor string, cliMode string) environment {
	if cliMode != "" {
		return environment{
			isTsProject:      cliMode == "ts",
			isDarkluaProject: cliMode == "darklua",
		}
	}
	return environment{
		isTsProject:      fileExists(filepath.Join(anchor, "tsconfig.json")),
		isDarkluaProject: fileExists(filepath.Join(anchor, ".darklua.json")) || fileExists(filepath.Join(anchor, ".darklua.json5")),
	}
}

// resolveActiveModes picks which mode pipelines run.
func resolveActiveModes(cfg *Config, hasConfig bool, cliMode string, env environment) ([]Mode, error) {
	if hasConfig {
		if cliMode != "" {
			mode, ok := cfg.Modes[cliMode]
			if !ok {
				return nil, fmt.Errorf("mode %q is not defined in your config file", cliMode)
			}
			return []Mode{mode}, nil
		}

		var active []Mode
		for _, key := range cfg.ModeOrder {
			if key == "luau" && env.isTsProject {
				continue
			}
			if key == "ts" && !env.isTsProject {
				continue
			}
			if key == "darklua" && !env.isDarkluaProject {
				continue
			}
			active = append(active, cfg.Modes[key])
		}
		if len(active) == 0 {
			return nil, fmt.Errorf("no output modes defined in configuration file; add 'luau', 'ts', or custom modes")
		}
		return active, nil
	}

	base := defaultModes["luau"]
	if env.isTsProject {
		base = defaultModes["ts"]
	}

	if cliMode != "" {
		fallback, ok := defaultModes[cliMode]
		if !ok {
			return nil, fmt.Errorf("mode %q is not defined in the fallback config", cliMode)
		}
		return []Mode{fallback}, nil
	}

	active := []Mode{base}
	if env.isDarkluaProject {
		active = append(active, defaultModes["darklua"])
	}
	return active, nil
}

// loadProjectTree resolves the base Rojo tree template: an explicit
// --template path (cwd-relative), the config's template (inline object or
// anchor-relative file path), or the built-in default.
func loadProjectTree(cliTemplateArg string, cfg *Config) (map[string]any, error) {
	var targetPath string

	if cliTemplateArg != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		targetPath = absJoin(cwd, cliTemplateArg)
	} else if pathTemplate, ok := cfg.Template.(string); ok {
		targetPath = absJoin(cfg.Anchor, pathTemplate)
	}

	if targetPath != "" {
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, fmt.Errorf("specified template file not found: %s", targetPath)
		}
		return decodeJSONObject(data)
	}

	if inline, ok := cfg.Template.(map[string]any); ok {
		return deepCopyTree(inline), nil
	}

	defaults, err := parseConfig([]byte(defaultConfigJSON), cfg.Anchor)
	if err != nil {
		return nil, err
	}
	return defaults.Template.(map[string]any), nil
}

// decodeJSONValue decodes JSON preserving number literals via json.Number.
func decodeJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return value, nil
}

func decodeJSONObject(data []byte) (map[string]any, error) {
	value, err := decodeJSONValue(data)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return object, nil
}

func deepCopyTree(tree map[string]any) map[string]any {
	copied := make(map[string]any, len(tree))
	for key, value := range tree {
		switch v := value.(type) {
		case map[string]any:
			copied[key] = deepCopyTree(v)
		case []any:
			copiedSlice := make([]any, len(v))
			for i, item := range v {
				if m, ok := item.(map[string]any); ok {
					copiedSlice[i] = deepCopyTree(m)
				} else {
					copiedSlice[i] = item
				}
			}
			copied[key] = copiedSlice
		default:
			copied[key] = value
		}
	}
	return copied
}

// absJoin resolves p against base unless p is already absolute.
func absJoin(base, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
