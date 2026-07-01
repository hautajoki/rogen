package rogen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// makeSyntheticProject lays out a large feature-based source tree:
// nFeatures features, each with shared schema files, a client and a server
// folder, plus a block of init-file modules.
func makeSyntheticProject(tb testing.TB, nFeatures int) (root string, sources []resolvedSource, cfg *Config) {
	tb.Helper()
	root = tb.TempDir()

	var files []string
	for i := range nFeatures {
		feature := fmt.Sprintf("features/feature-%03d", i)
		files = append(files,
			feature+"/schema.ts",
			feature+"/logic.ts",
			feature+"/helpers.ts",
			feature+"/client/controller.ts",
			feature+"/client/effects.ts",
			feature+"/server/service.ts",
			feature+"/server/data.ts",
		)
		if i%4 == 0 {
			files = append(files, fmt.Sprintf("modules/mod-%03d/index.ts", i))
		}
	}
	for i, file := range files {
		full := filepath.Join(root, "src", filepath.FromSlash(file))
		if i%7 == 0 {
			// A sprinkling of marker files.
			_ = os.MkdirAll(filepath.Dir(full), 0o755)
			_ = os.WriteFile(filepath.Join(filepath.Dir(full), ".shared"), nil, 0o644)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			tb.Fatal(err)
		}
	}

	cfg = &Config{Anchor: root}
	sources = []resolvedSource{{abs: filepath.Join(root, "src"), rel: "src"}}
	return root, sources, cfg
}

func BenchmarkRunBuild(b *testing.B) {
	_, sources, cfg := makeSyntheticProject(b, 500) // ~3600 files
	mode := Mode{Output: "default.project.json", Build: "out"}
	baseTree := map[string]any{"name": "bench", "tree": map[string]any{}}
	env := environment{isTsProject: true}

	b.ResetTimer()
	for b.Loop() {
		if _, err := runBuild(mode, baseTree, cfg, env, sources, &cliArgs{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveRoute(b *testing.B) {
	ctx := &routeContext{
		build:             "out",
		isTsProject:       true,
		emitLegacyScripts: true,
		maps:              generateRoutingMaps(nil),
		directoryMarkers:  map[string]string{},
	}
	paths := []string{
		"features/audio/schema.ts",
		"features/audio/client/local.ts",
		"features/profile/server/driver.ts",
		"react/client/components/layout/layer.tsx",
		"modules/deep/nested/path/util.ts",
	}

	b.ResetTimer()
	for b.Loop() {
		for _, p := range paths {
			resolveRoute(p, false, ctx)
		}
	}
}

func BenchmarkMarshalSortedJSON(b *testing.B) {
	_, sources, cfg := makeSyntheticProject(b, 500)
	mode := Mode{Output: "default.project.json", Build: "out"}
	baseTree := map[string]any{"name": "bench", "tree": map[string]any{}}
	result, err := runBuild(mode, baseTree, cfg, environment{isTsProject: true}, sources, &cliArgs{})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := marshalSortedJSON(result.tree); err != nil {
			b.Fatal(err)
		}
	}
}
