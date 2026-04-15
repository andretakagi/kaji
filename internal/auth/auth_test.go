package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	t.Run("correct password", func(t *testing.T) {
		hash, err := HashPassword("hunter2")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if !CheckPassword(hash, "hunter2") {
			t.Fatal("CheckPassword rejected the correct password")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		hash, err := HashPassword("hunter2")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if CheckPassword(hash, "wrong") {
			t.Fatal("CheckPassword accepted a wrong password")
		}
	})

	t.Run("empty password rejected", func(t *testing.T) {
		hash, err := HashPassword("notempty")
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if CheckPassword(hash, "") {
			t.Fatal("CheckPassword accepted an empty password")
		}
	})
}

func TestAPIKeyRoundTrip(t *testing.T) {
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	hashed := HashAPIKey(key)

	t.Run("correct key", func(t *testing.T) {
		if !CheckAPIKey(key, hashed) {
			t.Fatal("CheckAPIKey rejected the correct key")
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		if CheckAPIKey("deadbeef", hashed) {
			t.Fatal("CheckAPIKey accepted a wrong key")
		}
	})
}

func TestGenerateSessionToken(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken: %v", err)
	}

	// Should be 64 hex characters (32 random bytes).
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64", len(token))
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("token is not valid hex: %v", err)
	}

	// Two tokens should never collide.
	token2, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken (second): %v", err)
	}
	if token == token2 {
		t.Fatal("two generated tokens are identical")
	}
}

func TestSignAndValidateToken(t *testing.T) {
	secret := "test-secret-key"
	token := "abc123"

	t.Run("valid token", func(t *testing.T) {
		signed := SignToken(token, secret)
		if !ValidateSignedToken(signed, secret, 0) {
			t.Fatal("ValidateSignedToken rejected a valid token")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		signed := SignToken(token, secret)
		if ValidateSignedToken(signed, "wrong-secret", 0) {
			t.Fatal("ValidateSignedToken accepted a token signed with a different secret")
		}
	})

	t.Run("tampered token", func(t *testing.T) {
		signed := SignToken(token, secret)
		tampered := "tampered" + signed[len("abc123"):]
		if ValidateSignedToken(tampered, secret, 0) {
			t.Fatal("ValidateSignedToken accepted a tampered token")
		}
	})

	t.Run("two-part legacy rejected", func(t *testing.T) {
		if ValidateSignedToken("token.signature", secret, 0) {
			t.Fatal("ValidateSignedToken accepted a two-part legacy token")
		}
	})

	t.Run("freshly signed with short maxAge", func(t *testing.T) {
		signed := SignToken(token, secret)
		if !ValidateSignedToken(signed, secret, 1) {
			t.Fatal("ValidateSignedToken rejected a fresh token with maxAge=1")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		issuedAt := strconv.FormatInt(time.Now().Unix()-3600, 10)
		payload := token + "." + issuedAt
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(payload))
		sig := hex.EncodeToString(mac.Sum(nil))
		signed := token + "." + issuedAt + "." + sig
		if ValidateSignedToken(signed, secret, 60) {
			t.Fatal("ValidateSignedToken accepted an expired token")
		}
	})
}

func TestShouldSecure(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		useTLS  bool
		xfproto string
		want    bool
	}{
		{"always mode", "always", false, "", true},
		{"never mode", "never", true, "", false},
		{"auto with TLS", "auto", true, "", true},
		{"auto without TLS or header", "auto", false, "", false},
		{"auto with X-Forwarded-Proto https", "auto", false, "https", true},
		{"auto with X-Forwarded-Proto http", "auto", false, "http", false},
		{"empty mode defaults to auto with TLS", "", true, "", true},
		{"empty mode defaults to auto without TLS", "", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.useTLS {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.xfproto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.xfproto)
			}
			got := shouldSecure(r, tt.mode)
			if got != tt.want {
				t.Fatalf("shouldSecure(mode=%q, tls=%v, xfp=%q) = %v, want %v",
					tt.mode, tt.useTLS, tt.xfproto, got, tt.want)
			}
		})
	}
}

func TestSessionCookie(t *testing.T) {
	token := "session-token-value"

	t.Run("set and read back", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.TLS = &tls.ConnectionState{}

		SetSessionCookie(rec, r, token, DefaultSessionMaxAge, "always")

		resp := rec.Result()
		cookies := resp.Cookies()
		if len(cookies) == 0 {
			t.Fatal("no cookies set")
		}

		var found *http.Cookie
		for _, c := range cookies {
			if c.Name == sessionCookieName {
				found = c
				break
			}
		}
		if found == nil {
			t.Fatal("session cookie not found")
		}
		if !found.HttpOnly {
			t.Error("cookie should be HttpOnly")
		}
		if !found.Secure {
			t.Error("cookie should be Secure (mode=always)")
		}
		if found.Value != token {
			t.Errorf("cookie value = %q, want %q", found.Value, token)
		}

		// Read it back via GetSessionToken.
		r2 := httptest.NewRequest("GET", "/", nil)
		r2.AddCookie(found)
		got := GetSessionToken(r2)
		if got != token {
			t.Errorf("GetSessionToken = %q, want %q", got, token)
		}
	})

	t.Run("no cookie returns empty", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/", nil)
		if got := GetSessionToken(r); got != "" {
			t.Errorf("GetSessionToken with no cookie = %q, want empty", got)
		}
	})
}
