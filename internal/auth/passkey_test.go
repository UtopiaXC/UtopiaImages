package auth

import (
	"testing"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestWebAuthnMultiDomain(t *testing.T) {
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Utopia Images",
		RPID:          "imgs.utopiaxc.com",
		RPOrigins:     []string{"https://imgs.utopiaxc.com", "https://imgs.utopiaxc.cn"},
	})
	if err != nil {
		t.Fatalf("failed to create webauthn: %v", err)
	}

	user := &WebAuthnUser{User: &model.User{ID: 1, Username: "admin"}}

	// Registration on domain 1 (.com)
	opts1, sess1, err := w.BeginRegistration(user,
		webauthn.WithRegistrationRelyingPartyID("imgs.utopiaxc.com"),
		webauthn.WithRegistrationOrigin("https://imgs.utopiaxc.com"),
	)
	if err != nil {
		t.Fatalf("BeginRegistration .com failed: %v", err)
	}
	if opts1.Response.RelyingParty.ID != "imgs.utopiaxc.com" {
		t.Errorf("expected .com RPID, got %s", opts1.Response.RelyingParty.ID)
	}
	if sess1.RelyingPartyID != "imgs.utopiaxc.com" {
		t.Errorf("expected session RPID imgs.utopiaxc.com, got %s", sess1.RelyingPartyID)
	}

	// Registration on domain 2 (.cn)
	opts2, sess2, err := w.BeginRegistration(user,
		webauthn.WithRegistrationRelyingPartyID("imgs.utopiaxc.cn"),
		webauthn.WithRegistrationOrigin("https://imgs.utopiaxc.cn"),
	)
	if err != nil {
		t.Fatalf("BeginRegistration .cn failed: %v", err)
	}
	if opts2.Response.RelyingParty.ID != "imgs.utopiaxc.cn" {
		t.Errorf("expected .cn RPID, got %s", opts2.Response.RelyingParty.ID)
	}
	if sess2.RelyingPartyID != "imgs.utopiaxc.cn" {
		t.Errorf("expected session RPID imgs.utopiaxc.cn, got %s", sess2.RelyingPartyID)
	}

	// Login on domain 2 (.cn) using discoverable credentials
	loginOpts, loginSess, err := w.BeginDiscoverableLogin(
		webauthn.WithLoginRelyingPartyID("imgs.utopiaxc.cn"),
		webauthn.WithLoginOrigin("https://imgs.utopiaxc.cn"),
	)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin .cn failed: %v", err)
	}
	if loginOpts.Response.RelyingPartyID != "imgs.utopiaxc.cn" {
		t.Errorf("expected login RPID imgs.utopiaxc.cn, got %s", loginOpts.Response.RelyingPartyID)
	}
	if loginSess.RelyingPartyID != "imgs.utopiaxc.cn" {
		t.Errorf("expected login session RPID imgs.utopiaxc.cn, got %s", loginSess.RelyingPartyID)
	}
}
