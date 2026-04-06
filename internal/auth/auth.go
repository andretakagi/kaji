// Password hashing (bcrypt), API keys (SHA-256), signed session cookies.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const DefaultSessionMaxAge = 24 * 60 * 60 // 24 hours in seconds

const sessionCookieName = "kaji_session"

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateAPIKey() (string, error) {
	return GenerateSessionToken()
}

func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func CheckAPIKey(provided, storedHash string) bool {
	h := sha256.Sum256([]byte(provided))
	providedHash := hex.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(providedHash), []byte(storedHash)) == 1
}

func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// shouldSecure decides whether the Secure flag should be set on cookies.
// mode is the secure_cookies config value: "always", "never", or "auto" (default).
// In auto mode, it checks r.TLS and the X-Forwarded-Proto header.
func shouldSecure(r *http.Request, mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		if r.TLS != nil {
			return true
		}
		return r.Header.Get("X-Forwarded-Proto") == "https"
	}
}

func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int, secureCookies string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   shouldSecure(r, secureCookies),
		SameSite: http.SameSiteStrictMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, r *http.Request, secureCookies string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   shouldSecure(r, secureCookies),
		SameSite: http.SameSiteStrictMode,
	})
}

func GetSessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// SignToken produces a signed token string: "token.issuedAt.hmac-signature".
// The HMAC covers both the token and the issued-at timestamp.
func SignToken(token, secret string) string {
	issuedAt := strconv.FormatInt(time.Now().Unix(), 10)
	payload := token + "." + issuedAt
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s.%s", token, issuedAt, sig)
}

// ValidateSignedToken checks that a "token.issuedAt.signature" string has a
// valid HMAC and that the token was issued within maxAge seconds. Pass 0 for
// maxAge to use the default (24 hours).
func ValidateSignedToken(signed, secret string, maxAge int) bool {
	parts := strings.SplitN(signed, ".", 3)

	// Accept legacy two-part tokens during migration, but reject them since
	// they have no expiry.
	if len(parts) != 3 {
		return false
	}

	payload := parts[0] + "." + parts[1]
	expected := hmac.New(sha256.New, []byte(secret))
	expected.Write([]byte(payload))
	sig, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	if !hmac.Equal(sig, expected.Sum(nil)) {
		return false
	}

	issuedAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}

	if maxAge <= 0 {
		maxAge = DefaultSessionMaxAge
	}
	age := time.Now().Unix() - issuedAt
	return age >= 0 && age < int64(maxAge)
}
