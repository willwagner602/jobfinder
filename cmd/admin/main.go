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
	addr := envOrDefault("JOBS_ADMIN_ADDR", ":8081")
	dbPath := envOrDefault("JOBS_DB_PATH", "jobs.sqlite3")
	adminToken := os.Getenv("JOBS_ADMIN_TOKEN")

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		log.Fatal(err)
	}
	if err := admin.EnsureAdminTables(ctx, conn); err != nil {
		log.Fatal(err)
	}

	mailer := admin.NewSMTPMailerFromEnv()
	server := admin.NewServer(conn, adminToken, mailer)
	log.Printf("admin service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, server.Routes()))
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
