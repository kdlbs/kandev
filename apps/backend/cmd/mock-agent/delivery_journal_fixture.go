package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

const journalFixtureRead = "read"

// The mock binary seeds closed test-owned journals and inspects copies without
// taking the live owner's lock or introducing a production mutation endpoint.
func runDeliveryJournalFixture(mode string, input io.Reader, output io.Writer) error {
	var request struct {
		Path       string             `json:"path"`
		Submission journal.Submission `json:"submission"`
	}
	if err := json.NewDecoder(input).Decode(&request); err != nil {
		return err
	}
	if mode != "seed" && mode != journalFixtureRead {
		return fmt.Errorf("unknown delivery journal fixture mode")
	}
	journalPath := request.Path
	if mode == journalFixtureRead {
		dir, err := os.MkdirTemp("", "kandev-journal-inspection-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		journalPath = filepath.Join(dir, "delivery.bbolt")
		if err := copyFixtureJournal(request.Path, journalPath); err != nil {
			return err
		}
	}
	store, err := journal.Open(journal.Config{Path: journalPath})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	var retained journal.Submission
	if mode == "seed" {
		retained, err = store.PutSubmission(context.Background(), request.Submission)
	} else {
		retained, err = store.GetSubmission(context.Background(), request.Submission.ID)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(retained)
}

func copyFixtureJournal(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}
