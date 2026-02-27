# IssueTracker

A local web-based issue tracker that uses the filesystem for storage. No database required — issues are Markdown files, statuses are folders.

## Requirements

- Go 1.25 or later
- A modern web browser

## Installing Go

### Windows
1. Download the installer from https://go.dev/dl/ (choose the `.msi` file for your architecture).
2. Run the installer and follow the prompts.
3. Open a new Command Prompt and verify: `go version`

### macOS
Using Homebrew:
```sh
brew install go
```
Or download the `.pkg` installer from https://go.dev/dl/ and run it.
Verify: `go version`

### Linux
Download the tarball for your architecture from https://go.dev/dl/, then:
```sh
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile
go version
```

## Getting dependencies

From the project root (the directory containing `go.mod`):
```sh
go mod tidy
```

This downloads `gopkg.in/yaml.v3` and writes `go.sum`.

## Building

### macOS / Linux
```sh
go build -o issuetracker .
```

### Windows (Command Prompt or PowerShell)
```cmd
go build -o issuetracker.exe .
```

## Cross-compiling

Go makes cross-compilation straightforward by setting `GOOS` and `GOARCH` environment variables.

### From macOS or Linux — build for Windows (64-bit)
```sh
GOOS=windows GOARCH=amd64 go build -o issuetracker.exe .
```

### From macOS or Linux — build for Linux (64-bit)
```sh
GOOS=linux GOARCH=amd64 go build -o issuetracker .
```

### From macOS or Linux — build for macOS (Apple Silicon)
```sh
GOOS=darwin GOARCH=arm64 go build -o issuetracker-arm64 .
```

### From macOS or Linux — build for macOS (Intel)
```sh
GOOS=darwin GOARCH=amd64 go build -o issuetracker-amd64 .
```

### From Windows (PowerShell) — build for Linux (64-bit)
```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o issuetracker .
```

### From Windows (PowerShell) — build for macOS (Apple Silicon)
```powershell
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -o issuetracker .
```

## Usage

```
issuetracker [flags] <data-directory>

Flags:
  --help        Show this help message
  --port int    TCP port to listen on (default: auto-detect free port)
```

The `data-directory` is where your status folders and issue files live. It will be created if it doesn't exist — actually, it must already exist; create it first if needed.

Example:
```sh
mkdir ~/my-issues
./issuetracker ~/my-issues
```

The app opens your default browser automatically. If no `--port` is given, a free port is chosen and printed to the console.

## Filesystem layout

```
<data-directory>/
  Open/
    my-first-issue.md
    another-issue.md
  In Progress/
    big-feature.md
  Closed/
    old-bug.md
```

Each `.md` file must begin with YAML front matter:

```
---
assignee: alice
priority: High
---

# Issue Title

Description text goes here.

# Comments

**2026-02-26 14:32:00 UTC** —

This is a comment.
```

Fields missing from front matter default to `UNKNOWN`.

## Module name

`issuetracker`

## Version

v0.1.0
