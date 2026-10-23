package qoder

import (
	"context"
	"os"
	"testing"
	"work2api/internal/qoder/account"
	"work2api/internal/qoder/bridge"
)

func TestRenewalAtomicallyRotatesAndInvalidates(t *testing.T) {
	root := account.DataRoot()
	account.SetDataRoot(t.TempDir())
	defer account.SetDataRoot(root)
	a := &account.Account{ID: "fixture", AuthMode: "oauth", Region: account.RegionCN}
	account.Save(a)
	old := `{"device_token":"dt-old","refresh_token":"drt-old"}`
	account.SaveSecret(a.ID, old)
	previous := exchangeRenewal
	defer func() { exchangeRenewal = previous }()
	exchangeRenewal = func(ctx context.Context, cred, mid, mt, typ string, urls ...string) (map[string]interface{}, error) {
		if cred != "drt-old" {
			t.Fatal("did not use refresh token")
		}
		return map[string]any{"device_token": "dt-new", "refresh_token": "drt-new"}, nil
	}
	r := New("error")
	if e := r.RenewAccount(context.Background(), a.ID); e != nil {
		t.Fatal(e)
	}
	saved, _ := account.GetSecret(a.ID)
	dt, rt := bridge.ParseOAuthSecret(saved)
	if dt != "dt-new" || rt != "drt-new" || r.bridgeGeneration[a.ID] != 1 {
		t.Fatal("rotation not published")
	}
	if e := account.RotateSecret(a.ID, old, "overwrite"); e == nil {
		t.Fatal("stale writer overwritten newer token")
	}
	account.Delete(a.ID)
	if e := account.RotateSecret(a.ID, saved, "resurrect"); e == nil {
		t.Fatal("deleted account resurrected")
	}
}
func TestLiveNativeRenewal(t *testing.T) {
	if os.Getenv("WORK2API_QODER_RENEW_LIVE") != "1" {
		t.Skip("explicit live renewal")
	}
	as, e := account.List()
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range as {
		if a.AuthMode == "oauth" && !account.IsGatewayHidden(a.ID) {
			r := New("error")
			if e = r.renewToken(context.Background(), &a); e != nil {
				t.Fatal(e)
			}
			secret, _ := account.GetSecret(a.ID)
			dt, _ := bridge.ParseOAuthSecret(secret)
			ctx, cancel := context.WithTimeout(context.Background(), 20e9)
			defer cancel()
			if _, e = bridge.FetchUserInfoWithTokenContext(ctx, dt, a.Region); e != nil {
				t.Fatal("renewed credential validation failed")
			}
			t.Log("renewed credentials atomically saved and authenticated")
			return
		}
	}
	t.Fatal("no OAuth account")
}

func TestRenewalRejectsInvalidTokensAndConcurrentReplacement(t *testing.T) {
	root := account.DataRoot()
	account.SetDataRoot(t.TempDir())
	defer account.SetDataRoot(root)
	previous := exchangeRenewal
	defer func() { exchangeRenewal = previous }()
	a := &account.Account{ID: "renew-guard", AuthMode: "oauth", Region: account.RegionCN}
	if err := account.Save(a); err != nil {
		t.Fatal(err)
	}
	old := `{"device_token":"dt-old","refresh_token":"drt-old"}`
	for _, concurrent := range []bool{false, true} {
		if err := account.SaveSecret(a.ID, old); err != nil {
			t.Fatal(err)
		}
		want := old
		exchangeRenewal = func(context.Context, string, string, string, string, ...string) (map[string]interface{}, error) {
			if concurrent {
				want = `{"device_token":"dt-imported","refresh_token":"drt-imported"}`
				if err := account.SaveSecret(a.ID, want); err != nil {
					t.Fatal(err)
				}
				return map[string]any{"device_token": "dt-new", "refresh_token": "drt-new"}, nil
			}
			return map[string]any{"device_token": "dt-new", "refresh_token": "invalid"}, nil
		}
		r := New("error")
		if err := r.renewToken(context.Background(), a); err == nil {
			t.Fatal("unsafe rotation accepted")
		}
		got, err := account.GetSecret(a.ID)
		if err != nil || got != want || r.bridgeGeneration[a.ID] != 0 {
			t.Fatal("valid credential or cache overwritten")
		}
	}
}
