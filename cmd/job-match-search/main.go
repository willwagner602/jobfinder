package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/matching"
)

func main() {
	var dbPath, defaultMatcher string
	flag.StringVar(&dbPath, "db", os.Getenv("JOB_DB_PATH"), "path to sqlite database")
	flag.StringVar(&defaultMatcher, "matcher", "jev", "global default matcher")
	flag.Parse()
	if dbPath == "" {
		log.Fatal("database path required via -db or JOB_DB_PATH")
	}
	classifiers, err := classifiersFromEnv()
	if err != nil {
		log.Fatalf("configure classifiers: %v", err)
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
		DB:          conn,
		Classifiers: classifiers,
		Options:     matching.Options{DefaultMatcher: defaultMatcher},
	}
	summary, err := engine.Run(ctx)
	fmt.Printf("processed=%d matched=%d no_match=%d failures=%d\n", summary.Processed, summary.Matched, summary.NoMatch, summary.Failures)
	if err != nil {
		log.Fatal(err)
	}
}

func classifiersFromEnv() (map[string]matching.Classifier, error) {
	jev, err := matching.NewLLMClassifierFromEnv()
	if err != nil {
		return nil, err
	}
	return map[string]matching.Classifier{"jev": jev}, nil
}
