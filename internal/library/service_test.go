package library

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeRepository struct {
	library  Library
	settings RetrievalSettings
}

type fakeReindexScheduler struct {
	called bool
	jobs   []uuid.UUID
}

func (scheduler *fakeReindexScheduler) ScheduleLibrary(context.Context, uuid.UUID, uuid.UUID) ([]uuid.UUID, error) {
	scheduler.called = true
	return scheduler.jobs, nil
}

func (repo *fakeRepository) ListOwned(context.Context, uuid.UUID, ListFilter) ([]Library, error) {
	return []Library{repo.library}, nil
}

func (repo *fakeRepository) ListPublic(context.Context, uuid.UUID, ListFilter) ([]Library, error) {
	return nil, nil
}

func (repo *fakeRepository) Create(_ context.Context, library Library, settings RetrievalSettings) error {
	repo.library, repo.settings = library, settings
	return nil
}

func (repo *fakeRepository) Get(_ context.Context, id uuid.UUID) (Library, error) {
	if repo.library.ID != id {
		return Library{}, ErrNotFound
	}
	return repo.library, nil
}

func (repo *fakeRepository) Update(_ context.Context, library Library) error {
	repo.library = library
	return nil
}

func (repo *fakeRepository) MarkDeleting(context.Context, uuid.UUID) error { return nil }

func (repo *fakeRepository) GetSettings(context.Context, uuid.UUID) (RetrievalSettings, error) {
	return repo.settings, nil
}

func (repo *fakeRepository) UpdateSettings(_ context.Context, settings RetrievalSettings) error {
	repo.settings = settings
	return nil
}

func TestDecideAccess(t *testing.T) {
	ownerID, otherID := uuid.New(), uuid.New()
	tests := []struct {
		name       string
		actor      uuid.UUID
		visibility Visibility
		want       Access
	}{
		{name: "owner private", actor: ownerID, visibility: VisibilityPrivate, want: AccessOwner},
		{name: "owner public", actor: ownerID, visibility: VisibilityPublic, want: AccessOwner},
		{name: "reader public", actor: otherID, visibility: VisibilityPublic, want: AccessRead},
		{name: "reader private", actor: otherID, visibility: VisibilityPrivate, want: AccessNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			library := Library{OwnerUserID: ownerID, Visibility: test.visibility, Status: StatusActive}
			if got := DecideAccess(test.actor, library); got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestRetrievalSettingsValidate(t *testing.T) {
	valid := DefaultRetrievalSettings()
	if err := valid.Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
	invalid := valid
	invalid.ChunkOverlap = invalid.ChunkSize
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("expected invalid overlap, got %v", err)
	}
}

func TestPublicReaderCannotUpdateLibrary(t *testing.T) {
	ownerID, readerID := uuid.New(), uuid.New()
	repository := &fakeRepository{
		library:  Library{ID: uuid.New(), OwnerUserID: ownerID, Name: "公开库", Visibility: VisibilityPublic, Status: StatusActive},
		settings: DefaultRetrievalSettings(),
	}
	service := NewService(repository)
	err := service.Update(context.Background(), readerID, repository.library.ID, UpdateCommand{Name: "越权修改"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestPrivateLibraryIsHiddenFromOtherUsers(t *testing.T) {
	ownerID, readerID := uuid.New(), uuid.New()
	repository := &fakeRepository{
		library:  Library{ID: uuid.New(), OwnerUserID: ownerID, Name: "私有库", Visibility: VisibilityPrivate, Status: StatusActive},
		settings: DefaultRetrievalSettings(),
	}
	service := NewService(repository)
	_, err := service.Get(context.Background(), readerID, repository.library.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected hidden resource, got %v", err)
	}
}

func TestOnlyOwnerCanScheduleLibraryReindex(t *testing.T) {
	ownerID, readerID := uuid.New(), uuid.New()
	repository := &fakeRepository{
		library:  Library{ID: uuid.New(), OwnerUserID: ownerID, Name: "公开库", Visibility: VisibilityPublic, Status: StatusActive},
		settings: DefaultRetrievalSettings(),
	}
	scheduler := &fakeReindexScheduler{jobs: []uuid.UUID{uuid.New()}}
	service := NewService(repository, scheduler)
	if _, err := service.Reindex(context.Background(), readerID, repository.library.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected reader to be forbidden, got %v", err)
	}
	jobs, err := service.Reindex(context.Background(), ownerID, repository.library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !scheduler.called || len(jobs) != 1 {
		t.Fatalf("scheduler was not called: %#v", jobs)
	}
}
