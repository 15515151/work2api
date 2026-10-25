package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"work2api/internal/config"
	"work2api/internal/core/provider"
	"work2api/internal/store"
	"work2api/internal/workbuddy/credentials"
	"work2api/internal/workbuddy/models"
	"work2api/internal/workbuddy/pool"
)

func TestDisabledAccountEntersMaintenanceWithoutReenabling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.json")
	if err := os.WriteFile(path, []byte(`{"auth":{"accessToken":"fixture","refreshToken":"refresh","domain":"www.workbuddy.cn","expiresAt":1},"account":{"uid":"fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManager(path)
	r := &Runtime{Managers: map[string]*credentials.Manager{"fixture": mgr}, Pool: pool.New(map[string]pool.Credential{"fixture": mgr}, "")}
	r.Pool.SetEnabled("fixture", false, "手动停用")
	acc := r.Pool.Get("fixture")
	if err := r.prepareBillingMaintenance(context.Background(), acc); err != nil {
		t.Fatalf("manual disable incorrectly pauses maintenance: %v", err)
	}
	identity, err := mgr.MaintenanceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	g := r.billingGuard()
	g.State(acc.UID, identity)
	g.Observe(acc.UID, identity, "authorization_expired", true)
	if _, err := r.checkinAccount(context.Background(), acc); !errors.Is(err, provider.ErrMaintenanceLogin) {
		t.Fatalf("known invalid auth still reached check-in: %v", err)
	}
	if r.Pool.Get("fixture").Enabled {
		t.Fatal("maintenance reenabled disabled account")
	}
}

type maintenanceTransport func(*http.Request) (*http.Response, error)

func (f maintenanceTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func TestConfirmedExpiryCannotUseStickyScopedOrAdmissionSelection(t *testing.T) {
	dir := t.TempDir()
	db, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	managers := map[string]*credentials.Manager{}
	creds := map[string]pool.Credential{}
	for _, uid := range []string{"expired", "valid"} {
		path := filepath.Join(dir, uid+".info")
		body := `{"auth":{"accessToken":"fixture","refreshToken":"refresh","domain":"www.workbuddy.cn","expiresAt":` + fmt.Sprint(time.Now().Add(time.Hour).UnixMilli()) + `},"account":{"uid":"` + uid + `"}}`
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		managers[uid] = credentials.NewManager(path)
		creds[uid] = managers[uid]
	}
	r := &Runtime{Managers: managers, Pool: pool.New(creds, "")}
	r.Catalog = models.NewWithCatalogClient(r.Pool, db, &http.Client{Transport: maintenanceTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"models":[{"id":"m","credits":1}],"agents":[{"name":"cli","models":["m"]}]}}`))}, nil
	})})
	r.Configure(&config.Config{DataDir: dir}, db, NewSessionRouter())
	if _, err := r.Catalog.RefreshWithStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.Sessions.Bind("sticky", "expired")
	mgr := managers["expired"]
	identity, _ := mgr.MaintenanceIdentity()
	g := r.billingGuard()
	g.State("expired", identity)
	g.Observe("expired", identity, "authorization_expired", true)
	if r.ModelCallable(r.Pool.Get("expired"), "m") || r.ModelHealthy(r.Pool.Get("expired"), "m", nowSec()) || r.AccountHealthy(r.Pool.Get("expired"), nowSec()) {
		t.Fatal("invalid credential remains callable")
	}
	selected, e := r.PickAccount("m", "sticky")
	if e != nil || selected.UID != "valid" {
		t.Fatal("sticky selection kept invalid credential", e)
	}
	selector, e := r.AccountSelector("m", "expired", nil, map[string]bool{"expired": true, "valid": true})
	if e != nil {
		t.Fatal(e)
	}
	selected, e = selector(nil)
	if e != nil || selected.UID != "valid" {
		t.Fatal("admission preferred invalid credential", e)
	}
	if _, e = r.PickInScope("m", "sticky", nil, map[string]bool{"expired": true}); e == nil {
		t.Fatal("scoped selection bypassed auth exclusion")
	}
	if _, err := r.headers(context.Background(), r.Pool.Get("expired"), nil); err == nil {
		t.Fatal("send boundary accepted known-invalid credentials")
	}
	if !r.Pool.Get("expired").Enabled {
		t.Fatal("auth state changed manual enablement")
	}
}

func TestDiagnosticUsageDoesNotEnterUserStatistics(t *testing.T) {
	r := &Runtime{}
	ctx := provider.WithInvocation(context.Background(), provider.Invocation{Diagnostic: true})
	r.LogUsage(LogArgs{Ctx: ctx, Status: "ok", Usage: map[string]any{"prompt_tokens": 1, "completion_tokens": 1}})
	// No configured DB/usage sink: a diagnostic must return before persistence.
}

func TestReadOnlyCheckReusesFreshBillingWithoutEnablingAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "check.info")
	body := `{"auth":{"accessToken":"fixture","refreshToken":"refresh","domain":"www.workbuddy.cn","expiresAt":` + fmt.Sprint(time.Now().Add(time.Hour).UnixMilli()) + `},"account":{"uid":"fixture"}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	mgr := credentials.NewManager(path)
	identity, _ := mgr.MaintenanceIdentity()
	r := &Runtime{Managers: map[string]*credentials.Manager{"fixture": mgr}, Pool: pool.New(map[string]pool.Credential{"fixture": mgr}, ""), creditStates: map[string]creditState{"fixture": {Manager: mgr, Identity: identity, Updated: nowSec()}}}
	r.Pool.SetEnabled("fixture", false, "手动停用")
	result := r.CheckAccount(context.Background(), "fixture", false)
	if result.Status != "valid" || r.Pool.Get("fixture").Enabled {
		t.Fatal("fresh check made extra requests or enabled account", result)
	}
}
