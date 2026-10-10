package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/jobs"
	"jobs.willwagner.me/internal/schema"
)

func main() {
	dbPath := flag.String("db", "jobs.sqlite", "SQLite database path")
	sinceFlag := flag.String("since", time.Now().UTC().AddDate(0, 0, -8).Format("2006-01-02"), "oldest posting date to request when providers support server-side filtering")
	healthAddr := flag.String("health-addr", "", "optional address for liveness endpoint /healthz while the run is active")
	flag.Parse()

	ctx := context.Background()
	since, err := time.Parse("2006-01-02", *sinceFlag)
	if err != nil {
		log.Fatalf("parse -since: %v", err)
	}
	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		log.Fatalf("ensure database schema: %v", err)
	}
	var server *http.Server
	if *healthAddr != "" {
		server = startHealth(*healthAddr)
		defer server.Shutdown(context.Background())
	}
	started := time.Now().UTC()
	result, runErr := jobs.Run(ctx, conn, &http.Client{Timeout: 30 * time.Second}, since)
	if err := recordHistory(ctx, conn, started, result, runErr); err != nil {
		log.Printf("record job_history: %v", err)
	}
	if runErr != nil {
		log.Printf("job-description-search: %v", runErr)
		for _, msg := range result.Errors {
			log.Printf("error: %s", msg)
		}
		os.Exit(1)
	}
	fmt.Printf("job-description-search complete: seen=%d inserted=%d updated=%d\n", result.Seen, result.Inserted, result.Updated)
}

func startHealth(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("health server: %v", err)
		}
	}()
	return server
}

func recordHistory(ctx context.Context, conn *sql.DB, started time.Time, result jobs.Result, runErr error) error {
	status := schema.ProcessStatusSuccess
	if runErr != nil {
		status = schema.ProcessStatusFailure
		if result.Seen > result.Inserted+result.Updated {
			status = schema.ProcessStatusPartial
		}
	}
	data, _ := json.Marshal(map[string]int{"seen": result.Seen, "inserted": result.Inserted, "updated": result.Updated})
	var errorJSON any
	if len(result.Errors) > 0 || runErr != nil {
		payload, _ := json.Marshal(map[string]any{"error": fmt.Sprint(runErr), "details": result.Errors})
		errorJSON = string(payload)
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO job_history(service, started_at, finished_at, status, items_processed, data_json, error_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"job-description-search", started.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), status, result.Seen, string(data), errorJSON)
	return err
}
