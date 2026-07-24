package rogen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveActiveModesFallbackLuau(t *testing.T) {
	cfg, err := parseConfig([]byte(defaultConfigJSON), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, false, nil, environment{})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 1 || modes[0].Build != defaultModes["luau"].Build {
		t.Errorf("modes = %+v", modes)
	}
}

func TestResolveActiveModesAutoDetectTs(t *testing.T) {
	cfg, err := parseConfig([]byte(defaultConfigJSON), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, false, nil, environment{isTsProject: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 1 || modes[0].Build != defaultModes["ts"].Build {
		t.Errorf("modes = %+v", modes)
	}
}

func TestResolveActiveModesUnknownCliMode(t *testing.T) {
	cfg, err := parseConfig([]byte(`{"myCustomMode": {"build": "dist", "output": "custom.json"}}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveActiveModes(cfg, true, []string{"nonExistentMode"}, environment{}); err == nil {
		t.Fatal("expected an error for a mode missing from the config")
	}
}

func TestResolveActiveModesCustomCliMode(t *testing.T) {
	cfg, err := parseConfig([]byte(`{"myCustomMode": {"build": "dist", "output": "custom.json"}}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, true, []string{"myCustomMode"}, environment{})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 1 || modes[0].Build != "dist" {
		t.Errorf("modes = %+v", modes)
	}
}

func TestGetEnvironmentDetectsMarkers(t *testing.T) {
	anchor := t.TempDir()
	if got := getEnvironment(anchor, nil); got.isTsProject {
		t.Error("empty dir detected as TS project")
	}
	if err := os.WriteFile(filepath.Join(anchor, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := getEnvironment(anchor, nil); !got.isTsProject {
		t.Error("tsconfig.json not detected")
	}
}

// Regression: an explicit --mode declares the project language even when the
// anchor directory carries no language marker, and overrides markers.
func TestGetEnvironmentExplicitModeIsAuthoritative(t *testing.T) {
	anchor := t.TempDir()
	if got := getEnvironment(anchor, []string{"ts"}); !got.isTsProject {
		t.Error("-m ts not treated as a TS project")
	}
	if err := os.WriteFile(filepath.Join(anchor, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := getEnvironment(anchor, []string{"luau"}); got.isTsProject {
		t.Error("-m luau did not override the tsconfig.json marker")
	}
}

func TestParseConfigValidation(t *testing.T) {
	anchor := t.TempDir()
	for name, data := range map[string]string{
		"legacy keepSuffixes key": `{"keepSuffixes": false}`,
		"non-bool keepRouteNames": `{"keepRouteNames": "yes"}`,
		"bad source":              `{"source": 5}`,
		"null source":             `{"source": null}`,
		"empty source":            `{"source": ""}`,
		"null aliases":            `{"aliases": null}`,
		"null globIgnorePaths":    `{"globIgnorePaths": null}`,
		"array mode":              `{"custom": ["a"]}`,
		"mode missing output":     `{"custom": {"build": "x"}}`,
		"mode missing build":      `{"custom": {"output": "x"}}`,
		"bad template":            `{"template": 5}`,
	} {
		if _, err := parseConfig([]byte(data), anchor); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseConfigModeOrderIsFileOrder(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
		"zeta": {"output": "z.json", "build": "z"},
		"source": "src",
		"alpha": {"output": "a.json", "build": "a"}
	}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModeOrder) != 2 || cfg.ModeOrder[0] != "zeta" || cfg.ModeOrder[1] != "alpha" {
		t.Errorf("ModeOrder = %v", cfg.ModeOrder)
	}
}

func TestLoadProjectTreeTemplatePathIsAnchorRelative(t *testing.T) {
	anchor := t.TempDir()
	templatePath := filepath.Join(anchor, "base.template.json")
	if err := os.WriteFile(templatePath, []byte(`{"name": "from-file", "tree": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig([]byte(`{"template": "base.template.json"}`), anchor)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := loadProjectTree("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tree["name"] != "from-file" {
		t.Errorf("name = %v", tree["name"])
	}
}

func TestLoadProjectTreeDefaultsWhenUnset(t *testing.T) {
	cfg, err := parseConfig([]byte(`{"source": "src"}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := loadProjectTree("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tree["name"] != "roblox-project" {
		t.Errorf("name = %v", tree["name"])
	}
}

func TestParseCliArgs(t *testing.T) {
	parsed, err := parseCliArgs([]string{"-m", "ts", "--config=../conf.json", "-s", "src/core", "-s", "src/hub", "-k", "-w"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.modes) != 1 || parsed.modes[0] != "ts" || parsed.config != "../conf.json" || !parsed.watch {
		t.Errorf("%+v", parsed)
	}
	if len(parsed.sources) != 2 || parsed.sources[1] != "src/hub" {
		t.Errorf("sources = %v", parsed.sources)
	}
	if parsed.verbatim == nil || !*parsed.verbatim {
		t.Error("verbatim not set")
	}

	if _, err := parseCliArgs([]string{"--nope"}); err == nil {
		t.Error("unknown flag accepted")
	}
	if _, err := parseCliArgs([]string{"-o"}); err == nil {
		t.Error("missing value accepted")
	}
}

func TestParseConfigUpstreamFeaturesAndCompatibility(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
		"source": ["src"],
		"verbatim": true,
		"casing": "PascalCase",
		"unwrap": true,
		"globIgnorePaths": ["**/*.spec.ts"],
		"ts": {
			"output": "project.json",
			"build": "out",
			"env": ["dev"],
			"globIgnorePaths": ["**/*.bench.ts"]
		}
	}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Verbatim || cfg.Casing != PascalCase || !cfg.Unwrap {
		t.Errorf("config flags = %+v", cfg)
	}
	mode := cfg.Modes["ts"]
	if len(mode.Environments) != 1 || mode.Environments[0] != "dev" ||
		len(mode.GlobIgnorePaths) != 1 {
		t.Errorf("mode = %+v", mode)
	}

	legacy, err := parseConfig([]byte(`{"keepRouteNames": true}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.Verbatim {
		t.Error("legacy keepRouteNames was not retained as a compatibility alias")
	}

	same, err := parseConfig([]byte(`{"verbatim": true, "keepRouteNames": true}`), t.TempDir())
	if err != nil || !same.Verbatim {
		t.Fatalf("matching compatibility settings were rejected: cfg=%+v err=%v", same, err)
	}
	if _, err := parseConfig([]byte(`{"verbatim": true, "keepRouteNames": false}`), t.TempDir()); err == nil {
		t.Error("conflicting verbatim and keepRouteNames values were accepted")
	}
}

func TestParseCliMultipleModesEnvironmentsAndCommands(t *testing.T) {
	parsed, err := parseCliArgs([]string{"watch", "-m", "ts", "--mode=darklua", "-e", "dev", "--env=debug", "--verbatim"})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.watch || len(parsed.modes) != 2 || len(parsed.environments) != 2 {
		t.Errorf("parsed = %+v", parsed)
	}
	if parsed.verbatim == nil || !*parsed.verbatim {
		t.Error("--verbatim not parsed")
	}

	for _, args := range [][]string{
		{"unknown-command"},
		{"watch", "help"},
		{"--mode="},
		{"--mode", "--watch"},
	} {
		if _, err := parseCliArgs(args); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
}

func TestResolveMultipleActiveModes(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
		"ts": {"output": "ts.json", "build": "out"},
		"darklua": {"output": "dark.json", "build": "dist"}
	}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, true, []string{"ts", "darklua"}, environment{})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 2 || modes[0].Name != "ts" || modes[1].Name != "darklua" {
		t.Errorf("modes = %+v", modes)
	}
	env := getEnvironment(cfg.Anchor, []string{"luau", "darklua"})
	if env.isTsProject || !env.isDarkluaProject {
		t.Errorf("environment = %+v", env)
	}
}

func TestResolveActiveModesFallsBackWhenConfigOnlyDefinesStructure(t *testing.T) {
	cfg, err := parseConfig([]byte(`{
		"source": "src",
		"template": {"name": "game", "tree": {}}
	}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, true, nil, environment{isTsProject: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 1 || modes[0].Name != "ts" {
		t.Errorf("modes = %+v", modes)
	}
}

func TestEnvironmentNameValidation(t *testing.T) {
	for _, environment := range []string{"", " dev", "dev-prod", "raw", "server"} {
		data := []byte(`{"ts":{"output":"x.json","build":"out","env":["` + environment + `"]}}`)
		if _, err := parseConfig(data, t.TempDir()); err == nil {
			t.Errorf("environment %q was accepted", environment)
		}
	}
	if _, err := parseConfig([]byte(`{
		"aliases": {"Preview": "ReplicatedStorage"},
		"ts": {"output": "x.json", "build": "out", "env": ["preview"]}
	}`), t.TempDir()); err == nil {
		t.Error("environment conflicting with a custom routing alias was accepted")
	}
	if _, err := parseConfig([]byte(`{
		"ts": {"output": "x.json", "build": "out", "env": ["dev", "prod2"]}
	}`), t.TempDir()); err != nil {
		t.Errorf("valid environments were rejected: %v", err)
	}
}
