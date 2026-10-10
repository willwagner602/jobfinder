package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	"jobs.willwagner.me/internal/web"
)

// run starts the web service using environment-driven configuration and keeps
// serving until ListenAndServe returns.
func run() error {
	dbPath := env("JOBS_DB", "jobs.db")
	addr := env("WEB_ADDR", ":8080")
	templateDir := env("TEMPLATE_DIR", "templates")
	key, err := sessionKey()
	if err != nil {
		return err
	}
	notifier, err := web.NewSMTPNotifier(web.SMTPConfig{
		Addr:     os.Getenv("JOBS_SMTP_ADDR"),
		Username: os.Getenv("JOBS_SMTP_USERNAME"),
		Password: os.Getenv("JOBS_SMTP_PASSWORD"),
		From:     env("JOBS_SMTP_FROM", "jobs@willwagner.me"),
	})
	if err != nil {
		return err
	}
	srv, err := web.New(dbPath, templateDir, key, notifier, web.NoopMatchRunner{})
	if err != nil {
		return err
	}
	defer srv.DB.Close()
	log.Printf("web listening on %s", addr)
	return http.ListenAndServe(addr, srv.Routes())
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func sessionKey() ([]byte, error) {
	if encoded := os.Getenv("SESSION_KEY"); encoded != "" {
		if raw, err := base64.RawURLEncoding.DecodeString(encoded); err == nil && len(raw) >= 32 {
			return raw, nil
		}
		if len(encoded) >= 32 {
			return []byte(encoded), nil
		}
		return nil, fmt.Errorf("SESSION_KEY must be at least 32 bytes or raw-url-base64 encoded 32 bytes")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	log.Print("SESSION_KEY is not set; generated an ephemeral key, so sessions will reset on restart")
	return key, nil
}
