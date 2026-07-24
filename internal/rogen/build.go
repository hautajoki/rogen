package rogen

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

var initFileRegex = regexp.MustCompile(`(?i)^(index|init)([.\-][a-z0-9_]+)?\.`)

func isScriptFile(filename string) bool {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".d.ts") {
		return false
	}
	return strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx") ||
		strings.HasSuffix(lower, ".lua") || strings.HasSuffix(lower, ".luau")
}

func isModelFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".rbxm") || strings.HasSuffix(lower, ".rbxmx")
}

func isDataFile(filename string) bool {
	lower := strings.ToLower(filename)
	for _, extension := range []string{".json", ".toml", ".yaml", ".yml", ".msgpack", ".md", ".txt", ".csv"} {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func isKeepFile(filename string) bool {
	switch strings.ToLower(filename) {
	case ".keep", ".keepme", ".gitkeep", ".gitkeepme":
		return true
	default:
		return false
	}
}

func isValidSource(filename string) bool {
	return isScriptFile(filename) || isModelFile(filename) || isDataFile(filename)
}

func isInitFile(filename string) bool {
	return isScriptFile(filename) && initFileRegex.MatchString(filename)
}

// resolvedSource is one source directory: its absolute location plus the
// anchor-relative form used to derive the multi-place build sub-path.
type resolvedSource struct {
	abs string
	rel string
}

// listTree enumerates every directory under root concurrently. Directory
// listing is syscall-bound and dominates rogen's runtime, so the I/O fans out
// while routing stays sequential and deterministic. Directories owned by an
// init file are not descended into — their contents are never routed.
func listTree(root string) (map[string][]os.DirEntry, error) {
	listings := make(map[string][]os.DirEntry)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error
	sem := make(chan struct{}, max(4, runtime.NumCPU()*4))

	var scan func(dir string)
	scan = func(dir string) {
		defer wg.Done()
		sem <- struct{}{}
		entries, err := os.ReadDir(dir)
		<-sem
		if err != nil {
			if !os.IsNotExist(err) {
				errOnce.Do(func() { firstErr = err })
			}
			return
		}

		mu.Lock()
		listings[dir] = entries
		mu.Unlock()

		for _, entry := range entries {
			if entry.Type().IsRegular() && isInitFile(entry.Name()) {
				return
			}
		}
		for _, entry := range entries {
			if entry.IsDir() {
				wg.Add(1)
				go scan(filepath.Join(dir, entry.Name()))
			}
		}
	}

	wg.Add(1)
	go scan(root)
	wg.Wait()
	return listings, firstErr
}

// walkSource traverses one source tree in deterministic (sorted) order and
// invokes the callback for each source or keep file.
// A directory containing an init file is reported as a single unit — Rojo
// mandates that structure — so its other contents are not routed further.
func walkSource(dir string, listings map[string][]os.DirEntry, callback func(filePath string, isInit bool)) error {
	entries, ok := listings[dir]
	if !ok {
		return nil
	}

	for _, entry := range entries {
		if entry.Type().IsRegular() && isInitFile(entry.Name()) {
			callback(filepath.Join(dir, entry.Name()), true)
			return nil
		}
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if err := walkSource(fullPath, listings, callback); err != nil {
				return err
			}
		} else if isValidSource(entry.Name()) || isKeepFile(entry.Name()) {
			callback(fullPath, false)
		}
	}
	return nil
}

var systemMarkers = map[string]bool{
	"raw":      true,
	"verbatim": true,
	"unwrap":   true,
}

func buildDirectoryMarkers(sourceRoot string, listings map[string][]os.DirEntry, maps *routingMaps, knownEnvs map[string]bool) (map[string][]string, error) {
	markers := map[string][]string{}
	for dir, entries := range listings {
		var found []string
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			marker := strings.ToLower(strings.TrimPrefix(entry.Name(), "."))
			if systemMarkers[marker] || maps.lowerCaseMap[marker] != "" || knownEnvs[marker] {
				found = append(found, marker)
			}
		}
		if len(found) == 0 {
			continue
		}
		relative, err := filepath.Rel(sourceRoot, dir)
		if err != nil {
			return nil, err
		}
		if relative == "." {
			relative = ""
		}
		markers[filepath.ToSlash(relative)] = found
	}
	return markers, nil
}

func environmentSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[strings.ToLower(value)] = true
	}
	return set
}

func buildEnvironments(cfg *Config, mode Mode, cli *cliArgs) (map[string]bool, map[string]bool, []environmentRegexes) {
	activeValues := append(append([]string{}, mode.Environments...), cli.environments...)
	active := environmentSet(activeValues)
	known := map[string]bool{}
	for _, configuredMode := range cfg.Modes {
		for env := range environmentSet(configuredMode.Environments) {
			known[env] = true
		}
	}
	for env := range active {
		known[env] = true
	}

	expressions := make([]environmentRegexes, 0, len(active))
	for _, env := range sortedKeysByLengthDesc(active) {
		quoted := regexp.QuoteMeta(env)
		expressions = append(expressions, environmentRegexes{
			suffix: regexp.MustCompile(`(?i)[.\-_+]` + quoted + `$`),
			prefix: regexp.MustCompile(`(?i)^` + quoted + `[.\-_+]`),
			middle: regexp.MustCompile(`(?i)[.\-_+]` + quoted + `(?P<delimiter>[.\-_+])`),
		})
	}
	return active, known, expressions
}

func validateGlobPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if _, err := doublestar.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid globIgnorePaths pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func matchesAnyGlob(patterns []string, filename string) bool {
	for _, pattern := range patterns {
		matched, _ := doublestar.Match(pattern, filename)
		if matched {
			return true
		}
	}
	return false
}

// buildSubPath derives the extra build sub-directory for multi-source setups:
// source "src/hub" builds into "<build>/hub". Leading "../" and "./" segments
// only navigate to the source root and are never part of the compiler's
// output layout, so they are skipped rather than corrupting the build path.
func buildSubPath(sourceRel string) string {
	segments := []string{}
	for _, segment := range strings.Split(filepath.ToSlash(filepath.Clean(sourceRel)), "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	rootIndex := 0
	for rootIndex < len(segments) && (segments[rootIndex] == ".." || segments[rootIndex] == ".") {
		rootIndex++
	}
	if rootIndex+1 >= len(segments) {
		return ""
	}
	return strings.Join(segments[rootIndex+1:], "/")
}

type buildResult struct {
	// outputPath is the absolute path of the project file to write.
	outputPath string
	tree       map[string]any
	missing    []missingPath
	removed    []removedPath
	collisions []string
	name       string
	buildDir   string
	fileCount  int
}

// runBuild produces the Rojo tree for one mode.
func runBuild(mode Mode, baseTree map[string]any, cfg *Config, env environment, sources []resolvedSource, cli *cliArgs) (*buildResult, error) {
	output := mode.Output
	build := mode.Build
	if cli.output != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		output = absJoin(cwd, cli.output)
	} else {
		output = absJoin(cfg.Anchor, output)
	}
	if cli.build != "" {
		build = cli.build
	}
	build = filepath.ToSlash(build)
	outputDir := filepath.Dir(output)

	rojoTree := deepCopyTree(baseTree)
	tree, ok := rojoTree["tree"].(map[string]any)
	if !ok {
		tree = map[string]any{"$className": "DataModel"}
		rojoTree["tree"] = tree
	}

	name := "unknown"
	if n, ok := rojoTree["name"].(string); ok {
		name = n
	}
	emitLegacyScripts := true
	if e, ok := rojoTree["emitLegacyScripts"].(bool); ok {
		emitLegacyScripts = e
	}

	verbatim := cfg.Verbatim
	if cli.verbatim != nil {
		verbatim = *cli.verbatim
	}

	maps := generateRoutingMaps(cfg.Aliases)
	activeEnvs, knownEnvs, envRegexes := buildEnvironments(cfg, mode, cli)

	globPatterns := append(append([]string{}, cfg.GlobIgnorePaths...), mode.GlobIgnorePaths...)
	if err := validateGlobPatterns(globPatterns); err != nil {
		return nil, err
	}

	type nodeOrigin struct {
		source string
		file   string
	}
	nodeOrigins := map[string]nodeOrigin{}
	var collisions []string
	fileCount := 0

	for _, source := range sources {
		listings, err := listTree(source.abs)
		if err != nil {
			return nil, err
		}
		markers, err := buildDirectoryMarkers(source.abs, listings, maps, knownEnvs)
		if err != nil {
			return nil, err
		}
		ctx := &routeContext{
			build:             path.Join(build, buildSubPath(source.rel)),
			isTsProject:       env.isTsProject,
			emitLegacyScripts: emitLegacyScripts,
			verbatim:          verbatim,
			unwrap:            cfg.Unwrap,
			maps:              maps,
			directoryMarkers:  markers,
			knownEnvs:         knownEnvs,
			activeEnvs:        activeEnvs,
			envRegexes:        envRegexes,
		}

		var callbackErr error
		err = walkSource(source.abs, listings, func(filePath string, isInit bool) {
			relativePath, err := filepath.Rel(source.abs, filePath)
			if err != nil {
				callbackErr = err
				return
			}
			relativePath = filepath.ToSlash(relativePath)
			if matchesAnyGlob(globPatterns, relativePath) {
				return
			}

			route := resolveRoute(relativePath, isInit, ctx)
			if route.dropped {
				return
			}
			fileCount++

			current := tree
			if parent, ok := serviceParents[route.targetService]; ok {
				current = getOrCreateNode(current, parent, "")
			}
			current = getOrCreateNode(current, route.targetService, "")
			routeKeyParts := []string{route.targetService}
			if !route.unwrap {
				wrapper := applyCasing(route.wrapperFolder, cfg.Casing)
				current = getOrCreateNode(current, wrapper, "Folder")
				routeKeyParts = append(routeKeyParts, wrapper)
			}

			for _, part := range route.virtualParts {
				current = getOrCreateNode(current, part, "Folder")
				routeKeyParts = append(routeKeyParts, part)
			}

			if isKeepFile(filepath.Base(filePath)) {
				return
			}

			node, ok := current[route.nodeName].(map[string]any)
			if !ok {
				node = map[string]any{}
			}
			routeKey := strings.Join(append(routeKeyParts, route.nodeName), "\x00")
			if _, hasPath := node["$path"]; hasPath {
				if origin, ok := nodeOrigins[routeKey]; ok && origin.source == source.abs {
					collisions = append(collisions,
						fmt.Sprintf("name collision: %q and %q both map to node %q", origin.file, relativePath, route.nodeName))
				}
			}
			node["$path"] = route.projectPath
			if node["$className"] == "Folder" {
				delete(node, "$className")
			}
			current[route.nodeName] = node
			nodeOrigins[routeKey] = nodeOrigin{source: source.abs, file: relativePath}
		})
		if err != nil {
			return nil, err
		}
		if callbackErr != nil {
			return nil, callbackErr
		}
	}

	removed := pruneTree(tree, build, outputDir)
	missing := findMissingPaths(tree, build, outputDir)

	return &buildResult{
		outputPath: output,
		tree:       rojoTree,
		missing:    missing,
		removed:    removed,
		collisions: collisions,
		name:       name,
		buildDir:   build,
		fileCount:  fileCount,
	}, nil
}
