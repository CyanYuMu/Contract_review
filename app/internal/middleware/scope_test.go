package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

type fakeResolver struct {
	scope ResourceScope
	err   error
	calls int
}

func (f *fakeResolver) Resolve(ctx context.Context, account string) (ResourceScope, error) {
	f.calls++
	f.scope.Account = account
	if f.err != nil {
		return ResourceScope{}, f.err
	}
	return f.scope, nil
}

func TestDBIdentityResolverNoCache(t *testing.T) {
	var lookupCalls int
	lookup := func(ctx context.Context, account string) (uint64, string, error) {
		lookupCalls++
		if account != "alice" {
			t.Fatalf("got account %q want alice", account)
		}
		return 42, "member", nil
	}
	r := NewDBIdentityResolver(lookup, nil) // nil cache -> direct lookup every time

	scope, err := r.Resolve(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if scope.UserID != 42 || scope.SystemRole != "member" || scope.Account != "alice" {
		t.Fatalf("unexpected scope: %+v", scope)
	}
	if scope.OrganizationID != DefaultOrganizationID {
		t.Fatalf("expected default org id, got %d", scope.OrganizationID)
	}
	if lookupCalls != 1 {
		t.Fatalf("lookup called %d times, want 1", lookupCalls)
	}
}

func TestDBIdentityResolverEmptyAccount(t *testing.T) {
	r := NewDBIdentityResolver(func(ctx context.Context, account string) (uint64, string, error) {
		t.Fatal("lookup should not be called for empty account")
		return 0, "", nil
	}, nil)
	if _, err := r.Resolve(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty account")
	}
}

func TestDBIdentityResolverLookupError(t *testing.T) {
	sentinel := errors.New("db down")
	r := NewDBIdentityResolver(func(ctx context.Context, account string) (uint64, string, error) {
		return 0, "", sentinel
	}, nil)
	_, err := r.Resolve(context.Background(), "bob")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestResolveScopeSetsContext(t *testing.T) {
	fr := &fakeResolver{scope: ResourceScope{UserID: 9, SystemRole: "admin", OrganizationID: DefaultOrganizationID}}
	c := app.NewContext(0)
	c.Set("account", "alice")

	ResolveScope(fr)(context.Background(), c)

	scope, ok := GetScope(c)
	if !ok {
		t.Fatal("scope not set in context")
	}
	if scope.UserID != 9 || scope.SystemRole != "admin" || scope.Account != "alice" {
		t.Fatalf("unexpected scope: %+v", scope)
	}
}

func TestResolveScopeAbortsWhenNoAccount(t *testing.T) {
	fr := &fakeResolver{}
	c := app.NewContext(0)
	// no account set -> should abort before calling resolver

	ResolveScope(fr)(context.Background(), c)

	if _, ok := GetScope(c); ok {
		t.Fatal("scope should not be set when account missing")
	}
	if fr.calls != 0 {
		t.Fatalf("resolver should not be called when account missing, called %d", fr.calls)
	}
}

func TestResolveScopeAbortsOnResolverError(t *testing.T) {
	fr := &fakeResolver{err: errors.New("lookup failed")}
	c := app.NewContext(0)
	c.Set("account", "alice")

	ResolveScope(fr)(context.Background(), c)

	if _, ok := GetScope(c); ok {
		t.Fatal("scope should not be set on resolver error")
	}
	if fr.calls != 1 {
		t.Fatalf("resolver should be called once, got %d", fr.calls)
	}
}
