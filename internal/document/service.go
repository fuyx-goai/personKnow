package document

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	librarydomain "knowledge-base/internal/library"
)

type Dependencies struct {
	Repository Repository
	Libraries  LibraryReader
	Files      *LocalFileStore
	Quota      StorageQuota
	Jobs       JobScheduler
	MaxBytes   int64
	Now        func() time.Time
}

type Service struct {
	repository Repository
	libraries  LibraryReader
	files      *LocalFileStore
	quota      StorageQuota
	jobs       JobScheduler
	maxBytes   int64
	now        func() time.Time
}

func NewService(dependencies Dependencies) *Service {
	if dependencies.MaxBytes <= 0 {
		dependencies.MaxBytes = 50 * 1024 * 1024
	}
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	return &Service{
		repository: dependencies.Repository, libraries: dependencies.Libraries, files: dependencies.Files,
		quota: dependencies.Quota, jobs: dependencies.Jobs, maxBytes: dependencies.MaxBytes, now: dependencies.Now,
	}
}

func (service *Service) Upload(ctx context.Context, actorID uuid.UUID, command UploadCommand) (UploadResult, error) {
	library, err := service.requireOwner(ctx, actorID, command.LibraryID)
	if err != nil {
		return UploadResult{}, err
	}
	documentID := uuid.New()
	location := Location{UserID: library.OwnerUserID, LibraryID: library.ID, DocumentID: documentID}
	stored, err := service.files.SaveOriginal(ctx, location, command.OriginalName, command.Reader, service.maxBytes)
	if err != nil {
		return UploadResult{}, err
	}
	format, mimeType, err := DetectFormat(service.files.Absolute(stored.RelativePath), command.OriginalName)
	if err != nil {
		_ = service.files.DeleteDocument(location)
		return UploadResult{}, err
	}
	if err := service.quota.ApplyStorage(ctx, actorID, documentID, stored.Size); err != nil {
		_ = service.files.DeleteDocument(location)
		return UploadResult{}, err
	}
	document := Document{
		ID: documentID, LibraryID: library.ID, UploadedBy: actorID, OriginalName: filepath.Base(command.OriginalName),
		DisplayName: filepath.Base(command.OriginalName), Format: format, MIMEType: mimeType,
		OriginalBytes: stored.Size, OriginalSHA256: stored.SHA256, OriginalPath: stored.RelativePath,
		Status: StatusQueued, Tags: normalizeTags(command.Tags), CreatedAt: service.now(), UpdatedAt: service.now(),
	}
	if err := service.repository.Create(ctx, document); err != nil {
		service.rollbackUpload(ctx, location, actorID, documentID, stored.Size)
		return UploadResult{}, err
	}
	jobID, err := service.jobs.ScheduleDocument(ctx, JobRequest{ActorID: actorID, LibraryID: library.ID, DocumentID: documentID, Type: JobIndex})
	if err != nil {
		return UploadResult{}, err
	}
	return UploadResult{Document: document, JobID: jobID}, nil
}

func (service *Service) List(ctx context.Context, actorID uuid.UUID, filter ListFilter) ([]Document, error) {
	return service.repository.List(ctx, actorID, filter)
}

func (service *Service) Get(ctx context.Context, actorID, documentID uuid.UUID) (Document, error) {
	return service.repository.Get(ctx, actorID, documentID)
}

func (service *Service) UpdateMetadata(ctx context.Context, actorID, documentID uuid.UUID, command MetadataCommand) error {
	document, library, err := service.ownedDocument(ctx, actorID, documentID)
	if err != nil {
		return err
	}
	if name := strings.TrimSpace(command.DisplayName); name != "" {
		if len([]rune(name)) > 255 {
			return fmt.Errorf("文件名不能超过 255 个字符")
		}
		document.DisplayName = name
	}
	document.Tags = normalizeTags(command.Tags)
	document.UpdatedAt = service.now()
	document.LibraryID = library.ID
	return service.repository.UpdateMetadata(ctx, document)
}

func (service *Service) EditContent(ctx context.Context, actorID, documentID uuid.UUID, text string) (EditResult, error) {
	document, library, err := service.ownedDocument(ctx, actorID, documentID)
	if err != nil {
		return EditResult{}, err
	}
	if strings.TrimSpace(text) == "" {
		return EditResult{}, fmt.Errorf("编辑内容不能为空")
	}
	version, err := service.repository.NextContentVersion(ctx, documentID)
	if err != nil {
		return EditResult{}, err
	}
	location := Location{UserID: library.OwnerUserID, LibraryID: library.ID, DocumentID: document.ID}
	stored, err := service.files.WriteContent(location, version, text)
	if err != nil {
		return EditResult{}, err
	}
	if err := service.quota.ApplyStorage(ctx, actorID, documentID, stored.Size); err != nil {
		return EditResult{}, err
	}
	content := ContentVersion{
		ID: uuid.New(), DocumentID: documentID, Version: version, SourceType: "edited",
		ContentPath: stored.RelativePath, ContentHash: stored.SHA256, TextBytes: stored.Size,
		StructureMap: map[string]any{}, CreatedBy: actorID, CreatedAt: service.now(),
	}
	if err := service.repository.CreateContentVersion(ctx, content); err != nil {
		return EditResult{}, err
	}
	jobID, err := service.jobs.ScheduleDocument(ctx, JobRequest{
		ActorID: actorID, LibraryID: library.ID, DocumentID: documentID, ContentVersionID: &content.ID, Type: JobReindex,
	})
	if err != nil {
		return EditResult{}, err
	}
	return EditResult{Content: content, JobID: jobID}, nil
}

func (service *Service) Content(ctx context.Context, actorID, documentID uuid.UUID) (ContentVersion, string, error) {
	if _, err := service.repository.Get(ctx, actorID, documentID); err != nil {
		return ContentVersion{}, "", err
	}
	content, err := service.repository.GetContent(ctx, actorID, documentID)
	if err != nil {
		return ContentVersion{}, "", err
	}
	data, err := service.files.Read(content.ContentPath)
	return content, string(data), err
}

func (service *Service) Reindex(ctx context.Context, actorID, documentID uuid.UUID) (uuid.UUID, error) {
	document, library, err := service.ownedDocument(ctx, actorID, documentID)
	if err != nil {
		return uuid.Nil, err
	}
	return service.jobs.ScheduleDocument(ctx, JobRequest{ActorID: actorID, LibraryID: library.ID, DocumentID: document.ID, ContentVersionID: document.ActiveContentVersionID, Type: JobReindex})
}

func (service *Service) Delete(ctx context.Context, actorID, documentID uuid.UUID) (uuid.UUID, error) {
	document, library, err := service.ownedDocument(ctx, actorID, documentID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := service.repository.MarkDeleting(ctx, documentID); err != nil {
		return uuid.Nil, err
	}
	return service.jobs.ScheduleDocument(ctx, JobRequest{ActorID: actorID, LibraryID: library.ID, DocumentID: document.ID, Type: JobDelete})
}

func (service *Service) ownedDocument(ctx context.Context, actorID, documentID uuid.UUID) (Document, librarydomain.Library, error) {
	document, err := service.repository.Get(ctx, actorID, documentID)
	if err != nil {
		return Document{}, librarydomain.Library{}, err
	}
	library, err := service.requireOwner(ctx, actorID, document.LibraryID)
	return document, library, err
}

func (service *Service) requireOwner(ctx context.Context, actorID, libraryID uuid.UUID) (librarydomain.Library, error) {
	library, err := service.libraries.Get(ctx, actorID, libraryID)
	if err != nil {
		return librarydomain.Library{}, err
	}
	if library.Access != librarydomain.AccessOwner {
		return librarydomain.Library{}, ErrDocumentReadOnly
	}
	return library, nil
}

func (service *Service) rollbackUpload(ctx context.Context, location Location, userID, documentID uuid.UUID, size int64) {
	_ = service.quota.ApplyStorage(ctx, userID, documentID, -size)
	_ = service.files.DeleteDocument(location)
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}
