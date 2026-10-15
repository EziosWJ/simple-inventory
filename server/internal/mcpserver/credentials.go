package mcpserver

import (
	"errors"
	"net/http"
	"strings"
)

// Clients send the original credential value in a `token` header; the original
// `Authorization: Bearer <value>` form keeps working. Header names are
// case-insensitive. Sending both is accepted only while they carry the same
// value: an ambiguous pair, a repeated header or a malformed one is refused
// instead of silently choosing one of them.
var (
	errCredentialMissing    = errors.New("credential required")
	errCredentialMalformed  = errors.New("malformed credential header")
	errCredentialConflict   = errors.New("conflicting credential headers")
	errCredentialDuplicated = errors.New("repeated credential header")
)

func singleValue(values []string, count *int) (string, bool) {
	if len(values) == 0 {
		return "", true
	}
	if len(values) > 1 {
		*count++
		return "", false
	}
	return values[0], true
}

// parseCredential resolves the single credential value of one request.
func parseCredential(header http.Header) (string, error) {
	duplicated := 0
	tokenValue, tokensOK := singleValue(header.Values("Token"), &duplicated)
	authValue, authsOK := singleValue(header.Values("Authorization"), &duplicated)
	if !tokensOK || !authsOK {
		return "", errCredentialDuplicated
	}
	if tokenValue == "" && authValue == "" {
		return "", errCredentialMissing
	}
	var fromToken, fromBearer string
	if tokenValue != "" {
		fromToken = strings.TrimSpace(tokenValue)
		if fromToken == "" || strings.ContainsAny(fromToken, " \t") {
			return "", errCredentialMalformed
		}
	}
	if authValue != "" {
		scheme, value, found := strings.Cut(strings.TrimSpace(authValue), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") {
			return "", errCredentialMalformed
		}
		fromBearer = strings.TrimSpace(value)
		if fromBearer == "" || strings.ContainsAny(fromBearer, " \t") {
			return "", errCredentialMalformed
		}
	}
	if fromToken != "" && fromBearer != "" {
		if fromToken != fromBearer {
			return "", errCredentialConflict
		}
		return fromToken, nil
	}
	if fromToken != "" {
		return fromToken, nil
	}
	return fromBearer, nil
}
