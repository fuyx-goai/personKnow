package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	. "knowledge-base/internal/library/entity"
	usage "knowledge-base/internal/usage/entity"
)

type libraryAuditRecorder struct {
	commands []usage.AuditCommand
}

func (recorder *libraryAuditRecorder) RecordAudit(_ context.Context, command usage.AuditCommand) error {
	recorder.commands = append(recorder.commands, command)
	return nil
}

func TestCreateLibraryWritesAuditRecord(t *testing.T) {
	recorder := &libraryAuditRecorder{}
	service := NewService(&fakeRepository{}).UseAuditor(recorder)
	library, err := service.Create(context.Background(), uuid.New(), CreateCommand{Name: "审计知识库"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.commands) != 1 || recorder.commands[0].Action != "library.create" {
		t.Fatalf("missing library audit: %+v", recorder.commands)
	}
	if recorder.commands[0].ResourceID == nil || *recorder.commands[0].ResourceID != library.ID {
		t.Fatalf("audit resource does not match library: %+v", recorder.commands[0])
	}
}
