package rogen

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFiles creates a file tree under root; entries are slash-separated
// relative paths, directories are created as needed.
func writeFiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, file := range files {
		full := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func node(t *testing.T, tree map[string]any, path ...string) map[string]any {
	t.Helper()
	current := tree
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("missing node %v (at %q)", path, key)
		}
		current = next
	}
	return current
}

func buildFixture(t *testing.T, root string, cfg *Config, env environment, sources []resolvedSource) *buildResult {
	t.Helper()
	mode := Mode{Output: "test.project.json", Build: "out"}
	baseTree := map[string]any{"name": "test-game", "tree": map[string]any{}}
	result, err := runBuild(mode, baseTree, cfg, env, sources, &cliArgs{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBuildRoutesAndIgnoresNonRobloxFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/systems/Combat.server.lua",
		"src/ui/init.lua",
		"src/ui/Button.lua",
		"src/ignoreMe.pdf",
		"src/Weapon.rbxm",
	)

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})

	if result.fileCount != 3 {
		t.Errorf("fileCount = %d, want 3", result.fileCount)
	}
	if result.name != "test-game" {
		t.Errorf("name = %q", result.name)
	}

	tree := result.tree["tree"].(map[string]any)
	combat := node(t, tree, "ServerScriptService", "server", "systems", "Combat")
	if combat["$path"] != "out/systems/Combat.server.lua" {
		t.Errorf("Combat $path = %v", combat["$path"])
	}
	weapon := node(t, tree, "ReplicatedStorage", "shared", "Weapon")
	if weapon["$path"] != "out/Weapon.rbxm" {
		t.Errorf("Weapon $path = %v", weapon["$path"])
	}
	ui := node(t, tree, "ReplicatedStorage", "shared", "ui")
	if ui["$path"] != "out/ui" {
		t.Errorf("ui $path = %v", ui["$path"])
	}
}

func TestBuildMergesMultipleSources(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/core/CoreMath.lua",
		"src/chapter1/LevelData.lua",
	)

	cfg := &Config{Anchor: root}
	sources := []resolvedSource{
		{abs: filepath.Join(root, "src/core"), rel: "src/core"},
		{abs: filepath.Join(root, "src/chapter1"), rel: "src/chapter1"},
	}
	result := buildFixture(t, root, cfg, environment{}, sources)

	if result.fileCount != 2 {
		t.Errorf("fileCount = %d, want 2", result.fileCount)
	}

	tree := result.tree["tree"].(map[string]any)
	coreMath := node(t, tree, "ReplicatedStorage", "shared", "CoreMath")
	if coreMath["$path"] != "out/core/CoreMath.lua" {
		t.Errorf("CoreMath $path = %v", coreMath["$path"])
	}
	levelData := node(t, tree, "ReplicatedStorage", "shared", "LevelData")
	if levelData["$path"] != "out/chapter1/LevelData.lua" {
		t.Errorf("LevelData $path = %v", levelData["$path"])
	}
}

func TestBuildMarkerFileRouting(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/Database/.server",
		"src/Database/query.lua",
	)

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})

	if result.fileCount != 1 {
		t.Errorf("fileCount = %d, want 1", result.fileCount)
	}

	tree := result.tree["tree"].(map[string]any)
	query := node(t, tree, "ServerScriptService", "server", "Database", "query")
	if query["$path"] != "out/Database/query.lua" {
		t.Errorf("query $path = %v", query["$path"])
	}
}

// Regression: a source reached via ../ navigation (config anchored in a
// nested output directory) must be treated as a root; the navigation
// segments must never leak into the build path.
func TestBuildSubPathIgnoresParentNavigation(t *testing.T) {
	for input, want := range map[string]string{
		"src":              "",
		"./src":            "",
		"../../src":        "",
		"src/hub":          "hub",
		"src/hub/sub":      "hub/sub",
		"../../src/hub":    "hub",
		"../src/core/deep": "core/deep",
	} {
		if got := buildSubPath(input); got != want {
			t.Errorf("buildSubPath(%q) = %q, want %q", input, got, want)
		}
	}
}

// Regression: with a TS environment, generated paths must point at the
// compiled .luau files regardless of where rogen runs from.
func TestBuildTsProjectEmitsLuauPaths(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "src/Weapon.ts")

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{isTsProject: true}, []resolvedSource{source})

	tree := result.tree["tree"].(map[string]any)
	weapon := node(t, tree, "ReplicatedStorage", "shared", "Weapon")
	if weapon["$path"] != "out/Weapon.luau" {
		t.Errorf("Weapon $path = %v", weapon["$path"])
	}
}

func TestBuildPrunesMissingTemplatePathsAndReports(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "src/Math.lua", "existing/marker.txt")

	baseTree := map[string]any{
		"name": "test-game",
		"tree": map[string]any{
			"ReplicatedStorage": map[string]any{
				"Exists":  map[string]any{"$path": "existing"},
				"Missing": map[string]any{"$path": "not-here"},
			},
		},
	}

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	mode := Mode{Output: "test.project.json", Build: "out"}
	result, err := runBuild(mode, baseTree, cfg, environment{}, []resolvedSource{source}, &cliArgs{})
	if err != nil {
		t.Fatal(err)
	}

	tree := result.tree["tree"].(map[string]any)
	replicated := node(t, tree, "ReplicatedStorage")
	if _, ok := replicated["Exists"]; !ok {
		t.Error("Exists was pruned but its path is present")
	}
	if _, ok := replicated["Missing"]; ok {
		t.Error("Missing survived pruning")
	}
	if len(result.removed) != 1 || result.removed[0].rojoPath != "not-here" {
		t.Errorf("removed = %+v", result.removed)
	}
}

func TestBuildResolvesRojoPathsAgainstOutputDir(t *testing.T) {
	root := t.TempDir()
	// The project file generates into nested/, and the template references
	// ../services which exists relative to nested/, not the cwd.
	writeFiles(t, root, "src/Math.lua", "services/keep.txt")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	baseTree := map[string]any{
		"name": "test-game",
		"tree": map[string]any{
			"Workspace": map[string]any{"$path": "../services"},
		},
	}

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	mode := Mode{Output: "nested/test.project.json", Build: "build"}
	result, err := runBuild(mode, baseTree, cfg, environment{}, []resolvedSource{source}, &cliArgs{})
	if err != nil {
		t.Fatal(err)
	}

	tree := result.tree["tree"].(map[string]any)
	if _, ok := tree["Workspace"]; !ok {
		t.Error("Workspace was pruned even though ../services exists relative to the output file")
	}
	if len(result.removed) != 0 {
		t.Errorf("removed = %+v", result.removed)
	}
}

func TestBuildInitDirectoryConsumesContents(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/ui/init.lua",
		"src/ui/nested/deep.server.lua", // must NOT be routed separately
	)

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})

	if result.fileCount != 1 {
		t.Errorf("fileCount = %d, want 1", result.fileCount)
	}
	tree := result.tree["tree"].(map[string]any)
	if _, ok := tree["ServerScriptService"]; ok {
		t.Error("contents of an init directory leaked into ServerScriptService")
	}
}

func TestBuildStarterPlayerScriptsNestUnderStarterPlayer(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "src/input/handler.client.lua")

	cfg := &Config{Anchor: root}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})

	tree := result.tree["tree"].(map[string]any)
	handler := node(t, tree, "StarterPlayer", "StarterPlayerScripts", "client", "input", "handler")
	if handler["$path"] != "out/input/handler.client.lua" {
		t.Errorf("handler $path = %v", handler["$path"])
	}
}
