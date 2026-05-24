const fs = require("fs");
const path = require("path");
const chokidar = require("chokidar");
const { printHelp, parseCliArgs } = require("./cli");
const { resolveConfigPath, loadAndValidateConfig, loadProjectTree, getEnvironment, resolveActiveModes } = require("./config");
const { execute } = require("./core/pipeline");
const { defaultConfig } = require("./constants");

async function main() {
	const cliArgs = parseCliArgs();

	if (cliArgs.help) {
		printHelp();
		process.exit(0);
	}

	if (cliArgs.init) {
		const targetPath = path.resolve(process.cwd(), ".rogen.json");
		
		if (fs.existsSync(targetPath)) {
			console.error(`\nA .rogen.json file already exists in this directory.\n`);
			process.exit(1);
		}

		fs.writeFileSync(targetPath, JSON.stringify(defaultConfig, null, '\t'));
		console.log(`\nSuccess! Created .rogen.json in the current directory.\n`);
		process.exit(0);
	}

	const configPath = resolveConfigPath(cliArgs.config);
	const { config, hasConfig } = loadAndValidateConfig(configPath);
	
	const source = cliArgs.source || config.source || "src";
	const sourcePath = path.resolve(process.cwd(), source);
	
	if (!fs.existsSync(sourcePath)) {
		throw new Error(`Source directory not found: ${sourcePath}`);
	}

	const env = getEnvironment();
	const activeModes = resolveActiveModes(config, hasConfig, cliArgs.mode, env);
	const baseProjectTree = loadProjectTree(cliArgs.template, config.template);

	execute(sourcePath, env, activeModes, baseProjectTree, config, cliArgs);

	if (cliArgs.watch) {
		console.log(`Rogen watching for file changes in "${source}"... (Press Ctrl+C to stop)`);
		
		let debounceTimeout;
		const watcher = chokidar.watch(sourcePath, {
			ignoreInitial: true,
			persistent: true,
		});

		watcher.on("all", () => {
			clearTimeout(debounceTimeout);
			debounceTimeout = setTimeout(() => {
				execute(sourcePath, env, activeModes, baseProjectTree, config, cliArgs);
			}, 100);
		});

		watcher.on("error", (error) => {
			console.error(`\nWatch Failed: ${error.message}\n`);
		});
		
		await new Promise(() => {}); // Keep alive
	}
}

module.exports = () => {
	main().catch((error) => {
		console.error(`\nBuild Failed: ${error.message}\n`);
		process.exit(1);
	});
};
