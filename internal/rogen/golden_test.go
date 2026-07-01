package rogen

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEndToEndNestedOutputLayout replicates the layout that broke upstream:
// the config lives at the project root while the generated project file lives
// in a nested rojo/generated directory. Template $path values are written the
// way Rojo reads them — relative to the output file.
func TestEndToEndNestedOutputLayout(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/features/audio/schema.ts",
		"src/features/audio/client/local.ts",
		"src/features/profile/server/driver.ts",
		"src/modules/minpq/index.ts",
		"rojo/services/workspace/.gitkeep",
		"rojo/generated/include/.gitkeep",
		"node_modules/@rbxts/.gitkeep",
	)

	configJSON := `{
		"source": "src",
		"keepRouteNames": false,
		"aliases": {},
		"ts": {
			"output": "rojo/generated/default.project.json",
			"build": "build"
		},
		"template": {
			"name": "fixture",
			"globIgnorePaths": ["**/package.json", "**/tsconfig.json"],
			"tree": {
				"$className": "DataModel",
				"ReplicatedStorage": {
					"rbxts_include": {
						"$path": "include",
						"node_modules": {
							"$className": "Folder",
							"@rbxts": { "$path": "../../node_modules/@rbxts" }
						}
					},
					"shared": { "$className": "Folder" }
				},
				"Workspace": { "$path": "../services/workspace" },
				"Lighting": { "$path": "../services/does-not-exist" }
			}
		}
	}`
	configPath := filepath.Join(root, ".rogen.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, hasConfig, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasConfig {
		t.Fatal("config not detected")
	}

	env := getEnvironment(cfg.Anchor, "ts")
	modes, err := resolveActiveModes(cfg, true, "ts", env)
	if err != nil {
		t.Fatal(err)
	}
	baseTree, err := loadProjectTree("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := resolveSources(&cliArgs{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := execute(sources, env, modes, baseTree, cfg, &cliArgs{}); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(root, "rojo/generated/default.project.json")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("project file not generated: %v", err)
	}

	tree, err := decodeJSONObject(data)
	if err != nil {
		t.Fatal(err)
	}
	inner := node(t, tree, "tree")

	// Generated TS leaves point at compiled .luau under build/, relative to
	// the output file.
	schema := node(t, inner, "ReplicatedStorage", "shared", "features", "audio", "schema")
	if schema["$path"] != "build/features/audio/schema.luau" {
		t.Errorf("schema $path = %v", schema["$path"])
	}
	local := node(t, inner, "StarterPlayer", "StarterPlayerScripts", "client", "features", "audio", "local")
	if local["$path"] != "build/features/audio/client/local.luau" {
		t.Errorf("local $path = %v", local["$path"])
	}
	driver := node(t, inner, "ServerScriptService", "server", "features", "profile", "driver")
	if driver["$path"] != "build/features/profile/server/driver.luau" {
		t.Errorf("driver $path = %v", driver["$path"])
	}
	minpq := node(t, inner, "ReplicatedStorage", "shared", "modules", "minpq")
	if minpq["$path"] != "build/modules/minpq" {
		t.Errorf("minpq $path = %v", minpq["$path"])
	}

	// Template paths that exist relative to the output file survive; the
	// missing one is pruned.
	include := node(t, inner, "ReplicatedStorage", "rbxts_include")
	if include["$path"] != "include" {
		t.Errorf("rbxts_include $path = %v", include["$path"])
	}
	if _, ok := node(t, inner, "Workspace")["$path"]; !ok {
		t.Error("Workspace pruned despite existing relative to the output file")
	}
	if _, ok := inner["Lighting"]; ok {
		t.Error("Lighting survived despite a missing path")
	}

	// Not-yet-compiled .luau files were stubbed so the toolchain can run.
	stub := filepath.Join(root, "rojo/generated/build/features/audio/schema.luau")
	if !fileExists(stub) {
		t.Error("missing .luau stub for not-yet-compiled output")
	}

	// Re-running with no changes must leave the file byte-identical.
	before, _ := os.ReadFile(outputPath)
	if err := execute(sources, env, modes, baseTree, cfg, &cliArgs{}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(outputPath)
	if string(before) != string(after) {
		t.Error("regeneration was not idempotent")
	}
}

// TestEndToEndRunsFromAnyDirectory proves the cwd no longer matters: results
// are identical whether rogen runs from the project root or a nested folder.
func TestEndToEndRunsFromAnyDirectory(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"src/Math.lua",
		"nested/dir/.gitkeep",
	)
	configJSON := `{
		"source": "src",
		"luau": { "output": "out.project.json", "build": "src" },
		"template": { "name": "fixture", "tree": { "$className": "DataModel" } }
	}`
	configPath := filepath.Join(root, ".rogen.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	generate := func(fromDir string) string {
		t.Helper()
		original, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(original)
		if err := os.Chdir(fromDir); err != nil {
			t.Fatal(err)
		}

		if err := Run([]string{"-c", configPath, "-m", "luau"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, "out.project.json"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	fromRoot := generate(root)
	os.Remove(filepath.Join(root, "out.project.json"))
	fromNested := generate(filepath.Join(root, "nested/dir"))

	if fromRoot != fromNested {
		t.Errorf("output differs by cwd:\nroot:\n%s\nnested:\n%s", fromRoot, fromNested)
	}
	if fromRoot == "" {
		t.Fatal("no output generated")
	}
}
