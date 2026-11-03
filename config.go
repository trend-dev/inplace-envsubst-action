package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const defaultDepth = 15

type searchInput struct {
	Patterns []string `json:"patterns"`
	Files    []string `json:"files"`
	Depth    int      `json:"depth"`
}

type config struct {
	search        searchInput
	patterns      []*regexp.Regexp
	whitelist     map[string]bool
	blacklist     map[string]bool
	failOnMissing bool
	parallel      bool
}

func readConfig() (config, error) {
	var cfg config
	if err := readJSON("INPUT_SEARCH_INPUT", &cfg.search); err != nil {
		return cfg, err
	}
	if cfg.search.Depth <= 0 {
		cfg.search.Depth = defaultDepth
	}
	for i, pattern := range cfg.search.Patterns {
		r, err := regexp.Compile(pattern)
		if err != nil {
			// Do not include input values: they may contain secrets.
			return cfg, fmt.Errorf("search_input.patterns[%d] is not a valid regular expression", i)
		}
		cfg.patterns = append(cfg.patterns, r)
	}
	var err error
	if cfg.whitelist, err = readSet("INPUT_WHITELIST"); err != nil {
		return cfg, err
	}
	if cfg.blacklist, err = readSet("INPUT_BLACKLIST"); err != nil {
		return cfg, err
	}
	if len(cfg.whitelist) > 0 && len(cfg.blacklist) > 0 {
		return cfg, fmt.Errorf("whitelist and blacklist cannot both be set")
	}
	if cfg.failOnMissing, err = readBool("INPUT_FAIL_ON_MISSING_VARIABLES"); err != nil {
		return cfg, err
	}
	// The misspelled input was exposed by older versions of action.yml.
	if os.Getenv("INPUT_REPLACE_IN_PARALLEL") != "" {
		cfg.parallel, err = readBool("INPUT_REPLACE_IN_PARALLEL")
	} else {
		cfg.parallel, err = readBool("INPUT_REPLACE_IN_PARALEL")
	}
	return cfg, err
}

func readJSON(name string, target any) error {
	if value := os.Getenv(name); value != "" {
		if err := json.Unmarshal([]byte(value), target); err != nil {
			return fmt.Errorf("%s must contain valid JSON of the expected type", name)
		}
	}
	return nil
}

func readSet(name string) (map[string]bool, error) {
	var values []string
	if err := readJSON(name, &values); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set, nil
}

func readBool(name string) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return b, nil
}

func openWorkingDirectory() (*os.Root, error) {
	workspace := os.Getenv("GITHUB_WORKSPACE")
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve workspace: %w", err)
		}
	}
	directory := os.Getenv("INPUT_WORKING-DIRECTORY")
	// Older action metadata used '/' to mean the workspace root.
	if directory == "" || directory == "/" {
		directory = "."
	}
	if filepath.IsAbs(directory) {
		var err error
		directory, err = filepath.Rel(workspace, directory)
		if err != nil {
			return nil, fmt.Errorf("working-directory must be inside GITHUB_WORKSPACE")
		}
	}
	if !filepath.IsLocal(directory) {
		return nil, fmt.Errorf("working-directory must be inside GITHUB_WORKSPACE")
	}
	workspaceRoot, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, fmt.Errorf("open workspace: %w", err)
	}
	defer workspaceRoot.Close()
	root, err := workspaceRoot.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open working-directory: %w", err)
	}
	return root, nil
}
