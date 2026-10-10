package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"jobs.willwagner.me/internal/admin"
	"jobs.willwagner.me/internal/db"
)

func main() {
	log.Fatal(run())
}

// run opens the admin database, applies pending schema work, and serves the
// admin routes until the listener stops. It returns the terminal error so
// main can report it; ListenAndServe never returns nil in practice.
func run() error {
	addr := envOrDefault("JOBS_ADMIN_ADDR", ":8081")
	dbPath := envOrDefault("JOBS_DB_PATH", "jobs.sqlite3")
	adminToken := os.Getenv("JOBS_ADMIN_TOKEN")

	conn, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		return err
	}
	if err := admin.EnsureAdminTables(ctx, conn); err != nil {
		return err
	}

	mailer := admin.NewSMTPMailerFromEnv()
	server := admin.NewServer(conn, adminToken, mailer)
	log.Printf("admin service listening on %s", addr)
	return http.ListenAndServe(addr, server.Routes())
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
