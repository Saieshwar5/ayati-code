package webapp

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// requireAccessPassword guards the whole application with HTTP Basic
// authentication. Any non-empty username is accepted; only the password must
// match the configured secret. It is a personal single-user gate for remote
// deployments and complements (not replaces) HTTPS and the GitHub OAuth flow.
func requireAccessPassword(password string, next http.Handler) http.Handler {
	expected := []byte(password)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		supplied, ok := basicPassword(request.Header.Get("Authorization"))
		if !ok || subtle.ConstantTimeCompare([]byte(supplied), expected) != 1 {
			writer.Header().Set("WWW-Authenticate", `Basic realm="perpetual", charset="UTF-8"`)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(writer).Encode(map[string]string{"error": "authentication required"})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// basicPassword extracts the password from an HTTP Basic Authorization header.
// The username is deliberately ignored so a personal browser prompt can use
// any value (for example the GitHub login).
func basicPassword(header string) (string, bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", false
	}
	_, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", false
	}
	return password, true
}
