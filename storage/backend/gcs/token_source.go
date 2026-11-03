package gcs

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/meltwater/drone-cache/internal"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jws"
	"golang.org/x/oauth2/jwt"
)

const (
	jwtBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	serviceAccountType = "service_account"
	googleUniverse     = "googleapis.com"

	// Google rejects assertions that live longer than an hour.
	assertionLifetime = time.Hour
	// Same backdating golang.org/x/oauth2/jws applies to tolerate small drift.
	assertionBackdate = 10 * time.Second

	maxTokenAttempts         = 3
	defaultTokenRetryBackoff = time.Second
	maxTokenResponseBytes    = 1 << 20
)

// clockSkewTokenSource exchanges a service account JWT assertion for an access token.
// Unlike jwt.Config it signs iat/exp itself, so when Google rejects the assertion
// because the local clock is off, it can re-sign against Google's clock and retry.
type clockSkewTokenSource struct {
	logger  log.Logger
	conf    *jwt.Config
	key     *rsa.PrivateKey
	client  *http.Client
	backoff time.Duration

	mu sync.Mutex
	// skew is how far the local clock is ahead of Google's (negative when behind).
	skew time.Duration
}

// serviceAccountCredentials builds credentials for a service account JSON key backed by
// clockSkewTokenSource. Other key types return an error so callers can fall back.
func serviceAccountCredentials(l log.Logger, jsonKey []byte, timeout time.Duration, scopes ...string) (*google.Credentials, error) {
	var f struct {
		Type           string `json:"type"`
		ProjectID      string `json:"project_id"`
		UniverseDomain string `json:"universe_domain"`
	}
	if err := json.Unmarshal(jsonKey, &f); err != nil {
		return nil, fmt.Errorf("parse json key, %w", err)
	}

	if f.Type != serviceAccountType {
		return nil, fmt.Errorf("json key type %q is not %q", f.Type, serviceAccountType)
	}

	// Other universes authenticate with self-signed JWTs instead of a token exchange.
	if f.UniverseDomain != "" && f.UniverseDomain != googleUniverse {
		return nil, fmt.Errorf("json key universe domain %q is not supported", f.UniverseDomain)
	}

	ts, err := newClockSkewTokenSource(l, jsonKey, timeout, scopes...)
	if err != nil {
		return nil, err
	}

	return &google.Credentials{
		ProjectID:   f.ProjectID,
		TokenSource: oauth2.ReuseTokenSource(nil, ts),
		JSON:        jsonKey,
	}, nil
}

func newClockSkewTokenSource(l log.Logger, jsonKey []byte, timeout time.Duration, scopes ...string) (*clockSkewTokenSource, error) {
	conf, err := google.JWTConfigFromJSON(jsonKey, scopes...)
	if err != nil {
		return nil, err
	}

	key, err := parseRSAKey(conf.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &clockSkewTokenSource{
		logger:  l,
		conf:    conf,
		key:     key,
		client:  &http.Client{Timeout: timeout},
		backoff: defaultTokenRetryBackoff,
	}, nil
}

// Token implements oauth2.TokenSource.
func (s *clockSkewTokenSource) Token() (*oauth2.Token, error) {
	var err error

	for attempt := 1; attempt <= maxTokenAttempts; attempt++ {
		var (
			tok        *oauth2.Token
			serverTime time.Time
		)

		tok, serverTime, err = s.exchange()
		if err == nil {
			return tok, nil
		}

		if !isClockSkewRejection(err) || attempt == maxTokenAttempts {
			break
		}

		if serverTime.IsZero() {
			level.Warn(s.logger).Log("msg", "gcs token rejected for iat/exp and no server time to correct with, retrying", "attempt", attempt)
			time.Sleep(s.backoff * time.Duration(attempt))

			continue
		}

		skew := time.Since(serverTime)
		s.setSkew(skew)
		level.Warn(s.logger).Log(
			"msg", "gcs token rejected because local clock is skewed, re-signing with google server time",
			"local_clock_skew", skew.Round(time.Second).String(),
			"attempt", attempt,
		)
	}

	if isClockSkewRejection(err) {
		skew := "unknown"
		if d := s.getSkew(); d != 0 {
			skew = d.Round(time.Second).String()
		}

		// Not %w: the storage client converts oauth2.RetrieveError into its own error
		// type and would drop this hint.
		return nil, fmt.Errorf("gcs token rejected for iat/exp after %d attempts, local clock skew vs google %s, check the machine clock (NTP): %v",
			maxTokenAttempts, skew, err)
	}

	return nil, err
}

// exchange posts a freshly signed assertion to the token endpoint. On failure it also
// returns the server time from the response Date header when available.
func (s *clockSkewTokenSource) exchange() (*oauth2.Token, time.Time, error) {
	iat := time.Now().Add(-s.getSkew()).Add(-assertionBackdate)
	claims := &jws.ClaimSet{
		Iss:   s.conf.Email,
		Scope: strings.Join(s.conf.Scopes, " "),
		Aud:   s.conf.TokenURL,
		Iat:   iat.Unix(),
		Exp:   iat.Add(assertionLifetime).Unix(),
	}

	if s.conf.Audience != "" {
		claims.Aud = s.conf.Audience
	}

	header := &jws.Header{Algorithm: "RS256", Typ: "JWT", KeyID: s.conf.PrivateKeyID}

	assertion, err := jws.Encode(header, claims, s.key)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("sign jwt assertion, %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", jwtBearerGrantType)
	form.Set("assertion", assertion)

	resp, err := s.client.PostForm(s.conf.TokenURL, form)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("oauth2: cannot fetch token: %w", err)
	}
	defer internal.CloseWithErrLogf(s.logger, resp.Body, "token response body, close defer")

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("oauth2: cannot fetch token: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var serverTime time.Time
		if t, perr := http.ParseTime(resp.Header.Get("Date")); perr == nil {
			serverTime = t
		}

		return nil, serverTime, newRetrieveError(resp, body)
	}

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, time.Time{}, fmt.Errorf("oauth2: cannot fetch token: %w", err)
	}

	if tokenRes.AccessToken == "" {
		return nil, time.Time{}, errors.New("oauth2: server response missing access_token")
	}

	tok := &oauth2.Token{AccessToken: tokenRes.AccessToken, TokenType: tokenRes.TokenType}
	if tokenRes.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)
	}

	return tok, time.Time{}, nil
}

func (s *clockSkewTokenSource) getSkew() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.skew
}

func (s *clockSkewTokenSource) setSkew(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.skew = d
}

func newRetrieveError(resp *http.Response, body []byte) *oauth2.RetrieveError {
	retrieveErr := &oauth2.RetrieveError{Response: resp, Body: body}

	var errBody struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &errBody) == nil {
		retrieveErr.ErrorCode = errBody.Error
		retrieveErr.ErrorDescription = errBody.Description
	}

	return retrieveErr
}

// isClockSkewRejection reports whether Google rejected the assertion's iat/exp, e.g.
// "Invalid JWT: Token must be a short-lived token (60 minutes) and in a reasonable timeframe."
func isClockSkewRejection(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) || retrieveErr.ErrorCode != "invalid_grant" {
		return false
	}

	desc := strings.ToLower(retrieveErr.ErrorDescription)

	return strings.Contains(desc, "reasonable timeframe") || strings.Contains(desc, "iat and exp")
}

func parseRSAKey(key []byte) (*rsa.PrivateKey, error) {
	if block, _ := pem.Decode(key); block != nil {
		key = block.Bytes
	}

	parsed, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsed, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("private key should be a PEM or plain PKCS1 or PKCS8, %w", err)
		}
	}

	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not an RSA key")
	}

	return rsaKey, nil
}
