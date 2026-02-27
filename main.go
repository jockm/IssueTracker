package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//go:embed static
var staticFiles embed.FS

const version = "v0.1.0"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "IssueTracker %s\n\n", version)
		fmt.Fprintf(os.Stderr, "A local web-based issue tracker that uses the filesystem for storage.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  issuetracker [flags] <data-directory>\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	port := flag.Int("port", 0, "TCP port to listen on (default: auto-detect free port)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: data directory argument is required.")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	dataDir := flag.Arg(0)
	info, err := os.Stat(dataDir)
	if err != nil {
		log.Fatalf("Cannot access data directory %q: %v", dataDir, err)
	}
	if !info.IsDir() {
		log.Fatalf("%q is not a directory", dataDir)
	}

	listener, err := newListener(*port)
	if err != nil {
		log.Fatalf("Could not open port: %v", err)
	}

	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("Could not access embedded static files: %v", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, dataDir, staticSub)

	addr := listener.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://localhost:%d", addr.Port)
	log.Printf("IssueTracker %s listening on %s", version, url)
	log.Printf("Data directory: %s", dataDir)

	go func() {
		if err := http.Serve(listener, mux); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	waitForServer(url)
	openBrowser(url)

	// Block forever
	select {}
}

func newListener(port int) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return net.Listen("tcp", addr)
}

// waitForServer polls until the server accepts connections.
func waitForServer(url string) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser: %v", err)
	}
}
