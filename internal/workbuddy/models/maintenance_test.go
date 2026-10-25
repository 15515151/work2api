package models

import (
	"context"
	"net/http"
	"testing"
	"work2api/internal/workbuddy/pool"
)

type maintenanceCredential struct {
	catalogTestCredential
	generation *string
	invalid    *bool
}

func (c maintenanceCredential) MaintenanceIdentity() (string, error) { return *c.generation, nil }
func (c maintenanceCredential) AuthorizationInvalid() bool           { return *c.invalid }
func TestCatalogAuthGateIsPerSourceAndCredential(t *testing.T) {
	generation := "first"
	invalid := false
	p := pool.New(map[string]pool.Credential{"account": maintenanceCredential{catalogTestCredential{profile: "cn-cli"}, &generation, &invalid}}, "")
	failed, healthy := 0, 0
	tr := isolatedCatalogTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/v3/config" {
			failed++
			return catalogResponse(req, 401, `{}`), nil
		}
		healthy++
		return catalogResponse(req, 200, catalogBody()), nil
	})
	r := NewWithCatalogClient(p, nil, &http.Client{Transport: tr})
	for i := 0; i < 3; i++ {
		r.RefreshWithStatus(context.Background())
	}
	if failed != 1 || healthy != 3 {
		t.Fatalf("source requests failed=%d healthy=%d", failed, healthy)
	}
	a := p.Get("account")
	if pending := r.CatalogMaintenance(a); len(pending) != 1 || pending[0]["needs_login"] != false {
		t.Fatal("source 401 incorrectly became account expiry", pending)
	}
	invalid = true
	r.RefreshWithStatus(context.Background())
	if failed != 1 || healthy != 3 {
		t.Fatal("confirmed invalid credential still fetched models")
	}
	invalid = false
	generation = "second"
	r.RefreshWithStatus(context.Background())
	if failed != 2 || healthy != 4 {
		t.Fatal("new credential did not resume directory validation")
	}
}

func TestCatalogBusinessAuthFailureDoesNotInvalidateWholeAccount(t *testing.T) {
	p := pool.New(map[string]pool.Credential{"account": catalogTestCredential{profile: "cn-cli"}}, "")
	calls := 0
	r := NewWithCatalogClient(p, nil, &http.Client{Transport: isolatedCatalogTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return catalogResponse(req, 200, `{"code":12153}`), nil
	})})
	a := p.Get("account")
	for i := 0; i < 2; i++ {
		if r.fetchMaintainedCatalog(context.Background(), "https://example.com/v3/config", nil, a, "cli") != nil {
			t.Fatal("auth failure accepted as catalog")
		}
	}
	if calls != 1 {
		t.Fatal("business auth failure repeated", calls)
	}
	pending := r.CatalogMaintenance(a)
	if len(pending) != 1 || pending[0]["kind"] != "authorization_unverified" || pending[0]["needs_login"] != false {
		t.Fatal(pending)
	}
}

func TestDirectoryAuthFailureDoesNotCoolModelDispatch(t *testing.T) {
	p := pool.New(map[string]pool.Credential{"account": catalogTestCredential{profile: "cn-cli"}}, "")
	r := NewWithCatalogClient(p, nil, &http.Client{Transport: isolatedCatalogTransport(func(req *http.Request) (*http.Response, error) { return catalogResponse(req, 401, `{}`), nil })})
	r.RefreshWithStatus(context.Background())
	if p.Get("account").CooldownUntil != 0 {
		t.Fatal("directory-only auth failure penalized model dispatch")
	}
}
