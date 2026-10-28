package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/matching"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, stdout io.Writer) error {
	flags := flag.NewFlagSet("job-match-search", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dbPath := getenv("JOB_DB_PATH")
	defaultMatcher := "jev"
	flags.StringVar(&dbPath, "db", dbPath, "path to sqlite database")
	flags.StringVar(&defaultMatcher, "matcher", defaultMatcher, "global default matcher")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if dbPath == "" {
		return fmt.Errorf("database path required via -db or JOB_DB_PATH")
	}
	classifiers, err := classifiersFromEnv(getenv)
	if err != nil {
		return fmt.Errorf("configure classifiers: %w", err)
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		return fmt.Errorf("prepare database: %w", err)
	}
	engine := matching.Engine{
		DB:          conn,
		Classifiers: classifiers,
		Options:     matching.Options{DefaultMatcher: defaultMatcher},
	}
	summary, err := engine.Run(ctx)
	_, writeErr := fmt.Fprintf(stdout, "processed=%d matched=%d no_match=%d failures=%d\n", summary.Processed, summary.Matched, summary.NoMatch, summary.Failures)
	if err != nil {
		return err
	}
	return writeErr
}

func classifiersFromEnv(getenv func(string) string) (map[string]matching.Classifier, error) {
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(getenv("JOB_MATCH_LLM_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid JOB_MATCH_LLM_TIMEOUT %q", raw)
		}
		timeout = parsed
	}
	jev, err := matching.NewLLMClassifier(matching.LLMConfig{
		Endpoint: getenv("JOB_MATCH_LLM_URL"),
		Model:    getenv("JOB_MATCH_LLM_MODEL"),
		APIKey:   getenv("JOB_MATCH_LLM_API_KEY"),
		Timeout:  timeout,
	})
	if err != nil {
		return nil, err
	}
	return map[string]matching.Classifier{"jev": jev}, nil
}
