package provider

import (
	"context"
	"testing"
)

func TestDiagnosticDispatchNeverChangesTarget(t *testing.T) {
	ctx := WithInvocation(context.Background(), Invocation{Diagnostic: true, Provider: "channel", Caller: Caller{TestAccount: "target"}})
	for _, ref := range []AccountRef{{Provider: "other", LocalID: "target"}, {Provider: "channel", LocalID: "other"}} {
		if CheckDispatch(ctx, ref, "m") == nil {
			t.Fatal("target boundary bypassed", ref)
		}
	}
	if err := CheckDispatch(ctx, AccountRef{Provider: "channel", LocalID: "target"}, "m"); err != nil {
		t.Fatal(err)
	}
}
