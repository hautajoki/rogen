package rogen

import (
	"path"
	"regexp"
	"strings"
)

var environmentDelimiterRegex = regexp.MustCompile(`[.\-_+]`)

type systemFlags struct {
	raw      bool
	verbatim bool
	unwrap   bool
}

type environmentRegexes struct {
	suffix *regexp.Regexp
	prefix *regexp.Regexp
	middle *regexp.Regexp
}

type routeContext struct {
	// build is the posix-style build directory as written into the project
	// file (already including any multi-source sub-path).
	build             string
	isTsProject       bool
	emitLegacyScripts bool
	verbatim          bool
	unwrap            bool
	maps              *routingMaps
	// directoryMarkers maps source-relative directory paths ("" for the
	// source root) to every recognized marker file found inside.
	directoryMarkers map[string][]string
	knownEnvs        map[string]bool
	activeEnvs       map[string]bool
	envRegexes       []environmentRegexes
}

type routeResolution struct {
	targetService string
	wrapperFolder string
	virtualParts  []string
	nodeName      string
	projectPath   string
	dropped       bool
	unwrap        bool
}

type folderRouting struct {
	targetService      string
	virtualParts       []string
	lastRouteKeyword   string
	environmentKeyword string
	flags              systemFlags
}

type affixResolution struct {
	mappedService      string
	matchedLength      int
	exactMatch         string
	environmentKeyword string
	isPrefix           bool
}

func applyMarkers(flags *systemFlags, markers []string) {
	for _, marker := range markers {
		switch marker {
		case "raw":
			flags.raw = true
		case "verbatim":
			flags.verbatim = true
		case "unwrap":
			flags.unwrap = true
		}
	}
}

func firstRoutingMarker(markers []string, maps *routingMaps) string {
	for _, marker := range markers {
		if maps.lowerCaseMap[marker] != "" {
			return marker
		}
	}
	return ""
}

func resolveFolderRouting(parts []string, ctx *routeContext) folderRouting {
	maps := ctx.maps
	virtualParts := []string{}
	targetService := "ReplicatedStorage"
	lastRouteKeyword := ""
	environmentKeyword := ""
	flags := systemFlags{}

	rootMarkers := ctx.directoryMarkers[""]
	applyMarkers(&flags, rootMarkers)
	if !flags.raw {
		if marker := firstRoutingMarker(rootMarkers, maps); marker != "" {
			targetService = maps.lowerCaseMap[marker]
			lastRouteKeyword = marker
			if serviceAliases[marker] {
				environmentKeyword = marker
			}
		}
	}

	currentPath := ""
	for _, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath += "/" + part
		}

		lowerPart := strings.ToLower(part)
		invisible := strings.HasPrefix(lowerPart, "(") && strings.HasSuffix(lowerPart, ")")
		if invisible {
			lowerPart = strings.TrimSuffix(strings.TrimPrefix(lowerPart, "("), ")")
		}

		markers := ctx.directoryMarkers[currentPath]
		applyMarkers(&flags, markers)

		if flags.raw {
			if !ctx.activeEnvs[lowerPart] {
				virtualParts = append(virtualParts, part)
			}
			continue
		}

		matchedService := maps.lowerCaseMap[lowerPart]
		if marker := firstRoutingMarker(markers, maps); marker != "" {
			targetService = maps.lowerCaseMap[marker]
			lastRouteKeyword = marker
			if serviceAliases[marker] {
				environmentKeyword = marker
			}
			if matchedService == "" && !ctx.activeEnvs[lowerPart] && !invisible {
				virtualParts = append(virtualParts, part)
			}
			continue
		}

		if matchedService != "" {
			targetService = matchedService
			lastRouteKeyword = lowerPart
			if serviceAliases[lowerPart] {
				environmentKeyword = lowerPart
			}
		} else if !ctx.activeEnvs[lowerPart] && !invisible {
			virtualParts = append(virtualParts, part)
		}
	}

	return folderRouting{
		targetService:      targetService,
		virtualParts:       virtualParts,
		lastRouteKeyword:   lastRouteKeyword,
		environmentKeyword: environmentKeyword,
		flags:              flags,
	}
}

func resolveAffixes(basename string, isInit bool, maps *routingMaps) *affixResolution {
	if match := maps.separatorSuffixRegex.FindStringSubmatch(basename); match != nil && len(match[0]) < len(basename) {
		suffix := strings.ToLower(match[1])
		result := &affixResolution{
			mappedService: maps.affixLowerCaseMap[suffix],
			matchedLength: len(match[0]),
			exactMatch:    match[0],
		}
		if !isInit && serviceAliases[suffix] {
			result.environmentKeyword = suffix
		}
		return result
	}

	if match := maps.pascalCaseSuffixRegex.FindStringSubmatch(basename); match != nil && len(match[0]) < len(basename) {
		suffix := strings.ToLower(match[1])
		result := &affixResolution{
			mappedService: maps.mergedServices[match[1]],
			matchedLength: len(match[0]),
			exactMatch:    match[0],
		}
		if !isInit && serviceAliases[suffix] {
			result.environmentKeyword = suffix
		}
		return result
	}

	if match := maps.separatorPrefixRegex.FindStringSubmatch(basename); match != nil && len(match[0]) < len(basename) {
		prefix := strings.ToLower(match[1])
		result := &affixResolution{
			mappedService: maps.affixLowerCaseMap[prefix],
			matchedLength: len(match[0]),
			exactMatch:    match[0],
			isPrefix:      true,
		}
		if !isInit && serviceAliases[prefix] {
			result.environmentKeyword = prefix
		}
		return result
	}

	if match := maps.camelCasePrefixRegex.FindStringSubmatch(basename); match != nil && len(match[1]) < len(basename) {
		prefix := strings.ToLower(match[1])
		result := &affixResolution{
			mappedService: maps.affixLowerCaseMap[prefix],
			matchedLength: len(match[1]),
			exactMatch:    match[1],
			isPrefix:      true,
		}
		if !isInit && serviceAliases[prefix] {
			result.environmentKeyword = prefix
		}
		return result
	}

	return nil
}

func wrapperFolder(targetService, environmentKeyword string) string {
	switch {
	case serverContainers[targetService]:
		return "server"
	case clientContainers[targetService]:
		return "client"
	case environmentKeyword != "":
		return environmentKeyword
	default:
		return "shared"
	}
}

// resolveRoute decides where one source file lands in the Rojo tree.
//
// The deepest folder instruction wins, a marker overrides the folder it is
// inside, and a file affix overrides both. Environment labels filter and
// disappear before routing is resolved.
func resolveRoute(relativePath string, isInit bool, ctx *routeContext) routeResolution {
	relativePath = strings.ReplaceAll(relativePath, "\\", "/")
	parts := strings.Split(relativePath, "/")
	filename := parts[len(parts)-1]
	parts = parts[:len(parts)-1]

	rawBasename := strings.TrimSuffix(filename, path.Ext(filename))
	basename := rawBasename
	hoisted := strings.HasPrefix(basename, "^")
	if hoisted {
		basename = strings.TrimPrefix(basename, "^")
	}

	currentPath := ""
	for _, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath += "/" + part
		}
		lowerPart := strings.ToLower(part)
		if ctx.knownEnvs[lowerPart] && !ctx.activeEnvs[lowerPart] {
			return routeResolution{dropped: true}
		}
		for _, marker := range ctx.directoryMarkers[currentPath] {
			if ctx.knownEnvs[marker] && !ctx.activeEnvs[marker] {
				return routeResolution{dropped: true}
			}
		}
	}
	for _, marker := range ctx.directoryMarkers[""] {
		if ctx.knownEnvs[marker] && !ctx.activeEnvs[marker] {
			return routeResolution{dropped: true}
		}
	}
	for _, component := range environmentDelimiterRegex.Split(basename, -1) {
		lowerComponent := strings.ToLower(component)
		if ctx.knownEnvs[lowerComponent] && !ctx.activeEnvs[lowerComponent] {
			return routeResolution{dropped: true}
		}
	}

	for _, expressions := range ctx.envRegexes {
		for expressions.suffix.MatchString(basename) {
			basename = expressions.suffix.ReplaceAllString(basename, "")
		}
		for expressions.prefix.MatchString(basename) {
			basename = expressions.prefix.ReplaceAllString(basename, "")
		}
		for expressions.middle.MatchString(basename) {
			basename = expressions.middle.ReplaceAllString(basename, "${delimiter}")
		}
	}

	folder := resolveFolderRouting(parts, ctx)
	if hoisted {
		folder.virtualParts = nil
	}

	var affix *affixResolution
	if !folder.flags.raw {
		affix = resolveAffixes(basename, isInit, ctx.maps)
	}

	targetService := folder.targetService
	environmentKeyword := folder.environmentKeyword
	if affix != nil {
		targetService = affix.mappedService
		if affix.environmentKeyword != "" {
			environmentKeyword = affix.environmentKeyword
		}
	}
	wrapper := wrapperFolder(targetService, environmentKeyword)

	isStarterPlayerContainer := targetService == "StarterPlayerScripts" || targetService == "StarterCharacterScripts"
	if !ctx.emitLegacyScripts && isStarterPlayerContainer {
		targetService = "ReplicatedStorage"
	}

	nodeName := basename
	projectPath := ""
	if isInit {
		projectPath = path.Join(ctx.build, path.Dir(relativePath))
		if len(folder.virtualParts) > 0 {
			nodeName = folder.virtualParts[len(folder.virtualParts)-1]
			folder.virtualParts = folder.virtualParts[:len(folder.virtualParts)-1]
		} else if folder.lastRouteKeyword != "" {
			nodeName = folder.lastRouteKeyword
		} else {
			nodeName = "source"
		}
	} else {
		compiledRelativePath := relativePath
		if ctx.isTsProject {
			compiledRelativePath = path.Join(path.Dir(relativePath), replaceTsExtension(filename))
		}
		projectPath = path.Join(ctx.build, compiledRelativePath)

		if affix != nil {
			keepFullNames := ctx.verbatim || folder.flags.verbatim
			shouldStrip := !keepFullNames
			if keepFullNames {
				exactMatch := strings.ToLower(affix.exactMatch)
				if exactMatch == ".server" || exactMatch == ".client" {
					shouldStrip = true
				}
			}
			if shouldStrip {
				if affix.isPrefix {
					nodeName = basename[affix.matchedLength:]
				} else {
					nodeName = basename[:len(basename)-affix.matchedLength]
				}
			}
		}
	}

	return routeResolution{
		targetService: targetService,
		wrapperFolder: wrapper,
		virtualParts:  folder.virtualParts,
		nodeName:      nodeName,
		projectPath:   projectPath,
		unwrap:        ctx.unwrap || folder.flags.unwrap,
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
