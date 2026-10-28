package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/jobs"
	"jobfinder/internal/schema"
)

func main() {
	dbPath := flag.String("db", "jobs.sqlite", "SQLite database path")
	sinceFlag := flag.String("since", time.Now().UTC().AddDate(0, 0, -8).Format("2006-01-02"), "oldest posting date to request when providers support server-side filtering")
	healthAddr := flag.String("health-addr", "", "optional address for liveness endpoint /healthz while the run is active")
	flag.Parse()
	if err := run(*dbPath, *sinceFlag, *healthAddr); err != nil {
		log.Fatal(err)
	}
}

func run(dbPath, sinceFlag, healthAddr string) error {
	ctx := context.Background()
	since, err := time.Parse("2006-01-02", sinceFlag)
	if err != nil {
		return fmt.Errorf("parse -since: %w", err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		return fmt.Errorf("ensure database schema: %w", err)
	}
	var server *http.Server
	if healthAddr != "" {
		server = startHealth(healthAddr)
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
		return runErr
	}
	fmt.Printf("job-description-search complete: seen=%d inserted=%d updated=%d\n", result.Seen, result.Inserted, result.Updated)
	return nil
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
