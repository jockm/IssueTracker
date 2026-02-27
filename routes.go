package main

import (
	"io/fs"
	"net/http"
)

func registerRoutes(mux *http.ServeMux, dataDir string, staticSub fs.FS) {
	// Static files
	mux.Handle("/", http.FileServer(http.FS(staticSub)))

	// API
	mux.HandleFunc("/api/scan", handleScan(dataDir))
	mux.HandleFunc("/api/issue/create", handleCreateIssue(dataDir))
	mux.HandleFunc("/api/issue/read", handleReadIssue(dataDir))
	mux.HandleFunc("/api/issue/update", handleUpdateIssue(dataDir))
	mux.HandleFunc("/api/issue/delete", handleDeleteIssue(dataDir))
	mux.HandleFunc("/api/issue/move", handleMoveIssue(dataDir))
	mux.HandleFunc("/api/status/create", handleCreateStatus(dataDir))
	mux.HandleFunc("/api/status/rename", handleRenameStatus(dataDir))
}
