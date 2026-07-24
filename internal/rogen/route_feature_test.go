package rogen

import (
	"slices"
	"testing"
)

func TestRouteAffixBoundariesAndPlusDelimiter(t *testing.T) {
	ctx := baseRouteContext()
	tests := []struct {
		path    string
		service string
		name    string
	}{
		{"serverside.lua", "ReplicatedStorage", "serverside"},
		{"serverData.lua", "ServerScriptService", "Data"},
		{"StarterGui.lua", "ReplicatedStorage", "StarterGui"},
		{"server+Data.lua", "ServerScriptService", "Data"},
		{"Data+server.lua", "ServerScriptService", "Data"},
	}
	for _, test := range tests {
		result := resolveRoute(test.path, false, ctx)
		if result.targetService != test.service || result.nodeName != test.name {
			t.Errorf("%s: got service=%q name=%q, want service=%q name=%q",
				test.path, result.targetService, result.nodeName, test.service, test.name)
		}
	}
}

func TestRouteAdditionalRobloxServices(t *testing.T) {
	for _, service := range []string{"Workspace", "Lighting", "SoundService", "RobloxPluginGuiService"} {
		result := resolveRoute(service+"/item.lua", false, baseRouteContext())
		if result.targetService != service || len(result.virtualParts) != 0 {
			t.Errorf("%s route = %+v", service, result)
		}
	}
	result := resolveRoute("features/default-lighting.lua", false, baseRouteContext())
	if result.targetService != "ReplicatedStorage" || result.nodeName != "default-lighting" {
		t.Errorf("folder-only service was treated as an affix: %+v", result)
	}
	ctx := baseRouteContext()
	ctx.maps = generateRoutingMaps(map[string]string{"Lighting": "Lighting"})
	result = resolveRoute("features/default-lighting.lua", false, ctx)
	if result.targetService != "Lighting" || result.nodeName != "default" {
		t.Errorf("custom alias did not opt into service affix routing: %+v", result)
	}
}

func TestRouteInvisibleGroupsAndHoisting(t *testing.T) {
	ctx := baseRouteContext()

	result := resolveRoute("features/(inventory)/controller.lua", false, ctx)
	if !slices.Equal(result.virtualParts, []string{"features"}) {
		t.Errorf("invisible group virtualParts = %v", result.virtualParts)
	}

	result = resolveRoute("systems/(server)/combat.lua", false, ctx)
	if result.targetService != "ServerScriptService" || !slices.Equal(result.virtualParts, []string{"systems"}) {
		t.Errorf("invisible route group = %+v", result)
	}

	result = resolveRoute("core/boot/^main.server.luau", false, ctx)
	if result.targetService != "ServerScriptService" || len(result.virtualParts) != 0 || result.nodeName != "main" {
		t.Errorf("hoisted route = %+v", result)
	}
	if result.projectPath != "src/core/boot/^main.server.luau" {
		t.Errorf("hoisted projectPath = %q", result.projectPath)
	}
}

func TestRouteEnvironmentFiltering(t *testing.T) {
	cfg := &Config{Modes: map[string]Mode{
		"dev":  {Environments: []string{"dev", "debug"}},
		"prod": {Environments: []string{"prod"}},
	}}
	active, known, expressions := buildEnvironments(cfg, Mode{Environments: []string{"dev", "debug"}}, &cliArgs{})
	ctx := baseRouteContext()
	ctx.activeEnvs = active
	ctx.knownEnvs = known
	ctx.envRegexes = expressions

	if result := resolveRoute("api.prod.lua", false, ctx); !result.dropped {
		t.Error("inactive file environment was not dropped")
	}
	if result := resolveRoute("prod/api.lua", false, ctx); !result.dropped {
		t.Error("inactive folder environment was not dropped")
	}

	result := resolveRoute("api+dev+server.lua", false, ctx)
	if result.dropped || result.targetService != "ServerScriptService" || result.nodeName != "api" {
		t.Errorf("active environment route = %+v", result)
	}

	result = resolveRoute("dev/systems/core.debug.lua", false, ctx)
	if result.dropped || result.nodeName != "core" || !slices.Equal(result.virtualParts, []string{"systems"}) {
		t.Errorf("active environment stripping = %+v", result)
	}

	ctx.directoryMarkers = map[string][]string{"api": {"prod", "server"}}
	if result := resolveRoute("api/endpoint.lua", false, ctx); !result.dropped {
		t.Error("inactive marker environment was not dropped")
	}

	ctx.directoryMarkers = map[string][]string{"api": {"dev", "server"}}
	result = resolveRoute("api/endpoint.lua", false, ctx)
	if result.dropped || result.targetService != "ServerScriptService" || !slices.Contains(result.virtualParts, "api") {
		t.Errorf("active environment plus routing marker = %+v", result)
	}
}

func TestRouteSystemMarkersCascade(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string][]string{"features": {"raw"}}
	result := resolveRoute("features/server/data.client.lua", false, ctx)
	if result.targetService != "ReplicatedStorage" ||
		!slices.Equal(result.virtualParts, []string{"features", "server"}) ||
		result.nodeName != "data.client" {
		t.Errorf("raw marker route = %+v", result)
	}

	ctx = baseRouteContext()
	ctx.directoryMarkers = map[string][]string{"network": {"verbatim"}}
	result = resolveRoute("network/api_server.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.nodeName != "api_server" {
		t.Errorf("verbatim marker route = %+v", result)
	}

	result = resolveRoute("network/api.server.lua", false, ctx)
	if result.nodeName != "api" {
		t.Errorf("exact .server must still strip under verbatim: %+v", result)
	}

	ctx.directoryMarkers = map[string][]string{"network": {"unwrap"}}
	result = resolveRoute("network/api.server.lua", false, ctx)
	if !result.unwrap {
		t.Errorf("unwrap marker route = %+v", result)
	}
}
