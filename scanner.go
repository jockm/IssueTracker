package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FrontMatter holds the YAML front matter of an issue file.
type FrontMatter struct {
	Assignee string `yaml:"assignee"`
	Priority string `yaml:"priority"`
}

// Issue represents a single issue loaded from a .md file.
type Issue struct {
	ID            string    `json:"id"`
	Assignee      string    `json:"assignee"`
	Priority      string    `json:"priority"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	CommentHeader string    `json:"commentHeader"`
	Comments      string    `json:"comments"`
	Modified      time.Time `json:"modified"`
	Status        string    `json:"status"`
}

// Status represents a status folder and its issues.
type Status struct {
	Name   string  `json:"name"`
	Issues []Issue `json:"issues"`
}

// ScanResult is returned by the /api/scan endpoint.
type ScanResult struct {
	Statuses  []Status  `json:"statuses"`
	Warnings  []string  `json:"warnings"`
	ScannedAt time.Time `json:"scannedAt"`
}

// scanDataDir scans the data directory and returns all statuses and issues.
func scanDataDir(dataDir string) ScanResult {
	result := ScanResult{ScannedAt: time.Now()}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Cannot read data directory: %v", err))
		return result
	}

	var statusNames []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			statusNames = append(statusNames, e.Name())
		}
	}
	sort.Strings(statusNames)

	for _, statusName := range statusNames {
		statusDir := filepath.Join(dataDir, statusName)
		status, warnings := loadStatus(statusDir, statusName)
		result.Statuses = append(result.Statuses, status)
		result.Warnings = append(result.Warnings, warnings...)
	}

	return result
}

// loadStatus reads all .md files from a status directory.
func loadStatus(statusDir, statusName string) (Status, []string) {
	status := Status{Name: statusName}
	var warnings []string

	entries, err := os.ReadDir(statusDir)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Cannot read status directory %q: %v", statusName, err))
		return status, warnings
	}

	seenIDs := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		id := strings.TrimSuffix(e.Name(), ".md")
		filePath := filepath.Join(statusDir, e.Name())

		if seenIDs[id] {
			warnings = append(warnings, fmt.Sprintf("Duplicate ID %q in status %q", id, statusName))
			continue
		}
		seenIDs[id] = true

		issue, w := loadIssue(filePath, id, statusName)
		if w != "" {
			warnings = append(warnings, w)
			continue
		}
		status.Issues = append(status.Issues, issue)
	}

	return status, warnings
}

// loadIssue parses a single .md file into an Issue.
// Returns a non-empty warning string and zero Issue if the file should be skipped.
func loadIssue(filePath, id, statusName string) (Issue, string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Issue{}, fmt.Sprintf("Cannot read file %q: %v", filePath, err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return Issue{}, fmt.Sprintf("Cannot stat file %q: %v", filePath, err)
	}

	fm, body, err := parseFrontMatter(data)
	if err != nil {
		return Issue{}, fmt.Sprintf("Invalid front matter in %q (%s/%s.md): %v", filePath, statusName, id, err)
	}

	if fm.Assignee == "" {
		fm.Assignee = "UNKNOWN"
	}
	if fm.Priority == "" {
		fm.Priority = "UNKNOWN"
	}

	title, description, commentHeader, comments := parseBody(body)

	return Issue{
		ID:            id,
		Assignee:      fm.Assignee,
		Priority:      fm.Priority,
		Title:         title,
		Description:   description,
		CommentHeader: commentHeader,
		Comments:      comments,
		Modified:      info.ModTime(),
		Status:        statusName,
	}, ""
}

// parseFrontMatter splits YAML front matter from the markdown body.
// Returns an error if the front matter block is missing or unparseable.
func parseFrontMatter(data []byte) (FrontMatter, []byte, error) {
	var fm FrontMatter

	// Must start with ---\n or ---\r\n
	var headerLen int
	if bytes.HasPrefix(data, []byte("---\r\n")) {
		headerLen = 5
	} else if bytes.HasPrefix(data, []byte("---\n")) {
		headerLen = 4
	} else {
		return fm, nil, fmt.Errorf("missing front matter delimiter")
	}

	// Find closing ---
	rest := data[headerLen:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx == -1 {
		return fm, nil, fmt.Errorf("unclosed front matter block")
	}

	yamlBlock := rest[:idx]
	body := rest[idx+4:] // skip \n---
	// Skip the newline after closing ---
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	} else if len(body) > 1 && body[0] == '\r' && body[1] == '\n' {
		body = body[2:]
	}

	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return fm, nil, fmt.Errorf("YAML parse error: %w", err)
	}

	return fm, body, nil
}

// parseBody extracts title, description, comment header, and comments from markdown body.
func parseBody(body []byte) (title, description, commentHeader, comments string) {
	title = "UNKNOWN"
	commentHeader = "# Comments"

	lines := strings.Split(string(body), "\n")

	firstH1 := -1
	secondH1 := -1

	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			if firstH1 == -1 {
				firstH1 = i
			} else if secondH1 == -1 {
				secondH1 = i
				break
			}
		}
	}

	if firstH1 != -1 {
		title = strings.TrimPrefix(lines[firstH1], "# ")
		title = strings.TrimSpace(title)
	}

	if firstH1 != -1 && secondH1 != -1 {
		descLines := lines[firstH1+1 : secondH1]
		description = strings.TrimSpace(strings.Join(descLines, "\n"))
		commentHeader = strings.TrimSpace(lines[secondH1])
		commentLines := lines[secondH1+1:]
		comments = strings.TrimSpace(strings.Join(commentLines, "\n"))
	} else if firstH1 != -1 {
		descLines := lines[firstH1+1:]
		description = strings.TrimSpace(strings.Join(descLines, "\n"))
	}

	return
}

// serializeIssue writes an Issue back to disk as a .md file.
func serializeIssue(filePath string, fm FrontMatter, title, description, commentHeader, existingComments, newComment string) error {
	if fm.Assignee == "" {
		fm.Assignee = "UNKNOWN"
	}
	if fm.Priority == "" {
		fm.Priority = "UNKNOWN"
	}
	if commentHeader == "" {
		commentHeader = "# Comments"
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("cannot marshal front matter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n\n")

	buf.WriteString("# ")
	buf.WriteString(title)
	buf.WriteString("\n\n")

	if description != "" {
		buf.WriteString(description)
		buf.WriteString("\n\n")
	}

	buf.WriteString(commentHeader)
	buf.WriteString("\n\n")

	if existingComments != "" {
		buf.WriteString(existingComments)
		buf.WriteString("\n\n")
	}

	if newComment != "" {
		buf.WriteString(newComment)
		buf.WriteString("\n")
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// validateStatusName checks a proposed status folder name.
func validateStatusName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("name cannot start with a dash")
	}
	if strings.HasPrefix(name, " ") || strings.HasPrefix(name, "\t") {
		return fmt.Errorf("name cannot start with whitespace")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name cannot contain slashes")
	}
	return nil
}
