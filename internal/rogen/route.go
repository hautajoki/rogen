package rogen

import (
	"path"
	"strings"
)

type routeContext struct {
	// build is the posix-style build directory as written into the project
	// file (already including any multi-source sub-path).
	build             string
	isTsProject       bool
	emitLegacyScripts bool
	keepRouteNames    bool
	maps              *routingMaps
	// directoryMarkers maps source-relative directory paths ("" for the
	// source root) to the routing keyword of a marker file found inside.
	directoryMarkers map[string]string
}

type routeResolution struct {
	targetService string
	wrapperFolder string
	virtualParts  []string
	nodeName      string
	projectPath   string
}

// resolveRoute decides where one source file lands in the Rojo tree.
//
// Priority, lowest to highest: folder keyword, directory marker file, file
// affix (separator suffix, PascalCase suffix, then prefix). The most specific
// instruction wins; among folder rules the deepest wins.
func resolveRoute(relativePath string, isInit bool, ctx *routeContext) routeResolution {
	maps := ctx.maps

	parts := strings.Split(strings.ReplaceAll(relativePath, "\\", "/"), "/")
	filename := parts[len(parts)-1]
	parts = parts[:len(parts)-1]
	basename := strings.TrimSuffix(filename, path.Ext(filename))
	virtualParts := []string{}

	targetService := "ReplicatedStorage"
	lastRouteKeyword := ""
	environmentKeyword := ""

	// Marker routing for the source root.
	if rootMarker, ok := ctx.directoryMarkers[""]; ok {
		targetService = maps.lowerCaseMap[rootMarker]
		lastRouteKeyword = rootMarker
		if serviceAliases[rootMarker] {
			environmentKeyword = rootMarker
		}
	}

	// Folder routing.
	currentPath := ""
	for _, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath += "/" + part
		}

		lowerPart := strings.ToLower(part)
		matchedService := maps.lowerCaseMap[lowerPart]
		marker := ctx.directoryMarkers[currentPath]

		switch {
		case marker != "":
			targetService = maps.lowerCaseMap[marker]
			lastRouteKeyword = marker
			if serviceAliases[marker] {
				environmentKeyword = marker
			}
			// Keep the folder name unless it is itself a routing keyword.
			if matchedService == "" {
				virtualParts = append(virtualParts, part)
			}
		case matchedService != "":
			targetService = matchedService
			lastRouteKeyword = lowerPart
			if serviceAliases[lowerPart] {
				environmentKeyword = lowerPart
			}
		default:
			virtualParts = append(virtualParts, part)
		}
	}

	// Affix routing: an explicit affix on the file always wins.
	matchedLength := 0
	mappedService := ""
	isPrefix := false

	sepSuffixMatch := maps.separatorSuffixRegex.FindStringSubmatch(basename)
	pascalSuffixMatch := maps.pascalCaseSuffixRegex.FindStringSubmatch(basename)
	prefixMatch := maps.prefixRegex.FindStringSubmatch(basename)

	switch {
	case sepSuffixMatch != nil:
		suffix := strings.ToLower(sepSuffixMatch[1])
		mappedService = maps.lowerCaseMap[suffix]
		matchedLength = len(sepSuffixMatch[0])
		if !isInit && serviceAliases[suffix] {
			environmentKeyword = suffix
		}
	case pascalSuffixMatch != nil:
		suffix := strings.ToLower(pascalSuffixMatch[1])
		mappedService = maps.mergedServices[pascalSuffixMatch[1]]
		matchedLength = len(pascalSuffixMatch[0])
		if !isInit && serviceAliases[suffix] {
			environmentKeyword = suffix
		}
	case prefixMatch != nil:
		prefix := strings.ToLower(prefixMatch[1])
		mappedService = maps.lowerCaseMap[prefix]
		matchedLength = len(prefixMatch[0])
		if !isInit && serviceAliases[prefix] {
			environmentKeyword = prefix
		}
		isPrefix = true
	}

	if mappedService != "" {
		targetService = mappedService
	}

	// Resolve the namespace wrapper folder.
	wrapperFolder := "shared"
	switch {
	case serverContainers[targetService]:
		wrapperFolder = "server"
	case clientContainers[targetService]:
		wrapperFolder = "client"
	case environmentKeyword != "":
		wrapperFolder = environmentKeyword
	}

	// Scripts with non-legacy RunContext run incorrectly in StarterPlayer containers.
	isStarterPlayerContainer := targetService == "StarterPlayerScripts" || targetService == "StarterCharacterScripts"
	if !ctx.emitLegacyScripts && isStarterPlayerContainer {
		targetService = "ReplicatedStorage"
	}

	nodeName := basename
	var projectPath string

	if isInit {
		folderRelativePath := path.Dir(strings.ReplaceAll(relativePath, "\\", "/"))
		projectPath = path.Join(ctx.build, folderRelativePath)
		if len(virtualParts) > 0 {
			nodeName = virtualParts[len(virtualParts)-1]
			virtualParts = virtualParts[:len(virtualParts)-1]
		} else if lastRouteKeyword != "" {
			nodeName = lastRouteKeyword
		} else {
			nodeName = "source"
		}
	} else {
		compiledRelativePath := strings.ReplaceAll(relativePath, "\\", "/")
		if ctx.isTsProject {
			compiledFilename := replaceTsExtension(filename)
			compiledRelativePath = path.Join(path.Dir(compiledRelativePath), compiledFilename)
		}
		projectPath = path.Join(ctx.build, compiledRelativePath)

		if mappedService != "" {
			shouldStrip := !ctx.keepRouteNames

			// Rojo relies on '.server' and '.client' explicitly for script
			// types; those exact dot-affixes are stripped regardless.
			if ctx.keepRouteNames && sepSuffixMatch != nil {
				exactMatch := strings.ToLower(sepSuffixMatch[0])
				if exactMatch == ".server" || exactMatch == ".client" {
					shouldStrip = true
				}
			}

			if shouldStrip {
				if isPrefix {
					nodeName = basename[matchedLength:]
				} else {
					nodeName = basename[:len(basename)-matchedLength]
				}
			}
		}
	}

	return routeResolution{
		targetService: targetService,
		wrapperFolder: wrapperFolder,
		virtualParts:  virtualParts,
		nodeName:      nodeName,
		projectPath:   projectPath,
	}
}

func replaceTsExtension(filename string) string {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".tsx") {
		return filename[:len(filename)-4] + ".luau"
	}
	if strings.HasSuffix(lower, ".ts") {
		return filename[:len(filename)-3] + ".luau"
	}
	return filename
}
