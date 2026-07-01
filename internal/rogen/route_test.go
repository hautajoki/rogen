package rogen

import (
	"slices"
	"testing"
)

func baseRouteContext() *routeContext {
	return &routeContext{
		build:             "src",
		isTsProject:       false,
		emitLegacyScripts: true,
		keepRouteNames:    false,
		maps:              generateRoutingMaps(nil),
		directoryMarkers:  map[string]string{},
	}
}

func TestRouteSuffixToServerScriptService(t *testing.T) {
	result := resolveRoute("systems/Combat.server.lua", false, baseRouteContext())
	if result.targetService != "ServerScriptService" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if result.nodeName != "Combat" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
	if result.wrapperFolder != "server" {
		t.Errorf("wrapperFolder = %q", result.wrapperFolder)
	}
}

func TestRoutePascalCaseSuffix(t *testing.T) {
	result := resolveRoute("ui/InventoryStarterPlayerScripts.lua", false, baseRouteContext())
	if result.targetService != "StarterPlayerScripts" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if result.nodeName != "Inventory" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
	if result.wrapperFolder != "client" {
		t.Errorf("wrapperFolder = %q", result.wrapperFolder)
	}
}

func TestRouteDefaultsToReplicatedStorage(t *testing.T) {
	result := resolveRoute("utils/Math.lua", false, baseRouteContext())
	if result.targetService != "ReplicatedStorage" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if result.nodeName != "Math" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
	if result.wrapperFolder != "shared" {
		t.Errorf("wrapperFolder = %q", result.wrapperFolder)
	}
}

func TestRouteSwapsExtensionForTsProjects(t *testing.T) {
	ctx := baseRouteContext()
	ctx.isTsProject = true
	ctx.build = "out"
	result := resolveRoute("components/Button.ts", false, ctx)
	if result.projectPath != "out/components/Button.luau" {
		t.Errorf("projectPath = %q", result.projectPath)
	}

	result = resolveRoute("components/App.tsx", false, ctx)
	if result.projectPath != "out/components/App.luau" {
		t.Errorf("tsx projectPath = %q", result.projectPath)
	}
}

func TestRouteInitFiles(t *testing.T) {
	result := resolveRoute("systems/Combat/init.lua", true, baseRouteContext())
	if result.nodeName != "Combat" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
	if result.projectPath != "src/systems/Combat" {
		t.Errorf("projectPath = %q", result.projectPath)
	}
}

func TestRouteInitFileFallbackNames(t *testing.T) {
	// No virtual parts and no route keyword: node is named "source".
	result := resolveRoute("init.lua", true, baseRouteContext())
	if result.nodeName != "source" {
		t.Errorf("nodeName = %q", result.nodeName)
	}

	// No virtual parts but a route keyword: keyword names the node.
	result = resolveRoute("server/init.lua", true, baseRouteContext())
	if result.nodeName != "server" {
		t.Errorf("keyword nodeName = %q", result.nodeName)
	}
}

func TestRouteCustomAliases(t *testing.T) {
	ctx := baseRouteContext()
	ctx.maps = generateRoutingMaps(map[string]string{
		"Controller": "StarterPlayerScripts",
		"server":     "ReplicatedStorage",
	})

	result := resolveRoute("ui/PlayerController.lua", false, ctx)
	if result.targetService != "StarterPlayerScripts" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if result.nodeName != "Player" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
	if result.wrapperFolder != "client" {
		t.Errorf("wrapperFolder = %q", result.wrapperFolder)
	}

	result = resolveRoute("systems/Combat.server.lua", false, ctx)
	if result.targetService != "ReplicatedStorage" {
		t.Errorf("override targetService = %q", result.targetService)
	}
	if result.nodeName != "Combat" {
		t.Errorf("override nodeName = %q", result.nodeName)
	}
}

func TestRouteKeepRouteNames(t *testing.T) {
	ctx := baseRouteContext()
	ctx.keepRouteNames = true

	// .server/.client dot-affixes are stripped regardless: Rojo needs them.
	result := resolveRoute("systems/Combat.server.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.nodeName != "Combat" || result.wrapperFolder != "server" {
		t.Errorf("dot suffix: %+v", result)
	}

	result = resolveRoute("systems/Combat_server.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.nodeName != "Combat_server" {
		t.Errorf("underscore suffix: %+v", result)
	}

	ctx.maps = generateRoutingMaps(map[string]string{"Controller": "StarterPlayerScripts"})
	result = resolveRoute("ui/PlayerController.lua", false, ctx)
	if result.targetService != "StarterPlayerScripts" || result.nodeName != "PlayerController" {
		t.Errorf("pascal suffix: %+v", result)
	}
}

func TestRouteSeparatorPrefix(t *testing.T) {
	result := resolveRoute("systems/server.Combat.lua", false, baseRouteContext())
	if result.targetService != "ServerScriptService" || result.nodeName != "Combat" || result.wrapperFolder != "server" {
		t.Errorf("%+v", result)
	}
}

func TestRoutePascalPrefix(t *testing.T) {
	result := resolveRoute("ui/ClientController.ts", false, baseRouteContext())
	if result.targetService != "StarterPlayerScripts" || result.nodeName != "Controller" || result.wrapperFolder != "client" {
		t.Errorf("%+v", result)
	}
}

func TestRoutePrefixStripIncludesSeparator(t *testing.T) {
	result := resolveRoute("systems/server_Combat.lua", false, baseRouteContext())
	if result.targetService != "ServerScriptService" || result.nodeName != "Combat" {
		t.Errorf("%+v", result)
	}
}

func TestRouteRootMarker(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"": "server"}
	result := resolveRoute("Combat.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.wrapperFolder != "server" {
		t.Errorf("%+v", result)
	}
}

func TestRouteDirectoryMarkerPreservesFolderName(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"AntiCheat": "server"}
	result := resolveRoute("AntiCheat/scanner.lua", false, ctx)
	if result.targetService != "ServerScriptService" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if !slices.Contains(result.virtualParts, "AntiCheat") {
		t.Errorf("virtualParts = %v", result.virtualParts)
	}
}

func TestRouteSuffixBeatsMarker(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"network": "shared"}
	result := resolveRoute("network/api.server.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.wrapperFolder != "server" {
		t.Errorf("%+v", result)
	}
}

func TestRouteMarkerBeatsFolderKeywordAndStrips(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"client": "server"}
	result := resolveRoute("client/main.lua", false, ctx)
	if result.targetService != "ServerScriptService" {
		t.Errorf("targetService = %q", result.targetService)
	}
	if slices.Contains(result.virtualParts, "client") {
		t.Errorf("virtualParts = %v", result.virtualParts)
	}
}

func TestRouteDeepestFolderKeywordWins(t *testing.T) {
	result := resolveRoute("client/systems/server/main.lua", false, baseRouteContext())
	if result.targetService != "ServerScriptService" {
		t.Errorf("targetService = %q", result.targetService)
	}
}

func TestRouteDeepMarkerBeatsRootMarker(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"": "client", "systems": "server"}
	result := resolveRoute("systems/main.lua", false, ctx)
	if result.targetService != "ServerScriptService" {
		t.Errorf("targetService = %q", result.targetService)
	}
}

func TestRouteFileSuffixBeatsFolderKeyword(t *testing.T) {
	result := resolveRoute("client/ui/button.server.lua", false, baseRouteContext())
	if result.targetService != "ServerScriptService" || result.wrapperFolder != "server" {
		t.Errorf("%+v", result)
	}
}

func TestRouteFilePrefixBeatsRootMarker(t *testing.T) {
	ctx := baseRouteContext()
	ctx.directoryMarkers = map[string]string{"": "client"}
	result := resolveRoute("server.combat.lua", false, ctx)
	if result.targetService != "ServerScriptService" || result.wrapperFolder != "server" {
		t.Errorf("%+v", result)
	}
}

func TestRouteAllKeywordsStripped(t *testing.T) {
	result := resolveRoute("server/client/shared/test.lua", false, baseRouteContext())
	if result.targetService != "ReplicatedStorage" || result.wrapperFolder != "shared" {
		t.Errorf("%+v", result)
	}
	if len(result.virtualParts) != 0 {
		t.Errorf("virtualParts = %v", result.virtualParts)
	}
	if result.nodeName != "test" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
}

func TestRouteStandardFoldersPreserved(t *testing.T) {
	result := resolveRoute("server/inventory/shared/test.lua", false, baseRouteContext())
	if result.targetService != "ReplicatedStorage" || result.wrapperFolder != "shared" {
		t.Errorf("%+v", result)
	}
	if !slices.Equal(result.virtualParts, []string{"inventory"}) {
		t.Errorf("virtualParts = %v", result.virtualParts)
	}
	if result.nodeName != "test" {
		t.Errorf("nodeName = %q", result.nodeName)
	}
}

func TestRouteNonLegacyScriptsLeaveStarterPlayer(t *testing.T) {
	ctx := baseRouteContext()
	ctx.emitLegacyScripts = false
	result := resolveRoute("input/handler.client.lua", false, ctx)
	if result.targetService != "ReplicatedStorage" {
		t.Errorf("targetService = %q", result.targetService)
	}
	// The wrapper stays "client": the file is still client-side code.
	if result.wrapperFolder != "client" {
		t.Errorf("wrapperFolder = %q", result.wrapperFolder)
	}
}

func TestIsInitFile(t *testing.T) {
	for _, name := range []string{"init.lua", "init.luau", "index.ts", "init.client.luau", "index.server.ts", "Init.lua"} {
		if !isInitFile(name) {
			t.Errorf("expected %q to be an init file", name)
		}
	}
	for _, name := range []string{"initial.lua", "main.lua", "index.d.ts", "reindex.lua"} {
		if isInitFile(name) {
			t.Errorf("expected %q to not be an init file", name)
		}
	}
}
