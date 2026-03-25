package app

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	googleJWKSURL  = "https://www.googleapis.com/oauth2/v3/certs"
)

type googleIdentity struct {
	Subject string
	Email   string
	Name    string
	Picture string
}

type googleOAuthProvider interface {
	Configured() bool
	AuthCodeURL(state string) string
	ExchangeCode(ctx context.Context, code string) (googleIdentity, error)
}

type disabledGoogleOAuthProvider struct{}

func (disabledGoogleOAuthProvider) Configured() bool {
	return false
}

func (disabledGoogleOAuthProvider) AuthCodeURL(state string) string {
	return ""
}

func (disabledGoogleOAuthProvider) ExchangeCode(ctx context.Context, code string) (googleIdentity, error) {
	return googleIdentity{}, models.ErrAuthNotConfigured
}

type liveGoogleOAuthProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client
}

func newGoogleOAuthProvider(cfg config.Config) googleOAuthProvider {
	if cfg.GoogleClientID == "" || cfg.GoogleSecret == "" || cfg.GoogleRedirect == "" {
		return disabledGoogleOAuthProvider{}
	}

	return &liveGoogleOAuthProvider{
		clientID:     cfg.GoogleClientID,
		clientSecret: cfg.GoogleSecret,
		redirectURL:  cfg.GoogleRedirect,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *liveGoogleOAuthProvider) Configured() bool {
	return p != nil && p.clientID != "" && p.clientSecret != "" && p.redirectURL != ""
}

func (p *liveGoogleOAuthProvider) AuthCodeURL(state string) string {
	values := url.Values{
		"client_id":              {p.clientID},
		"redirect_uri":           {p.redirectURL},
		"response_type":          {"code"},
		"scope":                  {"openid email profile"},
		"state":                  {state},
		"access_type":            {"online"},
		"include_granted_scopes": {"true"},
	}
	return googleAuthURL + "?" + values.Encode()
}

func (p *liveGoogleOAuthProvider) ExchangeCode(ctx context.Context, code string) (googleIdentity, error) {
	form := url.Values{
		"code":          {strings.TrimSpace(code)},
		"client_id":     {p.clientID},
		"client_secret": {p.clientSecret},
		"redirect_uri":  {p.redirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleTokenExchangeFailed,
			fmt.Errorf("build google token request: %w", err),
		)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleTokenExchangeFailed,
			fmt.Errorf("exchange google auth code: %w", err),
		)
	}
	defer resp.Body.Close()

	var tokenResponse struct {
		IDToken          string `json:"id_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleTokenExchangeFailed,
			fmt.Errorf("decode google token response: %w", err),
		)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleTokenExchangeFailed,
			fmt.Errorf("exchange google auth code: %s", strings.TrimSpace(tokenResponse.ErrorDescription+" "+tokenResponse.Error)),
		)
	}
	if tokenResponse.IDToken == "" {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenMissing,
			fmt.Errorf("google response did not include id_token"),
		)
	}

	return p.verifyIDToken(ctx, tokenResponse.IDToken)
}

func (p *liveGoogleOAuthProvider) verifyIDToken(ctx context.Context, rawToken string) (googleIdentity, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("invalid google id token format"),
		)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("decode google id token header: %w", err),
		)
	}
	if header.Alg != "RS256" {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("unexpected google id token algorithm %q", header.Alg),
		)
	}

	claims := make(map[string]interface{})
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("decode google id token claims: %w", err),
		)
	}

	if err := p.validateClaims(claims); err != nil {
		return googleIdentity{}, models.NewAuthFailureError(models.AuthFailureReasonGoogleIDTokenInvalid, err)
	}

	publicKey, err := p.fetchGooglePublicKey(ctx, header.Kid)
	if err != nil {
		return googleIdentity{}, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("decode google id token signature: %w", err),
		)
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature); err != nil {
		return googleIdentity{}, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleIDTokenInvalid,
			fmt.Errorf("verify google id token signature: %w", err),
		)
	}

	return googleIdentity{
		Subject: claimString(claims, "sub"),
		Email:   claimString(claims, "email"),
		Name:    claimString(claims, "name"),
		Picture: claimString(claims, "picture"),
	}, nil
}

func (p *liveGoogleOAuthProvider) validateClaims(claims map[string]interface{}) error {
	issuer := claimString(claims, "iss")
	if issuer != "accounts.google.com" && issuer != "https://accounts.google.com" {
		return fmt.Errorf("unexpected google id token issuer %q", issuer)
	}
	if !claimAudienceMatches(claims["aud"], p.clientID) {
		return fmt.Errorf("google id token audience mismatch")
	}

	subject := claimString(claims, "sub")
	if subject == "" {
		return fmt.Errorf("google id token subject is missing")
	}

	if expiresAt := claimUnix(claims, "exp"); expiresAt <= time.Now().Unix() {
		return fmt.Errorf("google id token is expired")
	}

	return nil
}

func (p *liveGoogleOAuthProvider) fetchGooglePublicKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleJWKSURL, nil)
	if err != nil {
		return nil, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleJWKSFetchFailed,
			fmt.Errorf("build google jwks request: %w", err),
		)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleJWKSFetchFailed,
			fmt.Errorf("fetch google jwks: %w", err),
		)
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string   `json:"kid"`
			Kty string   `json:"kty"`
			Alg string   `json:"alg"`
			N   string   `json:"n"`
			E   string   `json:"e"`
			X5c []string `json:"x5c"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleJWKSFetchFailed,
			fmt.Errorf("decode google jwks: %w", err),
		)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, models.NewAuthFailureError(
			models.AuthFailureReasonGoogleJWKSFetchFailed,
			fmt.Errorf("fetch google jwks: unexpected status %d", resp.StatusCode),
		)
	}

	for _, key := range jwks.Keys {
		if key.Kid != keyID {
			continue
		}

		if len(key.X5c) > 0 {
			pub, err := parseCertificateKey(key.X5c[0])
			if err == nil {
				return pub, nil
			}
			return nil, models.NewAuthFailureError(
				models.AuthFailureReasonGoogleJWKSFetchFailed,
				fmt.Errorf("parse google x5c certificate: %w", err),
			)
		}

		pub, err := parseJWKKey(key.N, key.E)
		if err != nil {
			return nil, models.NewAuthFailureError(
				models.AuthFailureReasonGoogleJWKSFetchFailed,
				fmt.Errorf("parse google jwk key: %w", err),
			)
		}
		return pub, nil
	}

	return nil, models.NewAuthFailureError(
		models.AuthFailureReasonGoogleJWKSFetchFailed,
		fmt.Errorf("google public key %q not found", keyID),
	)
}

func decodeJWTPart(part string, target interface{}) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func claimString(claims map[string]interface{}, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func claimUnix(claims map[string]interface{}, key string) int64 {
	switch value := claims[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}

func claimAudienceMatches(raw interface{}, clientID string) bool {
	switch value := raw.(type) {
	case string:
		return value == clientID
	case []interface{}:
		for _, item := range value {
			if itemString, ok := item.(string); ok && itemString == clientID {
				return true
			}
		}
	}
	return false
}

func parseCertificateKey(rawCert string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(rawCert)
	if err != nil {
		return nil, fmt.Errorf("decode google x5c certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse google x5c certificate: %w", err)
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("google certificate did not contain an RSA public key")
	}
	return publicKey, nil
}

func parseJWKKey(modulus, exponent string) (*rsa.PublicKey, error) {
	modulusBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, fmt.Errorf("decode jwk modulus: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, fmt.Errorf("decode jwk exponent: %w", err)
	}

	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulusBytes),
		E: 0,
	}
	for _, b := range exponentBytes {
		pub.E = pub.E<<8 | int(b)
	}
	if pub.E == 0 {
		return nil, fmt.Errorf("invalid jwk exponent")
	}
	return pub, nil
}
