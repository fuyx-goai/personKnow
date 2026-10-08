package main

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseLegacyCommandRequiresAllArguments(t *testing.T) {
	userID := uuid.New()
	options, err := parseCommand([]string{
		"legacy", "--target-user", userID.String(), "--source", "old/knowledge.json", "--backup-dir", "backups",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.name != "legacy" || options.targetUser != userID || options.source != "old/knowledge.json" || options.backupDir != "backups" {
		t.Fatalf("options = %+v", options)
	}

	invalid := [][]string{
		{"legacy", "--source", "knowledge.json", "--backup-dir", "backups"},
		{"legacy", "--target-user", "not-a-uuid", "--source", "knowledge.json", "--backup-dir", "backups"},
		{"legacy", "--target-user", userID.String(), "--backup-dir", "backups"},
		{"legacy", "--target-user", userID.String(), "--source", "knowledge.json"},
	}
	for _, args := range invalid {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("parseCommand(%v) succeeded, want error", args)
		}
	}
}

func TestParseSchemaCommands(t *testing.T) {
	for _, name := range []string{"up", "status"} {
		options, err := parseCommand([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		if options.name != name {
			t.Fatalf("name = %q, want %q", options.name, name)
		}
	}
}
