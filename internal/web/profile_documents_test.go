package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileSuggestionsAndDocumentUpload(t *testing.T) {
	s := newTestServer(t)
	uid := insertLiveUser(t, s.DB, "documents@example.com")
	h := s.Routes()
	cookies := []*http.Cookie{sessionCookieFor(t, s, uid)}
	csrf, extra := getAPICSRF(t, h, cookies)
	cookies = append(cookies, extra...)

	for _, seed := range []struct{ table, name string }{{"titles", "Software Engineer"}, {"titles", "Software Architect"}, {"companies", "Example Corp"}} {
		if _, err := s.DB.Exec(`INSERT INTO `+seed.table+`(name) VALUES (?)`, seed.name); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]string{"/api/titles?q=soft": "Software Engineer", "/api/companies?q=Exam": "Example Corp"} {
		rec := getJSON(t, h, path, cookies)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("suggestions %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "resume.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("%PDF document")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/documents", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "resume.pdf") {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stored []byte
	var owner int64
	if err := s.DB.QueryRow(`SELECT user_id, content FROM documents WHERE filename = ?`, "resume.pdf").Scan(&owner, &stored); err != nil {
		t.Fatal(err)
	}
	if owner != uid || string(stored) != "%PDF document" {
		t.Fatalf("document owner/content mismatch: owner=%d body=%q", owner, stored)
	}
	list := getJSON(t, h, "/api/documents", cookies)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "resume.pdf") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	if _, err := s.DB.Exec(`INSERT INTO user_configs(user_id, desired_titles_json, blocked_companies_json) VALUES (?, '[]', '[]') ON CONFLICT(user_id) DO UPDATE SET desired_titles_json='[]', blocked_companies_json='[]'`, uid); err != nil {
		t.Fatal(err)
	}
	csrf, _ = getAPICSRF(t, h, cookies)
	profile := putJSON(t, h, "/api/profile", map[string]string{"desiredTitles": "Software Engineer", "blockedCompanies": "Example Corp"}, csrf, cookies)
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), "Software Engineer") {
		t.Fatalf("profile update status=%d body=%s", profile.Code, profile.Body.String())
	}
}
