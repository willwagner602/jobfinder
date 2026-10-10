package orchestrator

import (
	"context"
	"database/sql"

	"jobs.willwagner.me/internal/db"
)

// Startup prepares the shared database before independently running services
// are allowed to use it.
func Startup(ctx context.Context, conn *sql.DB) error {
	return db.EnsureCurrent(ctx, conn)
}
