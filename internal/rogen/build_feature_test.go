package rogen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRoutesDataAndKeepsEmptyFolders(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/config/settings.json",
		"src/empty/.gitkeep",
		"src/ignored.pdf",
	)

	cfg := &Config{Anchor: root, Casing: CamelCase}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})

	if result.fileCount != 2 {
		t.Fatalf("fileCount = %d, want 2", result.fileCount)
	}
	tree := result.tree["tree"].(map[string]any)
	settings := node(t, tree, "ReplicatedStorage", "shared", "config", "settings")
	if settings["$path"] != "out/config/settings.json" {
		t.Errorf("settings $path = %v", settings["$path"])
	}
	node(t, tree, "ReplicatedStorage", "shared", "empty")
}

func TestBuildCombinesGlobalAndModeGlobIgnores(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/ignored.spec.ts",
		"src/tests/ignored.spec.ts",
		"src/tests/ignored.bench.ts",
		"src/tests/kept.ts",
	)

	cfg := &Config{
		Anchor:          root,
		Casing:          CamelCase,
		GlobIgnorePaths: []string{"**/*.spec.ts"},
	}
	mode := Mode{
		Output:          "test.project.json",
		Build:           "out",
		GlobIgnorePaths: []string{"**/*.bench.ts"},
	}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	result, err := runBuild(mode, map[string]any{"tree": map[string]any{}}, cfg, environment{isTsProject: true}, []resolvedSource{source}, &cliArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if result.fileCount != 1 {
		t.Fatalf("fileCount = %d, want 1", result.fileCount)
	}
	tree := result.tree["tree"].(map[string]any)
	node(t, tree, "ReplicatedStorage", "shared", "tests", "kept")
}

func TestBuildWrapperCasingAndUnwrap(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "src/systems/Combat.server.lua")
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}

	cfg := &Config{Anchor: root, Casing: PascalCase}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{source})
	tree := result.tree["tree"].(map[string]any)
	node(t, tree, "ServerScriptService", "Server", "systems", "Combat")

	cfg.Unwrap = true
	result = buildFixture(t, root, cfg, environment{}, []resolvedSource{source})
	tree = result.tree["tree"].(map[string]any)
	node(t, tree, "ServerScriptService", "systems", "Combat")
	if _, ok := tree["ServerScriptService"].(map[string]any)["Server"]; ok {
		t.Error("wrapper folder survived unwrap")
	}
}

func TestBuildReportsSameSourceCollisionsOnly(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/one/Foo.server.lua",
		"src/one/Foo_server.lua",
		"src/two/Foo.server.lua",
	)

	cfg := &Config{Anchor: root, Casing: CamelCase}
	one := resolvedSource{abs: filepath.Join(root, "src/one"), rel: "src/one"}
	two := resolvedSource{abs: filepath.Join(root, "src/two"), rel: "src/two"}
	result := buildFixture(t, root, cfg, environment{}, []resolvedSource{one, two})
	if len(result.collisions) != 1 {
		t.Fatalf("collisions = %v, want one same-source warning", result.collisions)
	}
}

func TestResolveSourcesNormalizesAbsoluteLayoutPaths(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "src/hub/main.lua")
	cfg := &Config{Anchor: root, Sources: []string{"src"}}

	sources, err := resolveSources(&cliArgs{sources: []string{filepath.Join(root, "src")}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].rel != "src" || buildSubPath(sources[0].rel) != "" {
		t.Errorf("absolute source normalized to rel=%q subpath=%q", sources[0].rel, buildSubPath(sources[0].rel))
	}

	sources, err = resolveSources(&cliArgs{sources: []string{filepath.Join(root, "src", "hub")}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].rel != filepath.Join("src", "hub") || buildSubPath(sources[0].rel) != "hub" {
		t.Errorf("nested absolute source normalized to rel=%q subpath=%q", sources[0].rel, buildSubPath(sources[0].rel))
	}
}

func TestBuildSystemMarkersAreCollectedTogether(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/api/.dev",
		"src/api/.server",
		"src/api/endpoint.lua",
	)

	cfg := &Config{
		Anchor: root,
		Casing: CamelCase,
		Modes: map[string]Mode{
			"dev":  {Environments: []string{"dev"}},
			"prod": {Environments: []string{"prod"}},
		},
	}
	source := resolvedSource{abs: filepath.Join(root, "src"), rel: "src"}
	mode := Mode{Output: "test.project.json", Build: "out", Environments: []string{"dev"}}
	result, err := runBuild(mode, map[string]any{"tree": map[string]any{}}, cfg, environment{}, []resolvedSource{source}, &cliArgs{})
	if err != nil {
		t.Fatal(err)
	}
	tree := result.tree["tree"].(map[string]any)
	node(t, tree, "ServerScriptService", "server", "api", "endpoint")
}

func TestCreateInitConfigDetectsProjectTools(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"tsconfig.json",
		"wally.toml",
		"node_modules/@rbxts/.gitkeep",
		"Packages/.gitkeep",
	)
	config := createInitConfig(root)
	if _, ok := config["ts"]; !ok {
		t.Error("TypeScript mode not detected")
	}
	if _, ok := config["luau"]; ok {
		t.Error("Luau mode included in a TypeScript init config")
	}
	template := config["template"].(map[string]any)
	if template["name"] != filepath.Base(root) {
		t.Errorf("template name = %v", template["name"])
	}
	tree := template["tree"].(map[string]any)
	node(t, tree, "ReplicatedStorage", "rbxts_include", "node_modules", "@rbxts")
	node(t, tree, "ReplicatedStorage", "Packages")

	if err := os.WriteFile(filepath.Join(root, ".darklua.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := createInitConfig(root)["darklua"]; !ok {
		t.Error("Darklua mode not detected")
	}
}
