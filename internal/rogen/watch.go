package rogen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 100 * time.Millisecond

// watch monitors every source directory recursively and re-invokes regenerate
// after changes settle. Marker files (.server etc.) are dotfiles and DO
// trigger regeneration; VCS and OS noise does not.
func watch(sources []resolvedSource, regenerate func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	addRecursive := func(root string) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if isIgnoredDir(d.Name()) {
					return filepath.SkipDir
				}
				return watcher.Add(p)
			}
			return nil
		})
	}

	var labels []string
	for _, source := range sources {
		if err := addRecursive(source.abs); err != nil {
			return err
		}
		labels = append(labels, source.rel)
	}

	fmt.Printf("Rogen watching for file changes in: %q... (Press Ctrl+C to stop)\n", strings.Join(labels, ", "))

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if isIgnoredFile(filepath.Base(event.Name)) {
				continue
			}
			// Watch newly created directories.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() && !isIgnoredDir(filepath.Base(event.Name)) {
					_ = addRecursive(event.Name)
				}
			}
			timer.Reset(watchDebounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "Error in watcher: %v\n", err)
		case <-timer.C:
			regenerate()
		}
	}
}

func isIgnoredDir(name string) bool {
	return name == ".git" || name == "node_modules"
}

func isIgnoredFile(name string) bool {
	return name == ".DS_Store" || strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".swp")
}
