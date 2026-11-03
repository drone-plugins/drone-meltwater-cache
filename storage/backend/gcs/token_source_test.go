package gcs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meltwater/drone-cache/test"

	gcstorage "cloud.google.com/go/storage"
	"github.com/go-kit/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jws"
)

const googleClockSkewDescription = "Invalid JWT: Token must be a short-lived token (60 minutes) and in a reasonable timeframe. " +
	"Check your iat and exp values in the JWT claim."

func TestServiceAccountCredentialsCorrectsClockSkew(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// serverClockOffset is how far Google's clock is from the local clock.
		serverClockOffset time.Duration
		wantRequests      int
	}{
		{name: "clocks in sync", serverClockOffset: 0, wantRequests: 1},
		{name: "local clock ahead", serverClockOffset: -2 * time.Hour, wantRequests: 2},
		{name: "local clock behind", serverClockOffset: 2 * time.Hour, wantRequests: 2},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newFakeTokenServer(t, tc.serverClockOffset)

			creds, err := credentials(log.NewNopLogger(), Config{JSONKey: string(srv.jsonKey), Timeout: 5 * time.Second})
			test.Ok(t, err)
			test.Equals(t, "test-project", creds.ProjectID)

			tok, err := creds.TokenSource.Token()
			test.Ok(t, err)
			test.Equals(t, fmt.Sprintf("token-%d", tc.wantRequests), tok.AccessToken)
			test.Equals(t, tc.wantRequests, srv.requestCount())

			iat := time.Unix(srv.lastIat(), 0)
			serverNow := time.Now().Add(tc.serverClockOffset)
			test.Assert(t, iat.Before(serverNow) && iat.After(serverNow.Add(-time.Minute)),
				"accepted iat %s should be just before server time %s", iat, serverNow)
		})
	}
}

func TestClockSkewTokenSourceReusesMeasuredSkew(t *testing.T) {
	t.Parallel()

	srv := newFakeTokenServer(t, -2*time.Hour)
	ts := srv.tokenSource(t)

	_, err := ts.Token()
	test.Ok(t, err)
	test.Equals(t, 2, srv.requestCount())

	_, err = ts.Token()
	test.Ok(t, err)
	test.Equals(t, 3, srv.requestCount())
}

func TestClockSkewTokenSourceDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()

	srv := newFakeTokenServer(t, 0)
	srv.rejectWith = "Invalid JWT Signature."
	ts := srv.tokenSource(t)

	_, err := ts.Token()
	test.NotOk(t, err)
	test.Equals(t, 1, srv.requestCount())

	var retrieveErr *oauth2.RetrieveError
	test.Assert(t, errors.As(err, &retrieveErr), "expected oauth2.RetrieveError, got %T", err)
	test.Equals(t, "invalid_grant", retrieveErr.ErrorCode)
}

func TestClockSkewTokenSourceRetriesWithoutServerTime(t *testing.T) {
	t.Parallel()

	srv := newFakeTokenServer(t, -2*time.Hour)
	srv.omitDate = true
	ts := srv.tokenSource(t)

	_, err := ts.Token()
	test.NotOk(t, err)
	test.Equals(t, maxTokenAttempts, srv.requestCount())
	test.Assert(t, strings.Contains(err.Error(), "skew vs google unknown, check the machine clock"), "unexpected error: %v", err)
	test.Assert(t, strings.Contains(err.Error(), "reasonable timeframe"), "google error should be preserved: %v", err)
}

func TestCredentialsFallsBackForNonServiceAccountKeys(t *testing.T) {
	t.Parallel()

	jsonKey := `{"type":"authorized_user","client_id":"id","client_secret":"secret","refresh_token":"refresh"}`

	_, err := serviceAccountCredentials(log.NewNopLogger(), []byte(jsonKey), time.Second, gcstorage.ScopeFullControl)
	test.NotOk(t, err)

	creds, err := credentials(log.NewNopLogger(), Config{JSONKey: jsonKey, Timeout: time.Second})
	test.Ok(t, err)
	test.Assert(t, creds.TokenSource != nil, "expected a token source from the default credentials path")
}

// fakeTokenServer mimics Google's JWT bearer token endpoint with its own clock.
type fakeTokenServer struct {
	jsonKey     []byte
	pub         *rsa.PublicKey
	clockOffset time.Duration
	omitDate    bool
	rejectWith  string

	mu       sync.Mutex
	requests int
	iat      int64
}

func newFakeTokenServer(t *testing.T, clockOffset time.Duration) *fakeTokenServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	test.Ok(t, err)

	f := &fakeTokenServer{pub: &key.PublicKey, clockOffset: clockOffset}

	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	test.Ok(t, err)

	f.jsonKey, err = json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "test-key-id",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email":   "cache@test-project.iam.gserviceaccount.com",
		"token_uri":      srv.URL + "/token",
	})
	test.Ok(t, err)

	return f
}

func (f *fakeTokenServer) tokenSource(t *testing.T) *clockSkewTokenSource {
	t.Helper()

	ts, err := newClockSkewTokenSource(log.NewNopLogger(), f.jsonKey, 5*time.Second, gcstorage.ScopeFullControl)
	test.Ok(t, err)
	ts.backoff = time.Millisecond

	return ts
}

func (f *fakeTokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests++
	now := time.Now().Add(f.clockOffset)

	if f.omitDate {
		w.Header()["Date"] = nil
	} else {
		w.Header().Set("Date", now.UTC().Format(http.TimeFormat))
	}

	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != jwtBearerGrantType {
		writeTokenError(w, "unsupported_grant_type", "bad grant type")
		return
	}

	assertion := r.PostForm.Get("assertion")
	if err := jws.Verify(assertion, f.pub); err != nil {
		writeTokenError(w, "invalid_grant", "Invalid JWT Signature.")
		return
	}

	claims, err := jws.Decode(assertion)
	if err != nil {
		writeTokenError(w, "invalid_grant", "Invalid JWT.")
		return
	}

	f.iat = claims.Iat

	if f.rejectWith != "" {
		writeTokenError(w, "invalid_grant", f.rejectWith)
		return
	}

	if claims.Iat > now.Add(time.Minute).Unix() || claims.Exp < now.Unix() || claims.Exp-claims.Iat > int64(time.Hour/time.Second) {
		writeTokenError(w, "invalid_grant", googleClockSkewDescription)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":3600}`, f.requests)
}

func (f *fakeTokenServer) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.requests
}

func (f *fakeTokenServer) lastIat() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.iat
}

func writeTokenError(w http.ResponseWriter, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
}
