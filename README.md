<div align="center">
	<h1>Rogen</h1>
	<p>A tool for feature-based folder structures with Rojo.</p>
	<img src="example.png" alt="Visual mapping of VS Code to Roblox Explorer" width="100%">
</div>

## About this fork

This is [hautajoki](https://github.com/hautajoki)'s fork of [LDGerrits/rogen](https://github.com/LDGerrits/rogen), rewritten in Go. It tracks the upstream 1.4 routing model while changing how paths are resolved so complex project layouts work reliably:

- **Paths in `.rogen.json` resolve relative to the config file**, not the current working directory. Running rogen from anywhere produces the same result.
- **The `build` directory and template `$path` values resolve relative to the generated output file** — exactly how Rojo itself interprets them. The project file can be generated into a nested directory (e.g. `rojo/generated/default.project.json`) while the config stays at the project root.
- **An explicit `--mode` is authoritative** for language detection instead of relying on a `tsconfig.json` in the cwd.
- **Nothing is pruned silently.** When rogen removes tree entries because their paths do not exist, it says so and names them.
- Single static binary per platform; no Node runtime. Startup is ~10x faster, which mostly matters for watch-mode regeneration latency.

The canonical name-preservation setting is now `verbatim`. Existing
`keepRouteNames` configs and the `--keepRouteNames` CLI spelling remain
supported as compatibility aliases.

## What is Rogen?
Rogen is a command line tool that brings **feature-based architecture** to Roblox development for both luau and roblox-ts.

Instead of separating your codebase in a `client`, `shared` and `server` folder at the root level, Rogen lets you group your code by domain and feature. You can keep your inventory UI, inventory server script, and inventory client script all inside a single `inventory` folder. This eliminates context-switching across different folders, making your codebase significantly easier to navigate, refactor, and scale.

In the background, Rogen watches your file system and dynamically generates your `default.project.json` map for Rojo. You get the freedom to group your code in any way you want, and Rogen takes care of sorting everything into the correct Roblox services like `ReplicatedStorage` and `ServerScriptService`.

Moreover, Rogen allows you to merge multiple directories into a single Rojo project. This is useful for multi-place games where you want to share a core across different places.

**Note:** *If you use luau, it is highly recommended to set up [darklua](https://github.com/seaofvoices/darklua) for improved string requires.*

## Automatic Routing
Rogen determines where a file belongs by looking at your folder structure, marker files, and file names.

When multiple rules apply, the deepest folder instruction wins. A marker
overrides the folder it is inside, and an explicit file affix overrides both.

Here are the routing strategies, listed from lowest to highest priority:

### 1. Folder Name
If a folder is named after a routing keyword (`server`, `client`, `shared`) or a Roblox service (e.g., `ReplicatedFirst`), all files within it inherit that destination.
* **Behavior:** Rogen consumes the routing keyword and strips it from the final generated path.
* **Example:** `src/combat/client/combatController.luau` becomes `StarterPlayerScripts/client/combat/combatController.luau`.

### 2. Marker File
To route a folder, you can also place an empty marker file (e.g., `.server`, `.client`, `.shared`) directly inside the directory.
* **Behavior:** The entire folder is routed to that service, but the folder's name is preserved in the Roblox tree.
* **Example:** A folder named `AntiCheat` containing a `.server` marker file will be routed to `ServerScriptService/server/AntiCheat`.

### 3. File Name
To route a specific file differently than its parent folder, use a routing prefix or suffix. File affixes are absolute and will always override folder names and marker files.
* **Delimited:** Use a separator (dot, hyphen, underscore, or plus) before or after the base name.
	* **Examples:** input-client.ts, server.data.ts
* **CamelCase & PascalCase:** Prepend or append the mapped keyword directly to the filename.
	* **Examples:** inputClient.ts, serverData.ts

Common service names added in 1.4 (`Workspace`, `Lighting`, `SoundService`, and
`RobloxPluginGuiService`) route as explicit folders or markers by default, not
as implicit filename affixes. Add one to `aliases` to opt into affix routing.

**Note:** *By default, Rogen strips the routing keyword from the final module
name. Set `verbatim` or use `--verbatim` to preserve it. Exact `.server` and
`.client` suffixes are still stripped because Rojo uses them to determine
script type.*

### 4. Default Fallback
If no routing rules or keywords are found anywhere in the path, the file defaults to `ReplicatedStorage`.

**Important Note for `init` Files:** *If a folder contains an initialization file (like `init.luau` or `index.ts`), Rogen routes the folder itself but will not apply any further routing to its nested contents. This ensures full compatibility with how Rojo handles folders containing initialization scripts.*

### Route controls

- Put `.raw` in a folder to stop route-folder and filename-affix processing
  for that subtree.
- Put `.verbatim` in a folder to preserve routing affixes in that subtree.
- Put `.unwrap` in a folder, or set `unwrap` globally, to omit the
  `shared`/`server`/`client` wrapper.
- Wrap a folder name in parentheses, such as `(inventory)` or `(server)`, to
  use it as an invisible route group.
- Prefix a filename with `^` to hoist it to the root of its target service
  wrapper.

Rogen also routes JSON, TOML, YAML, MessagePack, Markdown, text, and CSV data
files. `.keep`, `.keepme`, `.gitkeep`, and `.gitkeepme` preserve empty folders
without becoming nodes.

### Environments

Each mode may declare an `env` array. All environment names across modes are
known; names active for the selected mode are stripped from file/folder names,
while inactive names drop that file or subtree. Environment marker files work
alongside routing markers, and repeatable `-e/--env` flags add active
environments for a run.

## Merging of Multiple Sources
Rogen supports passing an array of directories to the source config (or passing the -s CLI flag multiple times).

* **Clean Merging:** If, for example, `src/core` and `src/hub` both contain a shared folder, Rogen will merge the contents of both into a single `ReplicatedStorage/shared` folder. No duplicates are created.

* **Overrides:** The order of your sources matters. If both directories contain a file with the exact same name and routing path, the directory listed last will overwrite the previous one.

## Setup & Integration
Integrate Rogen into your workflow to ensure that your `default.project.json` stays synchronized with your file system.

### 1. Installation
Rogen is distributed as a standalone CLI tool. Install it into your project using your preferred toolchain manager:

**Rokit (`rokit.toml`)**
```toml
[tools]
rogen = "hautajoki/rogen@2.1.0"
```

Or build from source with Go 1.26+:
```bash
go install github.com/hautajoki/rogen/v2@latest
```

### 2. Configuration (.rogen.json)
Create a `.rogen.json` file using `rogen --init`.

Every relative path in the config resolves against the config file's directory. The `build` value and any `$path` inside `template` are written into the generated project file and resolve — for both Rojo and rogen's own existence checks — against the directory of the generated output file.

Here is a default configuration structure that works for both roblox-ts and luau, including darklua support. You may want to define a custom tree in "template" for things like adding pesde packages, mapping node_modules, or customizing specific services. If you want to map specific suffixes or folder to a particular service, use the aliases field.

```json
{
	"source": ["src"],
	"verbatim": false,
	"casing": "camelCase",
	"unwrap": false,
	"globIgnorePaths": ["**/*.spec.ts"],
	"luau": {
		"output": "default.project.json",
		"build": "src",
		"env": []
	},
	"ts": {
		"output": "default.project.json",
		"build": "out",
		"env": ["dev"]
	},
	"darklua": {
		"output": "build.project.json",
		"build": "dist"
	},
	"aliases": {
		"Controller": "StarterPlayerScripts",
		"Service": "ServerScriptService"
	},
	"template": {
		"name": "roblox-project",
		"globIgnorePaths": [
			"**/package.json",
			"**/tsconfig.json"
		],
		"tree": {
			"$className": "DataModel",
			"ServerScriptService": {
				"ServerPackages": {
					"$path": "ServerPackages"
				}
			},
			"ReplicatedStorage": {
				"rbxts_include": {
					"$path": "include",
					"node_modules": {
						"$className": "Folder",
						"@rbxts": {
							"$path": "node_modules/@rbxts"
						}
					}
				},
				"Packages": {
					"$path": "Packages"
				}
			}
		}
	}
}
```

| Property            | Description                                                                                                                                                                                                                                                         |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| source              | The root directory (String) or directories (Array of Strings) where your source code lives (defaults to "src"), relative to the config file. Passing an array allows you to merge multiple source folders into a single tree.                                       |
| luau / ts / darklua | Mode-specific overrides. Rogen uses these to dictate where the compiled code ends up (build, relative to the output file) and the name of the generated Rojo file (output, relative to the config file)                                                             |
| <custom_mode>       | You can define your own custom pipeline modes (e.g., "lute") by adding a new key. Custom modes must include an output and a build value.                                                                                                                            |
| template            | The base Rojo tree template. Any standard Rojo `default.project.json` fields (like `name`, `globIgnorePaths`, or a custom `tree`) placed here will be safely merged with Rogen's auto-generated paths. You can also specify a path to a JSON file with a Rojo tree! |
| aliases             | An object allowing you to define custom suffix or folder routing mappings. You can use this to register new keywords (e.g., "Controller": "StarterPlayerScripts") or overwrite Rogen's default service routing behaviors.                                           |
| verbatim            | Preserve routing affixes globally. `keepRouteNames` is accepted as a compatibility alias. |
| casing              | `camelCase` (default) or `PascalCase` for wrapper folder names only. |
| unwrap              | Omit `shared`, `server`, and `client` wrapper folders globally. |
| globIgnorePaths     | Source-relative doublestar glob patterns (`**`, `*`, `?`, classes, and brace alternatives) to skip. Mode-specific patterns are combined with this list; negation and extglobs are not supported. |
| mode.env            | Environment labels active for that mode. Repeatable `--env` values augment them. |

### 3. CLI Usage
You can run Rogen with optional arguments to cleanly override your configurations on the fly:

- `-h, --help`: Show this help menu containing all available options.

- `-v, --version`: Print the rogen version.

- `-i, --init`: Generate a default .rogen.json config file.

- `-c, --config <path>`: Specify a custom Rogen config file path.

- `-m, --mode <mode>`: Specify a mode to run. Repeat it to run multiple modes. Explicit modes also declare the project language for routing.

- `-e, --env <env>`: Add an active environment. Repeatable.

- `-s, --source <path>`: Override the directory containing your raw, uncompiled code. Can be passed multiple times (e.g., -s src/core -s src/hub) to merge multiple directories. CLI paths resolve against the current working directory.

- `-t, --template <path>`: Specify a path to a JSON file that contains your base Rojo blueprint. If omitted, Rogen defaults to the inline object or file mapped in your .rogen.json.

- `-b, --build <path>`: Override the directory where your compiled/transpiled code lands.

- `-o, --output <path>`: Override the name and destination of the final generated Rojo .project.json file.

- `-k, --verbatim`: Preserve routing affixes. The legacy `--keepRouteNames` spelling remains accepted.

- `-w, --watch`: Watch the source directory for changes, automatically regenerating your project files. `rogen watch` is equivalent.

As an example, it is possible to pass a specific configuration file, run a custom mode, inject a base template, and force a targeted output file all at the same time:
```bash
rogen -c build.rogen.json -m darklua -t base.template.json -o build.project.json
```

### 4. Commands

#### For luau
To make Rogen run and watch your files automatically, use the following command:
```bash
rogen -w
```

#### For roblox-ts
Because there is an extra step in the compilation process, it is recommended to install `concurrently` for concurrent execution. That way, you only need to use a single command to set everything up:
```bash
npm install -D concurrently
```
Then, update your package.json script:
```json
"scripts": {
	"watch": "concurrently \"rogen -w\" \"rbxtsc -w\""
},
```
And simply run the script:
```bash
npm run watch
```

## Development

```bash
go build ./...   # build
go test ./...    # run the test suite
go vet ./...     # static checks
```

Releases are cut by pushing a `v*` tag; CI cross-compiles binaries for Linux, Windows, and macOS (x64 + arm64) and attaches them to a GitHub release in the naming scheme rokit expects.
