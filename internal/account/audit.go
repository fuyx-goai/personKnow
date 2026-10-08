package account

import (
	"context"

	"github.com/google/uuid"

	usage "knowledge-base/internal/usage"
)

func (service *Service) audit(ctx context.Context, actorID uuid.UUID, action, resourceType string, resourceID uuid.UUID, clientType, status string) error {
	if service.auditor == nil {
		return nil
	}
	return service.auditor.RecordAudit(ctx, usage.AuditCommand{
		ActorUserID: &actorID, Action: action, ResourceType: resourceType, ResourceID: &resourceID,
		Result: usage.AuditSuccess, ClientType: clientType, Metadata: map[string]any{"status": status},
	})
}
