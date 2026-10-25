package profiles

import (
	"regexp"
	"testing"
)

func TestBillingIdentityIsolationAndRequestUniqueness(t *testing.T) {
	first := map[string]string{"X-User-Id": "one", "Authorization": "Bearer fixture"}
	second := map[string]string{"X-User-Id": "two"}
	ApplyBillingIdentity(first)
	ApplyBillingIdentity(second)
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, h := range []map[string]string{first, second} {
		for _, name := range []string{"X-Machine-ID", "X-Session-ID"} {
			if !uuid.MatchString(h[name]) {
				t.Fatal("invalid identity shape")
			}
		}
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(h["X-Request-ID"]) {
			t.Fatal("invalid request identifier")
		}
	}
	machine, session, request := first["X-Machine-ID"], first["X-Session-ID"], first["X-Request-ID"]
	if machine == second["X-Machine-ID"] || session == second["X-Session-ID"] || machine == session {
		t.Fatal("identity not separated")
	}
	ApplyBillingIdentity(first)
	if first["X-Machine-ID"] != machine || first["X-Session-ID"] != session || first["X-Request-ID"] == request || first["Authorization"] != "Bearer fixture" {
		t.Fatal("identity stability or credential changed")
	}
	ApplyBillingIdentity(nil)
	unknown := map[string]string{}
	ApplyBillingIdentity(unknown)
	if len(unknown) != 0 {
		t.Fatal("unidentified credentials share a synthetic device")
	}
}
