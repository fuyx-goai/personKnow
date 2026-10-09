package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "knowledge-base/internal/document/entity"
	documentrepo "knowledge-base/internal/document/repo"
	librarydomain "knowledge-base/internal/library/entity"
	usage "knowledge-base/internal/usage/entity"
)

type documentAuditRecorder struct {
	commands []usage.AuditCommand
}

func (recorder *documentAuditRecorder) RecordAudit(_ context.Context, command usage.AuditCommand) error {
	recorder.commands = append(recorder.commands, command)
	return nil
}

func TestUploadDocumentWritesAuditRecord(t *testing.T) {
	userID, libraryID := uuid.New(), uuid.New()
	recorder := &documentAuditRecorder{}
	service := NewService(Dependencies{
		Repository: &fakeDocumentRepository{},
		Libraries: fakeLibraryReader{library: librarydomain.Library{
			ID: libraryID, OwnerUserID: userID, Access: librarydomain.AccessOwner, Status: librarydomain.StatusActive,
		}},
		Files: documentrepo.NewLocalFileStore(t.TempDir()), Quota: &fakeStorageQuota{}, Jobs: &fakeJobScheduler{}, Auditor: recorder,
	})
	result, err := service.Upload(context.Background(), userID, UploadCommand{
		LibraryID: libraryID, OriginalName: "audit.md", Reader: strings.NewReader("audit content"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.commands) != 1 || recorder.commands[0].Action != "document.upload" {
		t.Fatalf("missing document audit: %+v", recorder.commands)
	}
	if recorder.commands[0].ResourceID == nil || *recorder.commands[0].ResourceID != result.Document.ID {
		t.Fatalf("audit resource does not match document: %+v", recorder.commands[0])
	}
}
