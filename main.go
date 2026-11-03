package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "envsubst: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}
	root, err := openWorkingDirectory()
	if err != nil {
		return err
	}
	defer root.Close()
	files, err := findFiles(root, cfg)
	if err != nil {
		return err
	}
	return processFiles(root, files, cfg)
}
