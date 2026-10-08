package library

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	reindexer  ReindexScheduler
	now        func() time.Time
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
	return service.reindexer.ScheduleLibrary(ctx, actorID, libraryID)
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
	return library, nil
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
	return service.repository.Update(ctx, library)
}

func (service *Service) Delete(ctx context.Context, actorID, libraryID uuid.UUID) error {
	library, err := service.repository.Get(ctx, libraryID)
	if err != nil {
		return err
	}
	if DecideAccess(actorID, library) != AccessOwner {
		return ErrForbidden
	}
	return service.repository.MarkDeleting(ctx, libraryID)
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
	return SettingsUpdateResult{Settings: next, RequiresIndex: requiresIndex}, nil
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
