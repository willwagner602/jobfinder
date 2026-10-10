package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"

	"jobs.willwagner.me/internal/web"
)

func main() {
	dbPath := env("JOBS_DB", "jobs.db")
	addr := env("WEB_ADDR", ":8080")
	templateDir := env("TEMPLATE_DIR", "templates")
	key := sessionKey()

	srv, err := web.New(dbPath, templateDir, key, web.LogNotifier{}, web.NoopMatchRunner{})
	if err != nil {
		log.Fatal(err)
	}
	defer srv.DB.Close()

	log.Printf("web listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func sessionKey() []byte {
	if encoded := os.Getenv("SESSION_KEY"); encoded != "" {
		if raw, err := base64.RawURLEncoding.DecodeString(encoded); err == nil && len(raw) >= 32 {
			return raw
		}
		if len(encoded) >= 32 {
			return []byte(encoded)
		}
		log.Fatal("SESSION_KEY must be at least 32 bytes or raw-url-base64 encoded 32 bytes")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatal(err)
	}
	log.Print("SESSION_KEY is not set; generated an ephemeral key, so sessions will reset on restart")
	return key
}
