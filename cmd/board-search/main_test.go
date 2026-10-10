package main

import (
	"strings"
	"testing"
)

func TestDefaultProvidersUseDailySources(t *testing.T) {
	providers, err := buildProviders(defaultProviders, "", "adzuna-id", "adzuna-key", "us", "serp-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(providers))
	}
	if providers[0].Name() != "adzuna" || providers[1].Name() != "serpapi" {
		t.Fatalf("providers = %s,%s; want adzuna,serpapi", providers[0].Name(), providers[1].Name())
	}
}

func TestDefaultProvidersRequireCredentials(t *testing.T) {
	_, err := buildProviders(defaultProviders, "", "", "", "us", "")
	if err == nil {
		t.Fatal("expected credential error")
	}
	got := err.Error()
	if !strings.Contains(got, "adzuna provider requires") {
		t.Fatalf("error = %q, want adzuna credential guidance", got)
	}
}

func TestSerpAPIProviderRequiresCredential(t *testing.T) {
	_, err := buildProviders("serpapi", "", "", "", "us", "")
	if err == nil {
		t.Fatal("expected serpapi credential error")
	}
	if !strings.Contains(err.Error(), "SERPAPI_API_KEY") {
		t.Fatalf("error = %q, want SERPAPI_API_KEY guidance", err.Error())
	}
}

func TestBuildProvidersRequiresExplicitFixturePath(t *testing.T) {
	_, err := buildProviders("fixture", "", "", "", "us", "")
	if err == nil {
		t.Fatal("expected fixture path error")
	}
	providers, err := buildProviders("fixture", "candidates.json", "", "", "us", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Name() != "fixture" {
		t.Fatalf("fixture providers = %+v", providers)
	}
}

func TestBuildProvidersRejectsUnknownProvider(t *testing.T) {
	_, err := buildProviders("fixture,nope", "candidates.json", "", "", "us", "")
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
}
