package rogen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// execute runs every active mode: builds the tree, stubs not-yet-compiled
// script files, reports anything that had to be dropped, and writes the
// project file when its content changed.
func execute(sources []resolvedSource, env environment, activeModes []Mode, baseTree map[string]any, cfg *Config, cli *cliArgs) error {
	for _, mode := range activeModes {
		result, err := runBuild(mode, baseTree, cfg, env, sources, cli)
		if err != nil {
			return err
		}

		// Stub outputs the compiler has not produced yet so Rojo and the
		// compiler don't crash: script files become empty files, init-file
		// directories become empty directories. Anything else (e.g. models
		// that will never be generated) is dropped and reported.
		var dropped []string
		for _, item := range result.missing {
			switch ext := strings.ToLower(filepath.Ext(item.absolutePath)); ext {
			case ".luau", ".lua":
				if err := os.MkdirAll(filepath.Dir(item.absolutePath), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(item.absolutePath, nil, 0o644); err != nil {
					return err
				}
			case "":
				if err := os.MkdirAll(item.absolutePath, 0o755); err != nil {
					return err
				}
			default:
				delete(item.parent, item.key)
				dropped = append(dropped, fmt.Sprintf("%s ($path %q)", item.treePath, item.rojoPath))
			}
		}

		if len(result.removed) > 0 || len(dropped) > 0 {
			fmt.Printf("\nWarning: removed entries whose paths do not exist (checked relative to %s):\n", filepath.Dir(result.outputPath))
			for _, item := range result.removed {
				fmt.Printf("   - %s ($path %q)\n", item.treePath, item.rojoPath)
			}
			for _, item := range dropped {
				fmt.Printf("   - %s\n", item)
			}
		}
		for _, collision := range result.collisions {
			fmt.Printf("\nWarning: %s\n", collision)
		}

		content, err := marshalSortedJSON(result.tree)
		if err != nil {
			return err
		}

		if existing, err := os.ReadFile(result.outputPath); err == nil && bytes.Equal(existing, content) {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(result.outputPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(result.outputPath, content, 0o644); err != nil {
			return err
		}

		fmt.Printf("\nSuccess! Generated Rojo tree for %q\n", result.name)
		fmt.Printf("   Build:     %s\n", result.buildDir)
		fmt.Printf("   Processed: %d source files\n", result.fileCount)
		fmt.Printf("   Output:    %s\n\n", result.outputPath)
	}
	return nil
}

// Run is the CLI entry point.
func Run(args []string) error {
	cli, err := parseCliArgs(args)
	if err != nil {
		return err
	}

	if cli.help {
		printHelp()
		return nil
	}
	if cli.version {
		fmt.Printf("rogen %s\n", Version)
		return nil
	}

	if cli.initConfig {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		targetPath := filepath.Join(cwd, ".rogen.json")
		if fileExists(targetPath) {
			return fmt.Errorf("a .rogen.json file already exists in this directory")
		}
		content, err := marshalSortedJSON(createInitConfig(cwd))
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return err
		}
		fmt.Println("\nSuccess! Created .rogen.json in the current directory.")
		return nil
	}

	configPath, err := resolveConfigPath(cli.config)
	if err != nil {
		return err
	}
	cfg, hasConfig, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	for _, environment := range cli.environments {
		if err := validateEnvironmentName(environment, cfg.Aliases); err != nil {
			return fmt.Errorf("CLI error: %w", err)
		}
	}

	// CLI sources resolve against the cwd; config sources against the
	// config file's directory.
	sources, err := resolveSources(cli, cfg)
	if err != nil {
		return err
	}

	env := getEnvironment(cfg.Anchor, cli.modes)
	activeModes, err := resolveActiveModes(cfg, hasConfig, cli.modes, env)
	if err != nil {
		return err
	}
	baseTree, err := loadProjectTree(cli.template, cfg)
	if err != nil {
		return err
	}

	if err := execute(sources, env, activeModes, baseTree, cfg, cli); err != nil {
		return err
	}

	if cli.watch {
		return watch(sources, func() {
			if err := execute(sources, env, activeModes, baseTree, cfg, cli); err != nil {
				fmt.Fprintf(os.Stderr, "\nBuild Failed: %v\n\n", err)
			}
		})
	}
	return nil
}

func resolveSources(cli *cliArgs, cfg *Config) ([]resolvedSource, error) {
	rawSources := cfg.Sources
	resolveBase := cfg.Anchor
	if len(cli.sources) > 0 {
		rawSources = cli.sources
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		resolveBase = cwd
	}

	sources := make([]resolvedSource, 0, len(rawSources))
	for _, raw := range rawSources {
		abs := absJoin(resolveBase, raw)
		if !fileExists(abs) {
			return nil, fmt.Errorf("source directory not found: %s", abs)
		}
		relative, err := filepath.Rel(cfg.Anchor, abs)
		if err != nil {
			return nil, err
		}
		sources = append(sources, resolvedSource{abs: abs, rel: relative})
	}
	return sources, nil
}
