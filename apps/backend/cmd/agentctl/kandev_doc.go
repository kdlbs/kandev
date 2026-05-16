package main

import (
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func runDocCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev doc <create|read|list|upload> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdCreate:
		return docCreate(args[1:])
	case "read":
		return docRead(args[1:])
	case subcmdList:
		return docList(args[1:])
	case "upload":
		return docUpload(args[1:])
	default:
		cliError("unknown doc subcommand: %s", args[0])
		return 1
	}
}

// docCreate creates or updates a document for a task.
// Usage: kandev doc create <task-id> <key> --type <type> --title <title> --content <content>
func docCreate(args []string) int {
	// Go's flag.FlagSet.Parse stops at the first non-flag argument, so the
	// documented "positional THEN flags" form would silently leave --type /
	// --title / --content at their defaults. Peel off the two required
	// positionals first and parse the remainder as flags.
	if len(args) < 2 {
		cliError("usage: kandev doc create <task-id> <key> [flags]")
		return 1
	}
	taskID := args[0]
	key := args[1]

	fs := flag.NewFlagSet("doc create", flag.ContinueOnError)
	docType := fs.String("type", "custom", "Document type (plan, spec, notes, review, attachment, custom)")
	title := fs.String("title", "", "Document title")
	content := fs.String("content", "", "Document content (markdown). If empty, reads from stdin.")
	if err := fs.Parse(args[2:]); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	docContent := *content
	if docContent == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			cliError("read stdin: %v", err)
			return 1
		}
		docContent = string(data)
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	payload := map[string]interface{}{
		"type":    *docType,
		"title":   *title,
		"content": docContent,
	}

	body, status, doErr := client.do(http.MethodPut,
		fmt.Sprintf("/api/v1/tasks/%s/documents/%s", taskID, key), payload)
	return handleResponse(body, status, doErr)
}

// docRead fetches and prints the content of a document.
// Usage: kandev doc read <task-id> <key>
func docRead(args []string) int {
	fs := flag.NewFlagSet("doc read", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	positional := fs.Args()
	if len(positional) < 2 {
		cliError("usage: kandev doc read <task-id> <key>")
		return 1
	}
	taskID := positional[0]
	key := positional[1]

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	body, status, doErr := client.do(http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%s/documents/%s", taskID, key), nil)
	return handleResponse(body, status, doErr)
}

// docList lists all documents for a task.
// Usage: kandev doc list <task-id>
func docList(args []string) int {
	fs := flag.NewFlagSet("doc list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	positional := fs.Args()
	if len(positional) < 1 {
		cliError("usage: kandev doc list <task-id>")
		return 1
	}
	taskID := positional[0]

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	body, status, doErr := client.do(http.MethodGet,
		fmt.Sprintf("/api/v1/tasks/%s/documents", taskID), nil)
	return handleResponse(body, status, doErr)
}

// docUpload uploads a file as an attachment document.
// Usage: kandev doc upload <task-id> <key> <filepath>
func docUpload(args []string) int {
	fs := flag.NewFlagSet("doc upload", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	positional := fs.Args()
	if len(positional) < 3 {
		cliError("usage: kandev doc upload <task-id> <key> <filepath>")
		return 1
	}
	taskID := positional[0]
	key := positional[1]
	filePath := positional[2]

	data, err := os.ReadFile(filePath)
	if err != nil {
		cliError("read file %q: %v", filePath, err)
		return 1
	}

	filename := filepath.Base(filePath)
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	payload := map[string]interface{}{
		"filename":  filename,
		"mime_type": mimeType,
		"data":      data,
	}

	body, status, doErr := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/tasks/%s/documents/%s/upload", taskID, key), payload)
	return handleResponse(body, status, doErr)
}
