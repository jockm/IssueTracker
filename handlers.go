package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// safeChild verifies that joining parent+child stays within parent (no path traversal).
// child must not contain slashes. Returns the cleaned absolute path or an error.
func safeChild(parent, child string) (string, error) {
	if strings.ContainsAny(child, "/\\") {
		return "", fmt.Errorf("invalid path component: contains slash")
	}
	if child == "" || child == "." || child == ".." {
		return "", fmt.Errorf("invalid path component")
	}
	full := filepath.Clean(filepath.Join(parent, child))
	cleanParent := filepath.Clean(parent) + string(filepath.Separator)
	if !strings.HasPrefix(full+string(filepath.Separator), cleanParent) {
		return "", fmt.Errorf("invalid path: would escape data directory")
	}
	return full, nil
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// jsonOK writes a JSON success response.
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleScan returns the full scan of the data directory.
func handleScan(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		result := scanDataDir(dataDir)
		jsonOK(w, result)
	}
}

// CreateIssueRequest is the payload for creating a new issue.
type CreateIssueRequest struct {
	Status      string `json:"status"`
	ID          string `json:"id"`
	Assignee    string `json:"assignee"`
	Priority    string `json:"priority"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func handleCreateIssue(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CreateIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.ID == "" {
			jsonError(w, "id is required", http.StatusBadRequest)
			return
		}
		if req.Status == "" {
			jsonError(w, "status is required", http.StatusBadRequest)
			return
		}
		// IDs must not contain dots (would create e.g. foo.bar.md) or slashes
		if strings.ContainsAny(req.ID, "/\\") || strings.Contains(req.ID, ".") {
			jsonError(w, "id cannot contain slashes or dots", http.StatusBadRequest)
			return
		}

		statusDir, err := safeChild(dataDir, req.Status)
		if err != nil {
			jsonError(w, "invalid status name", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(statusDir); os.IsNotExist(err) {
			jsonError(w, "status does not exist", http.StatusBadRequest)
			return
		}

		// ID is already validated to not contain slashes; safe to join
		filePath := filepath.Join(statusDir, req.ID+".md")
		if _, err := os.Stat(filePath); err == nil {
			jsonError(w, "an issue with that ID already exists in this status", http.StatusConflict)
			return
		}

		assignee := req.Assignee
		if assignee == "" {
			assignee = "UNKNOWN"
		}
		priority := req.Priority
		if priority == "" {
			priority = "Normal"
		}
		title := req.Title
		if title == "" {
			title = "UNKNOWN"
		}

		fm := FrontMatter{Assignee: assignee, Priority: priority}
		if err := serializeIssue(filePath, fm, title, req.Description, "# Comments", "", ""); err != nil {
			jsonError(w, fmt.Sprintf("could not write file: %v", err), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]string{"id": req.ID, "status": req.Status})
	}
}

func handleReadIssue(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := r.URL.Query().Get("status")
		id := r.URL.Query().Get("id")
		if status == "" || id == "" {
			jsonError(w, "status and id are required", http.StatusBadRequest)
			return
		}

		statusDir, err := safeChild(dataDir, status)
		if err != nil {
			jsonError(w, "invalid status", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(id, "/\\.") {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(statusDir, id+".md")
		issue, warn := loadIssue(filePath, id, status)
		if warn != "" {
			jsonError(w, warn, http.StatusNotFound)
			return
		}

		info, _ := os.Stat(filePath)
		type ReadResponse struct {
			Issue    Issue  `json:"issue"`
			Modified string `json:"modified"`
		}
		jsonOK(w, ReadResponse{Issue: issue, Modified: info.ModTime().Format(time.RFC3339Nano)})
	}
}

// UpdateIssueRequest is the payload for updating an existing issue.
type UpdateIssueRequest struct {
	Status           string `json:"status"`
	ID               string `json:"id"`
	Assignee         string `json:"assignee"`
	Priority         string `json:"priority"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	CommentHeader    string `json:"commentHeader"`
	ExistingComments string `json:"existingComments"`
	NewComment       string `json:"newComment"`
}

func handleUpdateIssue(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req UpdateIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		statusDir, err := safeChild(dataDir, req.Status)
		if err != nil {
			jsonError(w, "invalid status", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(req.ID, "/\\.") || req.ID == "" {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(statusDir, req.ID+".md")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			jsonError(w, "issue not found", http.StatusNotFound)
			return
		}

		assignee := req.Assignee
		if assignee == "" {
			assignee = "UNKNOWN"
		}
		priority := req.Priority
		if priority == "" {
			priority = "UNKNOWN"
		}
		title := req.Title
		if title == "" {
			title = "UNKNOWN"
		}

		// Format new comment with timestamp + timezone if provided
		newComment := ""
		if strings.TrimSpace(req.NewComment) != "" {
			now := time.Now()
			zoneName, _ := now.Zone()
			ts := now.Format("2006-01-02 15:04:05") + " " + zoneName
			newComment = fmt.Sprintf("**%s** —\n\n%s", ts, strings.TrimSpace(req.NewComment))
		}

		fm := FrontMatter{Assignee: assignee, Priority: priority}
		if err := serializeIssue(filePath, fm, title, req.Description, req.CommentHeader, req.ExistingComments, newComment); err != nil {
			jsonError(w, fmt.Sprintf("could not write file: %v", err), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// DeleteIssueRequest is the payload for deleting an issue.
type DeleteIssueRequest struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

func handleDeleteIssue(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req DeleteIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		statusDir, err := safeChild(dataDir, req.Status)
		if err != nil {
			jsonError(w, "invalid status", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(req.ID, "/\\.") || req.ID == "" {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(statusDir, req.ID+".md")
		if err := os.Remove(filePath); err != nil {
			if os.IsNotExist(err) {
				jsonError(w, "issue not found", http.StatusNotFound)
			} else {
				jsonError(w, fmt.Sprintf("could not delete file: %v", err), http.StatusInternalServerError)
			}
			return
		}

		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// MoveIssueRequest is the payload for moving an issue to a different status.
type MoveIssueRequest struct {
	ID         string `json:"id"`
	FromStatus string `json:"fromStatus"`
	ToStatus   string `json:"toStatus"`
}

func handleMoveIssue(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req MoveIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.FromStatus == req.ToStatus {
			jsonOK(w, map[string]string{"status": "ok"})
			return
		}

		fromDir, err := safeChild(dataDir, req.FromStatus)
		if err != nil {
			jsonError(w, "invalid source status", http.StatusBadRequest)
			return
		}
		toDir, err := safeChild(dataDir, req.ToStatus)
		if err != nil {
			jsonError(w, "invalid destination status", http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(req.ID, "/\\.") || req.ID == "" {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}

		src := filepath.Join(fromDir, req.ID+".md")
		dst := filepath.Join(toDir, req.ID+".md")

		if _, err := os.Stat(src); os.IsNotExist(err) {
			jsonError(w, "source issue not found", http.StatusNotFound)
			return
		}
		if _, err := os.Stat(toDir); os.IsNotExist(err) {
			jsonError(w, "destination status not found", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(dst); err == nil {
			jsonError(w, "an issue with that ID already exists in the destination status", http.StatusConflict)
			return
		}

		if err := os.Rename(src, dst); err != nil {
			jsonError(w, fmt.Sprintf("could not move file: %v", err), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]string{"status": "ok"})
	}
}

// CreateStatusRequest is the payload for creating a new status folder.
type CreateStatusRequest struct {
	Name string `json:"name"`
}

func handleCreateStatus(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CreateStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := validateStatusName(req.Name); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		dir, err := safeChild(dataDir, req.Name)
		if err != nil {
			jsonError(w, "invalid status name", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(dir); err == nil {
			jsonError(w, "a status with that name already exists", http.StatusConflict)
			return
		}

		if err := os.Mkdir(dir, 0755); err != nil {
			jsonError(w, fmt.Sprintf("could not create directory: %v", err), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]string{"name": req.Name})
	}
}

// RenameStatusRequest is the payload for renaming a status folder.
type RenameStatusRequest struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

func handleRenameStatus(dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req RenameStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := validateStatusName(req.NewName); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		oldDir, err := safeChild(dataDir, req.OldName)
		if err != nil {
			jsonError(w, "invalid old status name", http.StatusBadRequest)
			return
		}
		newDir, err := safeChild(dataDir, req.NewName)
		if err != nil {
			jsonError(w, "invalid new status name", http.StatusBadRequest)
			return
		}

		if _, err := os.Stat(oldDir); os.IsNotExist(err) {
			jsonError(w, "status not found", http.StatusNotFound)
			return
		}
		if _, err := os.Stat(newDir); err == nil {
			jsonError(w, "a status with that name already exists", http.StatusConflict)
			return
		}

		if err := os.Rename(oldDir, newDir); err != nil {
			jsonError(w, fmt.Sprintf("could not rename directory: %v", err), http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]string{"oldName": req.OldName, "newName": req.NewName})
	}
}
