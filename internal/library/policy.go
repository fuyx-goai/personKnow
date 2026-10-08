package library

import "github.com/google/uuid"

func DecideAccess(actorID uuid.UUID, library Library) Access {
	if actorID == library.OwnerUserID {
		return AccessOwner
	}
	if library.Status == StatusActive && library.Visibility == VisibilityPublic {
		return AccessRead
	}
	return AccessNone
}
