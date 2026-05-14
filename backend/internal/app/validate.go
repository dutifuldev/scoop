package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	payloadschema "horse.fit/scoop/schema"
)

type validateResult struct {
	Scanned int
	Valid   int
	Invalid int
}

func runValidate(args []string) int {
	return runParsedCommand(args, parseValidateCommand, executeValidateCommand)
}

type validateCommandConfig struct {
	dir       string
	recursive bool
}

func parseValidateCommand(args []string) (validateCommandConfig, int, bool) {
	fs := newAppFlagSet("validate")

	dir := fs.String("dir", "testdata/news_items", "Directory containing .json news item files")
	recursive := fs.Bool("recursive", true, "Recursively scan subdirectories")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return validateCommandConfig{}, 0, false
		}
		return validateCommandConfig{}, 2, false
	}
	return validateCommandConfig{dir: strings.TrimSpace(*dir), recursive: *recursive}, 0, true
}

func executeValidateCommand(cfg validateCommandConfig) int {
	files, err := collectJSONFiles(cfg.dir, cfg.recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation setup failed: %v\n", err)
		return 1
	}

	result := validateFiles(files)

	fmt.Printf(
		"validate scanned=%d valid=%d invalid=%d dir=%s recursive=%t\n",
		result.Scanned,
		result.Valid,
		result.Invalid,
		cfg.dir,
		cfg.recursive,
	)

	if result.Scanned == 0 {
		fmt.Fprintf(os.Stderr, "Validation failed: no .json files found under %s\n", cfg.dir)
		return 1
	}
	if result.Invalid > 0 {
		return 1
	}
	return 0
}

func validateFiles(files []string) validateResult {
	result := validateResult{}
	for _, path := range files {
		result.Scanned++
		if validateJSONFile(path) {
			result.Valid++
		} else {
			result.Invalid++
		}
	}
	return result
}

func validateJSONFile(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "INVALID %s: read failed: %v\n", path, err)
		return false
	}
	if !json.Valid(raw) {
		fmt.Fprintf(os.Stderr, "INVALID %s: malformed JSON\n", path)
		return false
	}
	if _, err := payloadschema.ValidateNewsItemPayload(json.RawMessage(raw)); err != nil {
		fmt.Fprintf(os.Stderr, "INVALID %s: %v\n", path, err)
		return false
	}
	return true
}

func collectJSONFiles(root string, recursive bool) ([]string, error) {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		return nil, fmt.Errorf("directory path is empty")
	}

	info, err := os.Stat(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", cleanRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", cleanRoot)
	}

	var files []string
	if !recursive {
		return collectJSONFilesFlat(cleanRoot)
	}

	files, err = collectJSONFilesRecursive(cleanRoot)
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func collectJSONFilesFlat(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", root, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		path, ok := visibleJSONFile(root, entry)
		if ok {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files, nil
}

func collectJSONFilesRecursive(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		include, err := recursiveJSONWalkDecision(root, path, d, walkErr)
		if err != nil {
			return err
		}
		if include {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory %s: %w", root, err)
	}
	return files, nil
}

func recursiveJSONWalkDecision(root string, path string, d fs.DirEntry, walkErr error) (bool, error) {
	if walkErr != nil {
		return false, walkErr
	}
	if d.IsDir() {
		if strings.HasPrefix(d.Name(), ".") && path != root {
			return false, filepath.SkipDir
		}
		return false, nil
	}
	if strings.HasPrefix(d.Name(), ".") {
		return false, nil
	}
	return strings.EqualFold(filepath.Ext(d.Name()), ".json"), nil
}

func visibleJSONFile(root string, entry fs.DirEntry) (string, bool) {
	if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
		return "", false
	}
	if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
		return "", false
	}
	return filepath.Join(root, entry.Name()), true
}
