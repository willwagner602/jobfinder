package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/matching"
	"jobs.willwagner.me/internal/schema"
)

type keywordClassifier struct{}

func (keywordClassifier) Classify(_ context.Context, req matching.Request) (matching.Result, error) {
	text := strings.ToLower(req.Title + "\n" + req.BodyText)
	for _, desired := range req.DesiredTitles {
		desired = strings.TrimSpace(strings.ToLower(desired))
		if desired != "" && strings.Contains(text, desired) {
			return matching.Result{Decision: schema.JobMatchDecisionMatch, Rating: 4, Data: raw(map[string]string{"reason":"keyword_match"})}, nil
		}
	}
	return matching.Result{Decision: schema.JobMatchDecisionNoMatch, Data: raw(map[string]string{"reason":"no_keyword_match"})}, nil
}

func main() {
	var dbPath, defaultMatcher string
	flag.StringVar(&dbPath, "db", os.Getenv("JOB_DB_PATH"), "path to sqlite database")
	flag.StringVar(&defaultMatcher, "matcher", "jev", "global default matcher")
	flag.Parse()
	if dbPath == "" {
		log.Fatal("database path required via -db or JOB_DB_PATH")
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		log.Fatal(err)
	}
	engine := matching.Engine{
		DB: conn,
		Classifiers: map[string]matching.Classifier{
			"jev": keywordClassifier{},
		},
		Options: matching.Options{DefaultMatcher: defaultMatcher},
	}
	summary, err := engine.Run(ctx)
	fmt.Printf("processed=%d matched=%d no_match=%d failures=%d\n", summary.Processed, summary.Matched, summary.NoMatch, summary.Failures)
	if err != nil {
		log.Fatal(err)
	}
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
