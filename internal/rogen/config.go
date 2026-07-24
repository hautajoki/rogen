package rogen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Mode is one output pipeline: where compiled code lands and which project
// file to generate. Both values are Rojo-facing and resolve relative to the
// generated output file's directory (build) or the config anchor (output).
type Mode struct {
	// Name is populated when a mode is selected. It is not serialized.
	Name            string
	Output          string
	Build           string
	Environments    []string
	GlobIgnorePaths []string
}

type Casing string

const (
	CamelCase  Casing = "camelCase"
	PascalCase Casing = "PascalCase"
)

// Config is a parsed and validated .rogen.json.
type Config struct {
	// Anchor is the directory all config-relative paths resolve against:
	// the config file's directory, or the cwd when no config file exists.
	Anchor string

	Sources         []string
	Verbatim        bool
	Casing          Casing
	Unwrap          bool
	GlobIgnorePaths []string
	Aliases         map[string]string
	Modes           map[string]Mode
	ModeOrder       []string
	// Template is either a map[string]any (inline tree) or a string path
	// relative to Anchor; nil when unset.
	Template any
}

type environment struct {
	isTsProject      bool
	isDarkluaProject bool
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
		return "", fmt.Errorf("scan current directory for .rogen.json: %w", err)
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
		Casing:  CamelCase,
		Aliases: map[string]string{},
		Modes:   map[string]Mode{},
	}
	var verbatimValue *bool
	var keepRouteNamesValue *bool

	for _, key := range topLevelKeyOrder(data) {
		value := raw[key]
		switch key {
		case "source":
			if isJSONNull(value) {
				return nil, fmt.Errorf("configuration error: 'source' must be a non-empty string or an array of non-empty strings")
			}
			var single string
			if err := json.Unmarshal(value, &single); err == nil {
				if single == "" {
					return nil, fmt.Errorf("configuration error: 'source' must not be empty")
				}
				cfg.Sources = []string{single}
				continue
			}
			var many []string
			if err := json.Unmarshal(value, &many); err != nil {
				return nil, fmt.Errorf("configuration error: 'source' must be a string or an array of strings")
			}
			for _, source := range many {
				if source == "" {
					return nil, fmt.Errorf("configuration error: 'source' entries must not be empty")
				}
			}
			cfg.Sources = many
		case "verbatim", "keepRouteNames":
			if isJSONNull(value) {
				return nil, fmt.Errorf("configuration error: %q must be a boolean", key)
			}
			var flag bool
			if err := json.Unmarshal(value, &flag); err != nil {
				return nil, fmt.Errorf("configuration error: %q must be a boolean", key)
			}
			if key == "verbatim" {
				verbatimValue = &flag
				if keepRouteNamesValue != nil && *keepRouteNamesValue != flag {
					return nil, fmt.Errorf("configuration error: 'verbatim' conflicts with legacy 'keepRouteNames'")
				}
			} else {
				keepRouteNamesValue = &flag
				if verbatimValue != nil && *verbatimValue != flag {
					return nil, fmt.Errorf("configuration error: legacy 'keepRouteNames' conflicts with 'verbatim'")
				}
			}
			cfg.Verbatim = flag
		case "casing":
			var casing Casing
			if err := json.Unmarshal(value, &casing); err != nil || (casing != CamelCase && casing != PascalCase) {
				return nil, fmt.Errorf("configuration error: 'casing' must be either %q or %q", PascalCase, CamelCase)
			}
			cfg.Casing = casing
		case "unwrap":
			if isJSONNull(value) || json.Unmarshal(value, &cfg.Unwrap) != nil {
				return nil, fmt.Errorf("configuration error: 'unwrap' must be a boolean")
			}
		case "globIgnorePaths":
			if isJSONNull(value) || json.Unmarshal(value, &cfg.GlobIgnorePaths) != nil {
				return nil, fmt.Errorf("configuration error: 'globIgnorePaths' must be an array of strings")
			}
		case "aliases":
			if isJSONNull(value) || json.Unmarshal(value, &cfg.Aliases) != nil {
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
			return nil, fmt.Errorf("configuration error: 'keepSuffixes' was renamed to 'verbatim'")
		default:
			var mode struct {
				Output          *string  `json:"output"`
				Build           *string  `json:"build"`
				Environments    []string `json:"env"`
				GlobIgnorePaths []string `json:"globIgnorePaths"`
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
			cfg.Modes[key] = Mode{
				Name:            key,
				Output:          *mode.Output,
				Build:           *mode.Build,
				Environments:    mode.Environments,
				GlobIgnorePaths: mode.GlobIgnorePaths,
			}
			cfg.ModeOrder = append(cfg.ModeOrder, key)
		}
	}

	if len(cfg.Sources) == 0 {
		cfg.Sources = []string{"src"}
	}
	for modeName, mode := range cfg.Modes {
		for _, environment := range mode.Environments {
			if err := validateEnvironmentName(environment, cfg.Aliases); err != nil {
				return nil, fmt.Errorf("configuration error: mode %q: %w", modeName, err)
			}
		}
	}

	return cfg, nil
}

func isJSONNull(value []byte) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func validateEnvironmentName(environment string, aliases map[string]string) error {
	if environment == "" || strings.TrimSpace(environment) != environment {
		return fmt.Errorf("environment names must be non-empty and contain no surrounding whitespace")
	}
	if strings.ContainsAny(environment, ".-_+/\\") {
		return fmt.Errorf("environment %q contains a routing delimiter", environment)
	}
	lower := strings.ToLower(environment)
	if lower == "raw" || lower == "verbatim" || lower == "unwrap" {
		return fmt.Errorf("environment %q conflicts with a system marker", environment)
	}
	if generateRoutingMaps(aliases).lowerCaseMap[lower] != "" {
		return fmt.Errorf("environment %q conflicts with a routing keyword", environment)
	}
	return nil
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
func getEnvironment(anchor string, cliModes []string) environment {
	if len(cliModes) > 0 {
		return environment{
			isTsProject:      slices.Contains(cliModes, "ts"),
			isDarkluaProject: slices.Contains(cliModes, "darklua"),
		}
	}
	return environment{
		isTsProject:      fileExists(filepath.Join(anchor, "tsconfig.json")),
		isDarkluaProject: fileExists(filepath.Join(anchor, ".darklua.json")) || fileExists(filepath.Join(anchor, ".darklua.json5")),
	}
}

// resolveActiveModes picks which mode pipelines run.
func resolveActiveModes(cfg *Config, hasConfig bool, cliModes []string, env environment) ([]Mode, error) {
	if hasConfig && len(cfg.Modes) == 0 {
		if len(cliModes) > 0 {
			active := make([]Mode, 0, len(cliModes))
			for _, cliMode := range cliModes {
				mode, ok := defaultModes[cliMode]
				if !ok {
					return nil, fmt.Errorf("mode %q is not defined in your config file or the fallback config", cliMode)
				}
				active = append(active, mode)
			}
			return active, nil
		}
		base := defaultModes["luau"]
		if env.isTsProject {
			base = defaultModes["ts"]
		}
		active := []Mode{base}
		if env.isDarkluaProject {
			active = append(active, defaultModes["darklua"])
		}
		return active, nil
	}

	if hasConfig {
		if len(cliModes) > 0 {
			active := make([]Mode, 0, len(cliModes))
			for _, cliMode := range cliModes {
				mode, ok := cfg.Modes[cliMode]
				if !ok {
					return nil, fmt.Errorf("mode %q is not defined in your config file", cliMode)
				}
				active = append(active, mode)
			}
			return active, nil
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

	if len(cliModes) > 0 {
		active := make([]Mode, 0, len(cliModes))
		for _, cliMode := range cliModes {
			fallback, ok := defaultModes[cliMode]
			if !ok {
				return nil, fmt.Errorf("mode %q is not defined in the fallback config", cliMode)
			}
			active = append(active, fallback)
		}
		return active, nil
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

func createInitConfig(cwd string) map[string]any {
	tree := map[string]any{"$className": "DataModel"}
	template := map[string]any{
		"name": filepath.Base(cwd),
		"tree": tree,
	}
	config := map[string]any{
		"source":   []any{"src"},
		"template": template,
	}

	if fileExists(filepath.Join(cwd, "tsconfig.json")) {
		config["ts"] = map[string]any{"output": "default.project.json", "build": "out"}
		template["globIgnorePaths"] = []any{"**/package.json", "**/tsconfig.json"}

		include := map[string]any{"$path": "include"}
		nodeModules := map[string]any{"$className": "Folder"}
		for _, namespace := range []string{"@rbxts", "@flamework", "@rbxts-js"} {
			if fileExists(filepath.Join(cwd, "node_modules", namespace)) {
				nodeModules[namespace] = map[string]any{"$path": "node_modules/" + namespace}
			}
		}
		if len(nodeModules) > 1 {
			include["node_modules"] = nodeModules
		}
		tree["ReplicatedStorage"] = map[string]any{"rbxts_include": include}
	} else {
		config["luau"] = map[string]any{"output": "default.project.json", "build": "src"}
	}

	if fileExists(filepath.Join(cwd, ".darklua.json")) || fileExists(filepath.Join(cwd, ".darklua.json5")) {
		config["darklua"] = map[string]any{"output": "build.project.json", "build": "dist"}
	}

	replicatedStorage, _ := tree["ReplicatedStorage"].(map[string]any)
	if replicatedStorage == nil {
		replicatedStorage = map[string]any{}
	}
	serverScriptService := map[string]any{}

	if fileExists(filepath.Join(cwd, "wally.toml")) {
		if fileExists(filepath.Join(cwd, "Packages")) {
			replicatedStorage["Packages"] = map[string]any{"$path": "Packages"}
		}
		if fileExists(filepath.Join(cwd, "ServerPackages")) {
			serverScriptService["ServerPackages"] = map[string]any{"$path": "ServerPackages"}
		}
	}
	if fileExists(filepath.Join(cwd, "pesde.toml")) {
		if fileExists(filepath.Join(cwd, "roblox_packages")) {
			replicatedStorage["Packages"] = map[string]any{"$path": "roblox_packages"}
		}
		if fileExists(filepath.Join(cwd, "roblox_server_packages")) {
			serverScriptService["ServerPackages"] = map[string]any{"$path": "roblox_server_packages"}
		}
	}
	if len(replicatedStorage) > 0 {
		tree["ReplicatedStorage"] = replicatedStorage
	}
	if len(serverScriptService) > 0 {
		tree["ServerScriptService"] = serverScriptService
	}

	return config
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
