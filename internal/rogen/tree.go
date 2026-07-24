package rogen

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func applyCasing(value string, casing Casing) string {
	if value == "" {
		return value
	}
	if casing == PascalCase {
		return strings.ToUpper(value[:1]) + value[1:]
	}
	return strings.ToLower(value[:1]) + value[1:]
}

// getOrCreateNode returns parent[key], creating it when absent. A non-empty
// className seeds new nodes with $className.
func getOrCreateNode(parent map[string]any, key, className string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	node := map[string]any{}
	if className != "" {
		node["$className"] = className
	}
	parent[key] = node
	return node
}

type removedPath struct {
	treePath string
	rojoPath string
}

// pruneTree removes template nodes whose $path does not exist on disk.
// Generated paths under buildDir are exempt (the compiler produces them
// later). Rojo-facing paths resolve against outputDir, exactly as Rojo
// interprets them. Removed entries are reported, never silently dropped.
func pruneTree(node map[string]any, buildDir, outputDir string) []removedPath {
	var removed []removedPath
	pruneTreeInto(node, "", buildDir, outputDir, &removed)
	return removed
}

func pruneTreeInto(node map[string]any, treePath, buildDir, outputDir string, removed *[]removedPath) {
	for _, key := range sortedKeys(node) {
		child, ok := node[key].(map[string]any)
		if !ok {
			continue
		}

		childTreePath := key
		if treePath != "" {
			childTreePath = treePath + "." + key
		}

		if rojoPath, ok := child["$path"].(string); ok {
			if pathHasPrefix(rojoPath, buildDir) {
				continue
			}
			if !fileExists(absJoin(outputDir, filepath.FromSlash(rojoPath))) {
				delete(node, key)
				*removed = append(*removed, removedPath{treePath: childTreePath, rojoPath: rojoPath})
				continue
			}
		}
		pruneTreeInto(child, childTreePath, buildDir, outputDir, removed)
	}
}

type missingPath struct {
	parent       map[string]any
	key          string
	treePath     string
	rojoPath     string
	absolutePath string
}

// existsCache answers "does this file exist" with one ReadDir per directory
// instead of a stat syscall per file — the missing-path scan touches every
// generated leaf, and syscall count is what rogen's runtime is made of.
type existsCache struct {
	dirs map[string]map[string]bool
}

func (c *existsCache) exists(p string) bool {
	dir, base := filepath.Dir(p), filepath.Base(p)
	names, ok := c.dirs[dir]
	if !ok {
		names = map[string]bool{}
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				names[entry.Name()] = true
			}
		}
		if c.dirs == nil {
			c.dirs = map[string]map[string]bool{}
		}
		c.dirs[dir] = names
	}
	return names[base]
}

// findMissingPaths collects generated entries under buildDir whose files do
// not exist yet (pre-compile), so the caller can stub or drop them.
func findMissingPaths(node map[string]any, buildDir, outputDir string) []missingPath {
	var missing []missingPath
	cache := &existsCache{}
	findMissingPathsInto(node, "", buildDir, outputDir, cache, &missing)
	return missing
}

func findMissingPathsInto(node map[string]any, treePath, buildDir, outputDir string, cache *existsCache, missing *[]missingPath) {
	for _, key := range sortedKeys(node) {
		child, ok := node[key].(map[string]any)
		if !ok {
			continue
		}

		childTreePath := key
		if treePath != "" {
			childTreePath = treePath + "." + key
		}

		if rojoPath, ok := child["$path"].(string); ok && pathHasPrefix(rojoPath, buildDir) {
			absolute := absJoin(outputDir, filepath.FromSlash(rojoPath))
			if !cache.exists(absolute) {
				*missing = append(*missing, missingPath{
					parent:       node,
					key:          key,
					treePath:     childTreePath,
					rojoPath:     rojoPath,
					absolutePath: absolute,
				})
			}
		}
		findMissingPathsInto(child, childTreePath, buildDir, outputDir, cache, missing)
	}
}

// pathHasPrefix reports whether p is dir itself or nested inside it,
// comparing whole path segments ("build" does not prefix "build2/x").
func pathHasPrefix(p, dir string) bool {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	dir = path.Clean(strings.ReplaceAll(dir, "\\", "/"))
	return p == dir || strings.HasPrefix(p, dir+"/")
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// marshalSortedJSON serializes with alphabetically sorted object keys,
// two-space indentation, and no HTML escaping — a stable, diff-friendly
// project file.
func marshalSortedJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeSortedJSON(&buf, value, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func writeSortedJSON(buf *bytes.Buffer, value any, depth int) error {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		keys := sortedKeys(v)
		for i, key := range keys {
			writeIndent(buf, depth+1)
			if err := writeJSONScalar(buf, key); err != nil {
				return err
			}
			buf.WriteString(": ")
			if err := writeSortedJSON(buf, v[key], depth+1); err != nil {
				return err
			}
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		writeIndent(buf, depth)
		buf.WriteByte('}')
		return nil
	case []any:
		if len(v) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, item := range v {
			writeIndent(buf, depth+1)
			if err := writeSortedJSON(buf, item, depth+1); err != nil {
				return err
			}
			if i < len(v)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		writeIndent(buf, depth)
		buf.WriteByte(']')
		return nil
	case json.Number:
		buf.WriteString(v.String())
		return nil
	default:
		return writeJSONScalar(buf, value)
	}
}

func writeJSONScalar(buf *bytes.Buffer, value any) error {
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	// Encoder.Encode appends a newline; strip it.
	buf.Truncate(buf.Len() - 1)
	return nil
}

func writeIndent(buf *bytes.Buffer, depth int) {
	for range depth {
		buf.WriteString("  ")
	}
}
