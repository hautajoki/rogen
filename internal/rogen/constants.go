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
	"Workspace":               "Workspace",
	"Lighting":                "Lighting",
	"SoundService":            "SoundService",
	"RobloxPluginGuiService":  "RobloxPluginGuiService",
}

// These common words are useful as explicit folder or marker routes, but are
// too collision-prone as implicit filename affixes (for example
// default-lighting.ts). Keeping them folder-only adds the upstream services
// without changing existing module placement.
var folderOnlyServices = map[string]bool{
	"Workspace":              true,
	"Lighting":               true,
	"SoundService":           true,
	"RobloxPluginGuiService": true,
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
	"luau":    {Name: "luau", Output: "default.project.json", Build: "src"},
	"ts":      {Name: "ts", Output: "default.project.json", Build: "out"},
	"darklua": {Name: "darklua", Output: "build.project.json", Build: "dist"},
}

// defaultConfigJSON provides the fallback modes and template when no config
// file exists. `rogen --init` writes a project-aware minimal configuration.
const defaultConfigJSON = `{
	"source": ["src"],
	"verbatim": false,
	"casing": "camelCase",
	"unwrap": false,
	"globIgnorePaths": [],
	"aliases": {},
	"luau": {
		"output": "default.project.json",
		"build": "src",
		"env": [],
		"globIgnorePaths": []
	},
	"ts": {
		"output": "default.project.json",
		"build": "out",
		"env": [],
		"globIgnorePaths": []
	},
	"darklua": {
		"output": "build.project.json",
		"build": "dist",
		"env": [],
		"globIgnorePaths": []
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
	// mergedServices keeps affix-capable original-case keywords.
	mergedServices map[string]string
	// lowerCaseMap includes every folder/marker keyword.
	lowerCaseMap      map[string]string
	affixLowerCaseMap map[string]string

	separatorSuffixRegex  *regexp.Regexp
	pascalCaseSuffixRegex *regexp.Regexp
	separatorPrefixRegex  *regexp.Regexp
	camelCasePrefixRegex  *regexp.Regexp
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

	affixMerged := make(map[string]string, len(merged))
	for k, v := range services {
		if !folderOnlyServices[k] {
			affixMerged[k] = v
		}
	}
	for k, v := range customAliases {
		affixMerged[k] = v
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
	affixLower := make(map[string]string, len(affixMerged))
	for k, v := range services {
		if folderOnlyServices[k] {
			continue
		}
		affixLower[strings.ToLower(k)] = v
	}
	for k, v := range customAliases {
		affixLower[strings.ToLower(k)] = v
	}

	mergedKeys := sortedKeysByLengthDesc(affixMerged)
	lowerKeys := sortedKeysByLengthDesc(affixLower)
	allPrefixKeySet := make(map[string]bool, len(mergedKeys)+len(lowerKeys))
	for _, key := range mergedKeys {
		allPrefixKeySet[key] = true
	}
	for _, key := range lowerKeys {
		allPrefixKeySet[key] = true
	}
	allPrefixKeys := sortedKeysByLengthDesc(allPrefixKeySet)

	quote := func(keys []string) string {
		quoted := make([]string, len(keys))
		for i, k := range keys {
			quoted[i] = regexp.QuoteMeta(k)
		}
		return strings.Join(quoted, "|")
	}

	return &routingMaps{
		mergedServices:        affixMerged,
		lowerCaseMap:          lower,
		affixLowerCaseMap:     affixLower,
		separatorSuffixRegex:  regexp.MustCompile(`(?i)[.\-_+](` + quote(lowerKeys) + `)$`),
		pascalCaseSuffixRegex: regexp.MustCompile(`(` + quote(mergedKeys) + `)$`),
		separatorPrefixRegex:  regexp.MustCompile(`(?i)^(` + quote(lowerKeys) + `)([.\-_+])`),
		camelCasePrefixRegex:  regexp.MustCompile(`^(` + quote(allPrefixKeys) + `)[A-Z]`),
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
