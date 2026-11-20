package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIsConfigFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want bool
	}{
		{name: "config.json", want: true},
		{name: "config.jsonc", want: true},
		{name: "config.JSON", want: false},
		{name: "config.yaml", want: false},
		{name: "jsonc", want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isConfigFile(testCase.name); got != testCase.want {
				t.Fatalf("isConfigFile(%q) = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestReadConfigDirectoryJSONC(t *testing.T) {
	directory := t.TempDir()
	files := map[string]string{
		"01.json":  "{\"log\":{\"level\":\"info\"}}",
		"02.jsonc": "{\n// JSONC comments must reach the extended decoder\n\"log\":{\"level\":\"debug\"}\n}",
		"03.yaml":  "not JSON",
		"04.JSON":  "not JSON",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A directory with an accepted suffix must still be ignored.
	if err := os.Mkdir(filepath.Join(directory, "05.jsonc"), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPaths, oldDirectories, oldContext := configPaths, configDirectories, globalCtx
	t.Cleanup(func() {
		configPaths, configDirectories, globalCtx = oldPaths, oldDirectories, oldContext
	})
	configPaths = nil
	configDirectories = []string{directory}
	globalCtx = context.Background()

	entries, err := readConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("loaded %d entries, want JSON and JSONC only", len(entries))
	}
	for index, want := range []struct{ name, level string }{{"01.json", "info"}, {"02.jsonc", "debug"}} {
		entry := entries[index]
		if entry.path != filepath.Join(directory, want.name) {
			t.Errorf("entry %d path = %q, want %q", index, entry.path, want.name)
		}
		if entry.options.Log == nil || entry.options.Log.Level != want.level {
			t.Errorf("entry %d log options = %#v, want level %q", index, entry.options.Log, want.level)
		}
	}
}
