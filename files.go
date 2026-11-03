package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
)

func findFiles(root *os.Root, cfg config) ([]string, error) {
	var files []string
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		depth := strings.Count(name, "/") + 1
		if entry.IsDir() {
			if entry.Name() == ".git" || depth >= cfg.search.Depth {
				return fs.SkipDir
			}
			return nil
		}
		// Never follow symlinks or open devices, sockets, or named pipes.
		if !entry.Type().IsRegular() || entry.Name() == ".git" {
			return nil
		}
		matched := len(cfg.patterns) == 0 && len(cfg.search.Files) == 0
		for _, pattern := range cfg.patterns {
			matched = matched || pattern.MatchString(entry.Name())
		}
		for _, file := range cfg.search.Files {
			matched = matched || file == entry.Name()
		}
		if matched {
			files = append(files, name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	return files, nil
}

func processFiles(root *os.Root, files []string, cfg config) error {
	workers := 1
	if cfg.parallel {
		workers = min(runtime.GOMAXPROCS(0), len(files))
	}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var once sync.Once
	var firstError error
	for range workers {
		wg.Go(func() {
			for name := range jobs {
				if err := processFile(root, name, cfg); err != nil {
					once.Do(func() { firstError = fmt.Errorf("process %q: %w", name, err) })
				}
			}
		})
	}
	for _, name := range files {
		jobs <- name
	}
	close(jobs)
	wg.Wait()
	return firstError
}

func processFile(root *os.Root, name string, cfg config) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected a regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		file.Close()
		return fmt.Errorf("file changed during processing")
	}
	content, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return err
	}
	output, err := substitute(string(content), cfg)
	if err != nil {
		return err
	}
	if output == string(content) {
		return nil
	}
	return writeAtomic(root, name, []byte(output), info.Mode().Perm())
}

func writeAtomic(root *os.Root, name string, content []byte, mode fs.FileMode) error {
	// A fresh inode also prevents a hard link from modifying files outside root.
	temporary := path.Join(path.Dir(name), ".envsubst-"+rand.Text())
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(content)
	if err := errors.Join(writeErr, file.Sync(), file.Chmod(mode), file.Close()); err != nil {
		return err
	}
	return root.Rename(temporary, name)
}
