package orchestrator

import (
	"context"
	"testing"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

func TestStartupUpgradesDatabase(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := Startup(context.Background(), conn); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	got, err := db.Version(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if got != schema.CurrentVersion {
		t.Fatalf("version = %d, want %d", got, schema.CurrentVersion)
	}
}
