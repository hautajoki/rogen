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
	modes, err := resolveActiveModes(cfg, false, "", environment{})
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
	modes, err := resolveActiveModes(cfg, false, "", environment{isTsProject: true})
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
	if _, err := resolveActiveModes(cfg, true, "nonExistentMode", environment{}); err == nil {
		t.Fatal("expected an error for a mode missing from the config")
	}
}

func TestResolveActiveModesCustomCliMode(t *testing.T) {
	cfg, err := parseConfig([]byte(`{"myCustomMode": {"build": "dist", "output": "custom.json"}}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modes, err := resolveActiveModes(cfg, true, "myCustomMode", environment{})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 1 || modes[0].Build != "dist" {
		t.Errorf("modes = %+v", modes)
	}
}

func TestGetEnvironmentDetectsMarkers(t *testing.T) {
	anchor := t.TempDir()
	if got := getEnvironment(anchor, ""); got.isTsProject {
		t.Error("empty dir detected as TS project")
	}
	if err := os.WriteFile(filepath.Join(anchor, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := getEnvironment(anchor, ""); !got.isTsProject {
		t.Error("tsconfig.json not detected")
	}
}

// Regression: an explicit --mode declares the project language even when the
// anchor directory carries no language marker, and overrides markers.
func TestGetEnvironmentExplicitModeIsAuthoritative(t *testing.T) {
	anchor := t.TempDir()
	if got := getEnvironment(anchor, "ts"); !got.isTsProject {
		t.Error("-m ts not treated as a TS project")
	}
	if err := os.WriteFile(filepath.Join(anchor, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := getEnvironment(anchor, "luau"); got.isTsProject {
		t.Error("-m luau did not override the tsconfig.json marker")
	}
}

func TestParseConfigValidation(t *testing.T) {
	anchor := t.TempDir()
	for name, data := range map[string]string{
		"legacy keepSuffixes key": `{"keepSuffixes": false}`,
		"non-bool keepRouteNames": `{"keepRouteNames": "yes"}`,
		"bad source":              `{"source": 5}`,
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
	if parsed.mode != "ts" || parsed.config != "../conf.json" || !parsed.watch {
		t.Errorf("%+v", parsed)
	}
	if len(parsed.sources) != 2 || parsed.sources[1] != "src/hub" {
		t.Errorf("sources = %v", parsed.sources)
	}
	if parsed.keepRouteNames == nil || !*parsed.keepRouteNames {
		t.Error("keepRouteNames not set")
	}

	if _, err := parseCliArgs([]string{"--nope"}); err == nil {
		t.Error("unknown flag accepted")
	}
	if _, err := parseCliArgs([]string{"-o"}); err == nil {
		t.Error("missing value accepted")
	}
}
