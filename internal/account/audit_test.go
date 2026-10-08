package account

import (
	"context"
	"testing"

	usage "knowledge-base/internal/usage"
)

type accountAuditRecorder struct {
	commands []usage.AuditCommand
}

func (recorder *accountAuditRecorder) RecordAudit(_ context.Context, command usage.AuditCommand) error {
	recorder.commands = append(recorder.commands, command)
	return nil
}

func TestWeChatLoginWritesAuditRecord(t *testing.T) {
	recorder := &accountAuditRecorder{}
	service := newAccountServiceForTest(t).UseAuditor(recorder)
	result, err := service.WeChatLogin(context.Background(), LoginCommand{Code: "audit", ClientType: ClientMiniProgram})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.commands) != 1 || recorder.commands[0].Action != "account.login" {
		t.Fatalf("missing login audit: %+v", recorder.commands)
	}
	if recorder.commands[0].ActorUserID == nil || *recorder.commands[0].ActorUserID != result.User.ID {
		t.Fatalf("audit actor does not match login user: %+v", recorder.commands[0])
	}
}
