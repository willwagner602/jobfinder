package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunValidationBranches(t *testing.T) {
	for _, args := range [][]string{{"-unknown"}, {"-db", filepath.Join(t.TempDir(), "no-parent", "x.sqlite"), "-providers", "none"}, {"-db", filepath.Join(t.TempDir(), "x.sqlite"), "-providers", "unknown"}, {"-db", filepath.Join(t.TempDir(), "x.sqlite"), "-providers", "fixture"}} {
		if err := run(context.Background(), args); err == nil {
			t.Errorf("run(%v) succeeded", args)
		}
	}
	dbPath := filepath.Join(t.TempDir(), "x.sqlite")
	if err := run(context.Background(), []string{"-db", dbPath, "-json", filepath.Join(t.TempDir(), "boards.json"), "-providers", "none"}); err == nil {
		t.Fatal("expected no-provider validation")
	}
}
