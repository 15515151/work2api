package bridge

import (
	"testing"
	"work2api/internal/qoder/account"
	"work2api/internal/qoder/cosy"
)

func TestOptionalDesktopFingerprintPreservesLegacy(t *testing.T) {
	sess, e := cosy.NewSession(cosy.AuthIdentity{Uid: "fixture"}, "legacy-id", "legacy-token", "windows")
	if e != nil {
		t.Fatal(e)
	}
	c := NewBearerClient(sess)
	h, e := c.buildHeaders("/models", "", "application/json", nil)
	if e != nil || h["cosy-clienttype"] != "5" || h["cosy-machineid"] != "legacy-id" {
		t.Fatal("legacy changed")
	}
	c.fingerprint = &account.CheckinFingerprint{MachineID: "real-id", MachineToken: "real-token", MachineType: "windows"}
	h, e = c.buildHeaders("/models", "", "application/json", nil)
	if e != nil || h["cosy-clienttype"] != "10" || h["cosy-machineid"] != "real-id" || h["cosy-version"] != "1.1.69" {
		t.Fatal("desktop identity not forwarded")
	}
}
