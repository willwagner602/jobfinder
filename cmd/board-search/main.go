package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"jobs.willwagner.me/internal/boards"
	"jobs.willwagner.me/internal/db"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("board-search", flag.ContinueOnError)
	dbPath := fs.String("db", "jobs.sqlite3", "SQLite database path")
	jsonPath := fs.String("json", "data/job-boards.json", "job board JSON synchronization path")
	providerList := fs.String("providers", "fixture", "comma-separated providers: fixture,adzuna,serpapi")
	fixturePath := fs.String("fixture", "", "candidate fixture JSON path")
	adzunaID := fs.String("adzuna-app-id", os.Getenv("ADZUNA_APP_ID"), "Adzuna app id")
	adzunaKey := fs.String("adzuna-app-key", os.Getenv("ADZUNA_APP_KEY"), "Adzuna app key")
	adzunaCountry := fs.String("adzuna-country", "us", "Adzuna country")
	serpKey := fs.String("serpapi-key", os.Getenv("SERPAPI_API_KEY"), "SerpAPI key")
	if err := fs.Parse(args); err != nil { return err }

	conn, err := db.Open(*dbPath)
	if err != nil { return err }
	defer conn.Close()
	if err := db.EnsureCurrent(ctx, conn); err != nil { return err }

	providers, err := buildProviders(*providerList, *fixturePath, *adzunaID, *adzunaKey, *adzunaCountry, *serpKey)
	if err != nil { return err }
	result, err := boards.Run(ctx, conn, providers, boards.Config{JSONPath: *jsonPath})
	encoded, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr == nil { fmt.Println(string(encoded)) }
	return err
}

func buildProviders(list, fixturePath, adzunaID, adzunaKey, adzunaCountry, serpKey string) ([]boards.Provider, error) {
	var providers []boards.Provider
	for _, name := range strings.Split(list, ",") {
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "", "none":
			continue
		case "fixture":
			if fixturePath == "" { return nil, fmt.Errorf("-fixture is required when fixture provider is enabled") }
			providers = append(providers, boards.FixtureProvider{Path: fixturePath})
		case "adzuna":
			providers = append(providers, boards.AdzunaProvider{AppID: adzunaID, AppKey: adzunaKey, Country: adzunaCountry})
		case "serpapi":
			providers = append(providers, boards.SerpAPIProvider{APIKey: serpKey})
		default:
			return nil, fmt.Errorf("unknown provider %q", name)
		}
	}
	if len(providers) == 0 { return nil, fmt.Errorf("at least one provider is required") }
	return providers, nil
}
