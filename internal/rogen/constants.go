package rogen

import (
	"regexp"
	"sort"
	"strings"
)

// services maps routing keywords (and Roblox service names) to their target service.
var services = map[string]string{
	"Server":                  "ServerScriptService",
	"Client":                  "StarterPlayerScripts",
	"Shared":                  "ReplicatedStorage",
	"ServerScriptService":     "ServerScriptService",
	"ReplicatedStorage":       "ReplicatedStorage",
	"ReplicatedFirst":         "ReplicatedFirst",
	"ServerStorage":           "ServerStorage",
	"StarterGui":              "StarterGui",
	"StarterPack":             "StarterPack",
	"StarterPlayerScripts":    "StarterPlayerScripts",
	"StarterCharacterScripts": "StarterCharacterScripts",
}

// serviceParents nests services that live under another instance in the DataModel.
var serviceParents = map[string]string{
	"StarterPlayerScripts":    "StarterPlayer",
	"StarterCharacterScripts": "StarterPlayer",
}

var serverContainers = map[string]bool{
	"ServerScriptService": true,
	"ServerStorage":       true,
}

var clientContainers = map[string]bool{
	"StarterPlayer":           true,
	"StarterPlayerScripts":    true,
	"StarterCharacterScripts": true,
	"StarterGui":              true,
	"StarterPack":             true,
	"ReplicatedFirst":         true,
}

// serviceAliases are the environment keywords that pick the namespace wrapper folder.
var serviceAliases = map[string]bool{
	"server": true,
	"client": true,
	"shared": true,
}

var defaultModes = map[string]Mode{
	"luau":    {Output: "default.project.json", Build: "src"},
	"ts":      {Output: "default.project.json", Build: "out"},
	"darklua": {Output: "build.project.json", Build: "dist"},
}

// defaultConfigJSON is written by `rogen --init` and provides the fallback
// template when no config file exists.
const defaultConfigJSON = `{
	"source": ["src"],
	"keepRouteNames": false,
	"aliases": {},
	"luau": {
		"output": "default.project.json",
		"build": "src"
	},
	"ts": {
		"output": "default.project.json",
		"build": "out"
	},
	"darklua": {
		"output": "build.project.json",
		"build": "dist"
	},
	"template": {
		"name": "roblox-project",
		"globIgnorePaths": [
			"**/package.json",
			"**/tsconfig.json"
		],
		"tree": {
			"$className": "DataModel",
			"ServerScriptService": {
				"ServerPackages": {
					"$path": "ServerPackages"
				}
			},
			"ReplicatedStorage": {
				"rbxts_include": {
					"$path": "include",
					"node_modules": {
						"$className": "Folder",
						"@rbxts": {
							"$path": "node_modules/@rbxts"
						}
					}
				},
				"Packages": {
					"$path": "Packages"
				}
			}
		}
	}
}`

type routingMaps struct {
	// mergedServices keeps the original-case keyword -> service mapping.
	mergedServices map[string]string
	// lowerCaseMap is the lowercase keyword -> service mapping.
	lowerCaseMap map[string]string

	separatorSuffixRegex  *regexp.Regexp
	pascalCaseSuffixRegex *regexp.Regexp
	prefixRegex           *regexp.Regexp
}

// generateRoutingMaps merges the default keyword table with custom aliases and
// compiles the affix-matching regexes. Longer keywords are tried first so that
// e.g. "StarterPlayerScripts" wins over "Server".
func generateRoutingMaps(customAliases map[string]string) *routingMaps {
	merged := make(map[string]string, len(services)+len(customAliases))
	for k, v := range services {
		merged[k] = v
	}
	for k, v := range customAliases {
		merged[k] = v
	}

	// Custom aliases override defaults, including case-insensitive
	// collisions like a custom "server" replacing the built-in "Server".
	lower := make(map[string]string, len(merged))
	for k, v := range services {
		lower[strings.ToLower(k)] = v
	}
	for k, v := range customAliases {
		lower[strings.ToLower(k)] = v
	}

	mergedKeys := sortedKeysByLengthDesc(merged)
	lowerKeys := sortedKeysByLengthDesc(lower)

	quote := func(keys []string) string {
		quoted := make([]string, len(keys))
		for i, k := range keys {
			quoted[i] = regexp.QuoteMeta(k)
		}
		return strings.Join(quoted, "|")
	}

	return &routingMaps{
		mergedServices:        merged,
		lowerCaseMap:          lower,
		separatorSuffixRegex:  regexp.MustCompile(`(?i)[.\-_](` + quote(lowerKeys) + `)$`),
		pascalCaseSuffixRegex: regexp.MustCompile(`(` + quote(mergedKeys) + `)$`),
		prefixRegex:           regexp.MustCompile(`(?i)^(` + quote(lowerKeys) + `)([.\-_]?)`),
	}
}

func sortedKeysByLengthDesc[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}
