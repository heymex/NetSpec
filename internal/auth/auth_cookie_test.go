package auth

import "testing"

func TestSessionCookieSecureFlag(t *testing.T) {
	t.Parallel()
	m := NewManager("$2a$10$abcdefghijklmnopqrstuv", "")
	c := m.SessionCookie("sid")
	if c.Secure {
		t.Fatal("Secure should be false until HTTPS is enabled")
	}
	m.SetSecureCookies(true)
	c = m.SessionCookie("sid")
	if !c.Secure {
		t.Fatal("Secure should be true when HTTPS is enabled")
	}
}
