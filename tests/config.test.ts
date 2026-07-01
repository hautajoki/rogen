import { resolveActiveModes } from "../src/config.js";
import { defaultConfig } from "../src/constants.js";
import { Environment, RogenConfig } from "../src/types.js";

describe("Configuration Resolution", () => {
	const defaultEnv: Environment = { isTsProject: false, isDarkluaProject: false };

	it("should fallback to luau if no config exists and environment is standard", () => {
		const modes = resolveActiveModes({}, false, undefined, defaultEnv);
		expect(modes).toHaveLength(1);
		expect(modes[0].build).toBe(defaultConfig.luau!.build);
	});

	it("should auto-detect TypeScript and use ts defaults", () => {
		const tsEnv: Environment = { isTsProject: true, isDarkluaProject: false };
		const modes = resolveActiveModes({}, false, undefined, tsEnv);
		
		expect(modes).toHaveLength(1);
		expect(modes[0].build).toBe(defaultConfig.ts!.build);
	});

	it("should throw an error if a requested CLI mode does not exist", () => {
		const customConfig: RogenConfig = { myCustomMode: { build: "dist", output: "custom.json" } };
		
		expect(() => {
			resolveActiveModes(customConfig, true, "nonExistentMode", defaultEnv);
		}).toThrow('Mode "nonExistentMode" is not defined in your config file.');
	});

	it("should successfully load a custom CLI mode", () => {
		const customConfig: RogenConfig = { myCustomMode: { build: "dist", output: "custom.json" } };
		const modes = resolveActiveModes(customConfig, true, "myCustomMode", defaultEnv);
		
		expect(modes).toHaveLength(1);
		expect(modes[0].build).toBe("dist");
	});
});