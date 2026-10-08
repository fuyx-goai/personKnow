package library

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	usage "knowledge-base/internal/usage"
)

type Service struct {
	repository Repository
	reindexer  ReindexScheduler
	auditor    usage.Auditor
	now        func() time.Time
}

func (service *Service) UseAuditor(auditor usage.Auditor) *Service {
	service.auditor = auditor
	return service
}

func NewService(repository Repository, reindexers ...ReindexScheduler) *Service {
	service := &Service{repository: repository, now: time.Now}
	if len(reindexers) > 0 {
		service.reindexer = reindexers[0]
	}
	return service
}

func (service *Service) Reindex(ctx context.Context, actorID, libraryID uuid.UUID) ([]uuid.UUID, error) {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if DecideAccess(actorID, library) != AccessOwner {
		return nil, ErrForbidden
	}
	if service.reindexer == nil {
		return nil, ErrReindexUnavailable
	}
	jobs, err := service.reindexer.ScheduleLibrary(ctx, actorID, libraryID)
	if err != nil {
		return nil, err
	}
	return jobs, service.audit(ctx, actorID, "library.reindex", libraryID, "queued")
}

func (service *Service) List(ctx context.Context, actorID uuid.UUID, filter ListFilter) (ListResult, error) {
	owned, err := service.repository.ListOwned(ctx, actorID, filter)
	if err != nil {
		return ListResult{}, err
	}
	publicLibraries, err := service.repository.ListPublic(ctx, actorID, filter)
	if err != nil {
		return ListResult{}, err
	}
	for index := range owned {
		owned[index].Access = AccessOwner
	}
	for index := range publicLibraries {
		publicLibraries[index].Access = AccessRead
	}
	return ListResult{Owned: owned, Public: publicLibraries}, nil
}

func (service *Service) Create(ctx context.Context, actorID uuid.UUID, command CreateCommand) (Library, error) {
	now := service.now()
	visibility := command.Visibility
	if visibility == "" {
		visibility = VisibilityPrivate
	}
	library := Library{
		ID: uuid.New(), OwnerUserID: actorID, Name: strings.TrimSpace(command.Name),
		Category: normalizedCategory(command.Category), Description: strings.TrimSpace(command.Description),
		Visibility: visibility, Status: StatusActive, CreatedAt: now, UpdatedAt: now, Access: AccessOwner,
	}
	if err := library.Validate(); err != nil {
		return Library{}, err
	}
	settings := DefaultRetrievalSettings()
	settings.LibraryID, settings.UpdatedBy = library.ID, actorID
	if err := service.repository.Create(ctx, library, settings); err != nil {
		return Library{}, err
	}
	return library, service.audit(ctx, actorID, "library.create", library.ID, "active")
}

func (service *Service) Get(ctx context.Context, actorID, libraryID uuid.UUID) (Library, error) {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return Library{}, err
	}
	access := DecideAccess(actorID, library)
	if access == AccessNone {
		return Library{}, ErrNotFound
	}
	library.Access = access
	return library, nil
}

func (service *Service) Update(ctx context.Context, actorID, libraryID uuid.UUID, command UpdateCommand) error {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return err
	}
	if DecideAccess(actorID, library) != AccessOwner {
		return ErrForbidden
	}
	applyUpdate(&library, command)
	if err := library.Validate(); err != nil {
		return err
	}
	library.UpdatedAt = service.now()
	if err := service.repository.Update(ctx, library); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "library.update", library.ID, "active")
}

func (service *Service) Delete(ctx context.Context, actorID, libraryID uuid.UUID) error {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return err
	}
	if DecideAccess(actorID, library) != AccessOwner {
		return ErrForbidden
	}
	if err := service.repository.MarkDeleting(ctx, libraryID); err != nil {
		return err
	}
	return service.audit(ctx, actorID, "library.delete", libraryID, "deleting")
}

func (service *Service) Settings(ctx context.Context, actorID, libraryID uuid.UUID) (RetrievalSettings, error) {
	if _, err := service.Get(ctx, actorID, libraryID); err != nil {
		return RetrievalSettings{}, err
	}
	return service.repository.GetSettings(ctx, libraryID)
}

func (service *Service) UpdateSettings(ctx context.Context, actorID, libraryID uuid.UUID, next RetrievalSettings) (SettingsUpdateResult, error) {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return SettingsUpdateResult{}, err
	}
	if DecideAccess(actorID, library) != AccessOwner {
		return SettingsUpdateResult{}, ErrForbidden
	}
	current, err := service.repository.GetSettings(ctx, libraryID)
	if err != nil {
		return SettingsUpdateResult{}, err
	}
	next.LibraryID, next.UpdatedBy = libraryID, actorID
	if err := next.Validate(); err != nil {
		return SettingsUpdateResult{}, err
	}
	requiresIndex := current.ChunkSize != next.ChunkSize || current.ChunkOverlap != next.ChunkOverlap
	if err := service.repository.UpdateSettings(ctx, next); err != nil {
		return SettingsUpdateResult{}, err
	}
	result := SettingsUpdateResult{Settings: next, RequiresIndex: requiresIndex}
	return result, service.audit(ctx, actorID, "library.retrieval_settings.update", libraryID, "updated")
}

func (service *Service) audit(ctx context.Context, actorID uuid.UUID, action string, resourceID uuid.UUID, status string) error {
	if service.auditor == nil {
		return nil
	}
	return service.auditor.RecordAudit(ctx, usage.AuditCommand{
		ActorUserID: &actorID, Action: action, ResourceType: "library", ResourceID: &resourceID,
		Result: usage.AuditSuccess, Metadata: map[string]any{"status": status},
	})
}

func applyUpdate(library *Library, command UpdateCommand) {
	if strings.TrimSpace(command.Name) != "" {
		library.Name = strings.TrimSpace(command.Name)
	}
	if strings.TrimSpace(command.Category) != "" {
		library.Category = normalizedCategory(command.Category)
	}
	if command.Description != "" {
		library.Description = strings.TrimSpace(command.Description)
	}
	if command.Visibility != "" {
		library.Visibility = command.Visibility
	}
}

func normalizedCategory(category string) string {
	if normalized := strings.TrimSpace(category); normalized != "" {
		return normalized
	}
	return "uncategorized"
}
