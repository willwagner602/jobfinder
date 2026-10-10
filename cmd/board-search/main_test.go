package main

import "testing"

func TestBuildProvidersRequiresFixturePath(t *testing.T) {
	_, err := buildProviders("fixture", "", "", "", "us", "")
	if err == nil { t.Fatal("expected fixture path error") }
}

func TestBuildProvidersRejectsUnknownProvider(t *testing.T) {
	_, err := buildProviders("fixture,nope", "candidates.json", "", "", "us", "")
	if err == nil { t.Fatal("expected unknown provider error") }
}
