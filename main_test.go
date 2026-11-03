package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func cleanInputs(t *testing.T) {
	t.Helper()
	for _, name := range []string{"INPUT_SEARCH_INPUT", "INPUT_WHITELIST", "INPUT_BLACKLIST", "INPUT_FAIL_ON_MISSING_VARIABLES", "INPUT_REPLACE_IN_PARALLEL", "INPUT_REPLACE_IN_PARALEL", "INPUT_WORKING-DIRECTORY"} {
		t.Setenv(name, "")
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func assertContent(t *testing.T, name, want string) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s: got %q, want %q", name, got, want)
	}
}

func TestSubstitute(t *testing.T) {
	t.Setenv("PRESENT", "значение${OTHER}")
	t.Setenv("EMPTY", "")
	unsetEnv(t, "ABSENT")
	tests := []struct {
		name, input, want string
		cfg               config
		wantErr           bool
	}{
		{name: "values are not expanded again", input: "${PRESENT}", want: "значение${OTHER}"},
		{name: "line endings and empty lines", input: "${PRESENT}\r\n\r\nend\n", want: "значение${OTHER}\r\n\r\nend\n"},
		{name: "no trailing newline", input: "x${EMPTY}x", want: "xx"},
		{name: "missing remains", input: "${ABSENT}", want: "${ABSENT}"},
		{name: "default", input: "${ABSENT:-a:-b}", want: "a:-b", cfg: config{failOnMissing: true}},
		{name: "empty default", input: "${ABSENT:-}", want: "", cfg: config{failOnMissing: true}},
		{name: "empty env is present", input: "${EMPTY:-fallback}", want: "", cfg: config{failOnMissing: true}},
		{name: "whitelist uses variable name", input: "${ABSENT:-yes} ${PRESENT}", want: "yes ${PRESENT}", cfg: config{whitelist: map[string]bool{"ABSENT": true}, failOnMissing: true}},
		{name: "blacklist uses variable name", input: "${ABSENT:-no}", want: "${ABSENT:-no}", cfg: config{blacklist: map[string]bool{"ABSENT": true}, failOnMissing: true}},
		{name: "strict missing", input: "${ABSENT}", cfg: config{failOnMissing: true}, wantErr: true},
		{name: "unclosed", input: "${PRESENT", wantErr: true},
		{name: "multiline placeholder", input: "${PRESENT\n}", wantErr: true},
		{name: "empty placeholder", input: "${}", want: "${}"},
		{name: "literal dollars", input: "$PRESENT $$ ${EMPTY}}", want: "$PRESENT $$ }"},
		{name: "long line", input: strings.Repeat("a", 100000) + "${EMPTY}\n", want: strings.Repeat("a", 100000) + "\n"},
		{name: "binary bytes preserved", input: "\x00\xff${EMPTY}\r\n", want: "\x00\xff\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := substitute(tt.input, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v", err)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	for _, tt := range []struct{ name, value string }{
		{"INPUT_SEARCH_INPUT", `{"secret":"do-not-log"`},
		{"INPUT_SEARCH_INPUT", `{"patterns":["[do-not-log"]}`},
		{"INPUT_SEARCH_INPUT", `{"depth":"do-not-log"}`},
		{"INPUT_WHITELIST", `{"do-not-log":true}`},
		{"INPUT_BLACKLIST", `["do-not-log"`},
		{"INPUT_FAIL_ON_MISSING_VARIABLES", "do-not-log"},
		{"INPUT_REPLACE_IN_PARALLEL", "do-not-log"},
	} {
		t.Run(tt.name+tt.value, func(t *testing.T) {
			cleanInputs(t)
			t.Setenv(tt.name, tt.value)
			_, err := readConfig()
			if err == nil {
				t.Fatal("invalid input accepted")
			}
			if strings.Contains(err.Error(), "do-not-log") {
				t.Fatalf("input leaked: %v", err)
			}
		})
	}
	t.Run("conflicting filters", func(t *testing.T) {
		cleanInputs(t)
		t.Setenv("INPUT_WHITELIST", `["A"]`)
		t.Setenv("INPUT_BLACKLIST", `["B"]`)
		if _, err := readConfig(); err == nil {
			t.Fatal("conflicting filters accepted")
		}
	})
	t.Run("parallel alias and precedence", func(t *testing.T) {
		cleanInputs(t)
		t.Setenv("INPUT_REPLACE_IN_PARALEL", "true")
		cfg, err := readConfig()
		if err != nil || !cfg.parallel || cfg.search.Depth != defaultDepth {
			t.Fatalf("cfg=%+v err=%v", cfg, err)
		}
		t.Setenv("INPUT_REPLACE_IN_PARALLEL", "false")
		cfg, err = readConfig()
		if err != nil || cfg.parallel {
			t.Fatalf("cfg=%+v err=%v", cfg, err)
		}
	})
}

func TestWorkingDirectory(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	for _, directory := range []string{"", ".", "/", "sub", filepath.Join(workspace, "sub")} {
		t.Run("allowed "+directory, func(t *testing.T) {
			t.Setenv("INPUT_WORKING-DIRECTORY", directory)
			root, err := openWorkingDirectory()
			if err != nil {
				t.Fatal(err)
			}
			root.Close()
		})
	}
	for _, directory := range []string{"..", "sub/../../", outside, "escape", "missing"} {
		t.Run("rejected "+directory, func(t *testing.T) {
			t.Setenv("INPUT_WORKING-DIRECTORY", directory)
			root, err := openWorkingDirectory()
			if err == nil {
				root.Close()
				t.Fatal("unsafe or missing directory accepted")
			}
		})
	}
}

func TestFindFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.yaml", "sub/c.yaml", "sub/deeper/d.yaml", ".git/config", "sub/.git"} {
		writeFile(t, filepath.Join(dir, name), "${VALUE}")
	}
	if err := os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, tt := range []struct {
		name string
		cfg  config
		want []string
	}{
		{"all", config{search: searchInput{Depth: 15}}, []string{"a.txt", "b.yaml", "sub/c.yaml", "sub/deeper/d.yaml"}},
		{"depth one", config{search: searchInput{Depth: 1}}, []string{"a.txt", "b.yaml"}},
		{"patterns and names", config{search: searchInput{Depth: 2, Files: []string{"a.txt", "a.txt"}}, patterns: []*regexp.Regexp{regexp.MustCompile(`\.yaml$`)}}, []string{"a.txt", "b.yaml", "sub/c.yaml"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findFiles(root, tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtomicReplacement(t *testing.T) {
	t.Setenv("VALUE", "replaced")
	unsetEnv(t, "ABSENT")
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	original := "${VALUE}\r\n\n"
	writeFile(t, outside, original)
	if err := os.Link(outside, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "symlink")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "strict"), "${VALUE} ${ABSENT}")
	writeFile(t, filepath.Join(dir, "malformed"), "${VALUE} ${UNCLOSED")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := processFile(root, "hardlink", config{}); err != nil {
		t.Fatal(err)
	}
	assertContent(t, outside, original)
	assertContent(t, filepath.Join(dir, "hardlink"), "replaced\r\n\n")
	info, err := root.Stat("hardlink")
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions: info=%v err=%v", info, err)
	}
	for name, content := range map[string]string{"strict": "${VALUE} ${ABSENT}", "malformed": "${VALUE} ${UNCLOSED"} {
		if err := processFile(root, name, config{failOnMissing: true}); err == nil {
			t.Fatalf("%s: expected failure", name)
		}
		assertContent(t, filepath.Join(dir, name), content)
	}
	for _, name := range []string{"symlink", "../outside", "missing"} {
		if err := processFile(root, name, config{}); err == nil {
			t.Fatalf("%s: expected failure", name)
		}
	}
	assertContent(t, outside, original)
	// Renaming onto a directory fails; the original and temp-file cleanup survive.
	if err := root.Mkdir("directory", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(root, "directory", []byte("new"), 0o640); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".envsubst-") {
			t.Fatal("temporary file left behind")
		}
	}
}

func TestRunFixtures(t *testing.T) {
	for _, tt := range []struct {
		name, search, whitelist, blacklist, directory, parallel string
		want                                                    map[string]string
	}{
		{name: "search", directory: "replaced", search: `{"patterns":[".+.yaml"],"files":["replace.me"],"depth":2}`, want: map[string]string{
			"not-replaced.txt": "test ${COOL}", "replaced/test.txt": "test ${COOL}", "replaced/replaced2/test.yaml": "test: test value", "replaced/replaced2/replace.me": "test test value", "replaced/replaced2/notreplaced/replaced.yaml": "test: ${COOL}",
		}},
		{name: "all", parallel: "true", want: map[string]string{"not-replaced.txt": "test test value", "replaced/replaced2/notreplaced/replaced.yaml": "test: test value", "replaced/defaults.txt": "defaultValue notMissing"}},
		{name: "whitelist", whitelist: `["whitelisted"]`, want: map[string]string{"replaced/whitelisted.txt": "whitelisted ${COOL}"}},
		{name: "blacklist", blacklist: `["blacklisted"]`, want: map[string]string{"replaced/blacklisted.txt": "${blacklisted} test value"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cleanInputs(t)
			workspace := t.TempDir()
			if err := os.CopyFS(workspace, os.DirFS("testdir")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GITHUB_WORKSPACE", workspace)
			t.Setenv("INPUT_WORKING-DIRECTORY", tt.directory)
			t.Setenv("INPUT_SEARCH_INPUT", tt.search)
			t.Setenv("INPUT_WHITELIST", tt.whitelist)
			t.Setenv("INPUT_BLACKLIST", tt.blacklist)
			t.Setenv("INPUT_REPLACE_IN_PARALLEL", tt.parallel)
			t.Setenv("COOL", "test value")
			t.Setenv("whitelisted", "whitelisted")
			t.Setenv("blacklisted", "blacklisted")
			t.Setenv("notMissing", "notMissing")
			unsetEnv(t, "missing")
			if err := run(); err != nil {
				t.Fatal(err)
			}
			for name, want := range tt.want {
				assertContent(t, filepath.Join(workspace, name), want)
			}
		})
	}
}

func TestParallelErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VALUE", "ok")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var files []string
	for i := range 100 {
		name := fmt.Sprintf("file-%d", i)
		files = append(files, name)
		writeFile(t, filepath.Join(dir, name), "${VALUE}\n")
	}
	if err := processFiles(root, files, config{parallel: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		assertContent(t, filepath.Join(dir, name), "ok\n")
	}
	if err := processFiles(root, []string{"missing"}, config{parallel: true}); err == nil {
		t.Fatal("parallel failure ignored")
	}
	if err := processFiles(root, nil, config{parallel: true}); err != nil {
		t.Fatal(err)
	}
}
