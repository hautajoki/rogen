package rogen

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
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

func isValidSource(filename string) bool {
	return isScriptFile(filename) || isModelFile(filename)
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

// walkSource traverses one source tree in deterministic (sorted) order,
// recording directory marker files and invoking the callback per source file.
// A directory containing an init file is reported as a single unit — Rojo
// mandates that structure — so its other contents are not routed further.
func walkSource(dir, sourceRoot string, listings map[string][]os.DirEntry, markers map[string]string, maps *routingMaps, callback func(filePath string, isInit bool)) error {
	entries, ok := listings[dir]
	if !ok {
		return nil
	}

	// Scan for a marker file (.server / .client / .shared / custom alias).
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, ".") {
			marker := strings.ToLower(name[1:])
			if maps.lowerCaseMap[marker] != "" {
				relDir, err := filepath.Rel(sourceRoot, dir)
				if err != nil {
					return err
				}
				if relDir == "." {
					relDir = ""
				}
				markers[filepath.ToSlash(relDir)] = marker
				break
			}
		}
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
			if err := walkSource(fullPath, sourceRoot, listings, markers, maps, callback); err != nil {
				return err
			}
		} else if isValidSource(entry.Name()) {
			callback(fullPath, false)
		}
	}
	return nil
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

	keepRouteNames := false
	if cli.keepRouteNames != nil {
		keepRouteNames = *cli.keepRouteNames
	} else if cfg.KeepRouteNames != nil {
		keepRouteNames = *cfg.KeepRouteNames
	}

	maps := generateRoutingMaps(cfg.Aliases)
	fileCount := 0

	for _, source := range sources {
		markers := map[string]string{}
		ctx := &routeContext{
			build:             path.Join(build, buildSubPath(source.rel)),
			isTsProject:       env.isTsProject,
			emitLegacyScripts: emitLegacyScripts,
			keepRouteNames:    keepRouteNames,
			maps:              maps,
			directoryMarkers:  markers,
		}

		listings, err := listTree(source.abs)
		if err != nil {
			return nil, err
		}

		err = walkSource(source.abs, source.abs, listings, markers, maps, func(filePath string, isInit bool) {
			fileCount++
			relativePath, err := filepath.Rel(source.abs, filePath)
			if err != nil {
				return
			}
			route := resolveRoute(filepath.ToSlash(relativePath), isInit, ctx)

			current := tree
			if parent, ok := serviceParents[route.targetService]; ok {
				current = getOrCreateNode(current, parent, "")
			}
			current = getOrCreateNode(current, route.targetService, "")
			current = getOrCreateNode(current, route.wrapperFolder, "Folder")

			for _, part := range route.virtualParts {
				current = getOrCreateNode(current, part, "Folder")
			}

			node, ok := current[route.nodeName].(map[string]any)
			if !ok {
				node = map[string]any{}
			}
			node["$path"] = route.projectPath
			if node["$className"] == "Folder" {
				delete(node, "$className")
			}
			current[route.nodeName] = node
		})
		if err != nil {
			return nil, err
		}
	}

	removed := pruneTree(tree, build, outputDir)
	missing := findMissingPaths(tree, build, outputDir)

	return &buildResult{
		outputPath: output,
		tree:       rojoTree,
		missing:    missing,
		removed:    removed,
		name:       name,
		buildDir:   build,
		fileCount:  fileCount,
	}, nil
}
