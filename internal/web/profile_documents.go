package web

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

const maxDocumentSize = 25 << 20

func (s *Server) documentCount(ctx context.Context, userID int64) int {
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM documents WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (s *Server) apiTitleSuggestions(w http.ResponseWriter, r *http.Request, _ *user) {
	s.suggestNames(w, r, "titles", "name")
}

func (s *Server) apiCompanySuggestions(w http.ResponseWriter, r *http.Request, _ *user) {
	s.suggestNames(w, r, "companies", "name")
}

func (s *Server) suggestNames(w http.ResponseWriter, r *http.Request, table, column string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		s.writeJSON(w, http.StatusOK, map[string][]string{"items": {}})
		return
	}
	rows, err := s.DB.QueryContext(r.Context(), fmt.Sprintf(`SELECT %s FROM %s WHERE %s LIKE ? COLLATE NOCASE ORDER BY %s LIMIT 10`, column, table, column, column), query+"%")
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "suggestions_failed", "Suggestions could not be loaded")
		return
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			s.writeAPIError(w, http.StatusInternalServerError, "suggestions_failed", "Suggestions could not be loaded")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "suggestions_failed", "Suggestions could not be loaded")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]string{"items": items})
}

func (s *Server) apiDocuments(w http.ResponseWriter, r *http.Request, u *user) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT document_id, filename, content_type, size_bytes, created_at FROM documents WHERE user_id = ? ORDER BY document_id`, u.ID)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "documents_failed", "Documents could not be loaded")
		return
	}
	defer rows.Close()
	documents := []apiDocument{}
	for rows.Next() {
		var document apiDocument
		if err := rows.Scan(&document.ID, &document.Filename, &document.ContentType, &document.SizeBytes, &document.CreatedAt); err != nil {
			s.writeAPIError(w, http.StatusInternalServerError, "documents_failed", "Documents could not be loaded")
			return
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "documents_failed", "Documents could not be loaded")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string][]apiDocument{"documents": documents})
}

func (s *Server) apiUploadDocument(w http.ResponseWriter, r *http.Request, u *user) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentSize+1<<20)
	if err := r.ParseMultipartForm(maxDocumentSize); err != nil {
		s.writeAPIError(w, http.StatusRequestEntityTooLarge, "document_too_large", "Document upload is too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "document_required", "Choose a document to upload")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxDocumentSize+1))
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "document_read_failed", "Document could not be read")
		return
	}
	if len(content) == 0 {
		s.writeAPIError(w, http.StatusBadRequest, "document_empty", "The selected document is empty")
		return
	}
	if len(content) > maxDocumentSize {
		s.writeAPIError(w, http.StatusRequestEntityTooLarge, "document_too_large", "Document upload is too large")
		return
	}
	filename := filepath.Base(strings.TrimSpace(header.Filename))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		s.writeAPIError(w, http.StatusBadRequest, "document_name_invalid", "Document filename is invalid")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	result, err := s.DB.ExecContext(r.Context(), `INSERT INTO documents(user_id, filename, content_type, size_bytes, content) VALUES (?, ?, ?, ?, ?)`, u.ID, filename, contentType, len(content), content)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "document_save_failed", "Document could not be saved")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "document_save_failed", "Document could not be saved")
		return
	}
	var document apiDocument
	err = s.DB.QueryRowContext(r.Context(), `SELECT document_id, filename, content_type, size_bytes, created_at FROM documents WHERE document_id = ? AND user_id = ?`, id, u.ID).Scan(&document.ID, &document.Filename, &document.ContentType, &document.SizeBytes, &document.CreatedAt)
	if err != nil && err != sql.ErrNoRows {
		s.writeAPIError(w, http.StatusInternalServerError, "document_save_failed", "Document could not be saved")
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]apiDocument{"document": document})
}
