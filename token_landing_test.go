package main

// Unit and httptest-level coverage for `sigil token --landing <owner>/<repo>`
// (spec docs/specs/lead-session-gh-token.md, R-1 to R-5). GitHub is always an
// httptest mock and keys are generated in process; none of this needs network
// or 1Password.
//
// Every expectation below is derived from the spec's wording, not from what
// the implementation returns. In particular the contract body and permission
// map are written out here as literals, independent of any constant in
// token_landing.go. Fixture tokens are built at runtime and are not
// ghs_-shaped, so no scanner entry is needed (spec "Constraints", fixture tokens).

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
)

// --- fixtures ---

// contractPermissions is the API Contract's permission map, written out
// independently of the implementation.
func contractPermissions() map[string]string {
	return map[string]string{
		"contents":      "write",
		"pull_requests": "write",
		"checks":        "read",
		"workflows":     "write",
		"metadata":      "read",
	}
}

var contractKeys = []string{"contents", "pull_requests", "checks", "workflows", "metadata"}

// runtimeToken builds a fixture token that is unique per call and not
// ghs_-shaped.
func runtimeToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 12)
	if _, err := cryptorand.Read(b); err != nil {
		t.Fatalf("crypto/rand: %v", err)
	}
	return "fixture-" + hex.EncodeToString(b)
}

// landingResponse is a GitHub access_tokens response that equals the contract
// for repo.
func landingResponse(token, repo string, private bool) map[string]any {
	return map[string]any{
		"token":                token,
		"permissions":          contractPermissions(),
		"repository_selection": "selected",
		"repositories":         []map[string]any{{"name": repo, "private": private}},
	}
}

func decodeLandingResponse(t *testing.T, m map[string]any) *narrowedTokenResponse {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var out narrowedTokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return &out
}

// landingMock records what the landing path sends and answers as scripted.
type landingMock struct {
	srv *httptest.Server

	mu           sync.Mutex
	mintStatuses []int // per mint attempt; the last entry repeats
	body         any
	lastBody     string // the response body as sent, for the leak-scan control
	revokeStatus int
	revokeDrop   bool // close the connection instead of answering the revoke
	// hold401 makes a 401 wait until the wall-clock second has advanced, so a
	// JWT minted for the retry differs from the first (mintAppJWT's claims are
	// in Unix seconds and RS256 here is deterministic). Costs up to ~1 s.
	hold401 bool

	mintAuth    []string
	mintBodies  [][]byte
	mintMethods []string
	mintPaths   []string
	deleteAuth  []string
}

func newLandingMock(t *testing.T, body any) *landingMock {
	t.Helper()
	m := &landingMock{mintStatuses: []int{http.StatusCreated}, body: body, revokeStatus: http.StatusNoContent}
	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations/", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		defer m.mu.Unlock()
		attempt := len(m.mintAuth)
		m.mintAuth = append(m.mintAuth, r.Header.Get("Authorization"))
		m.mintBodies = append(m.mintBodies, raw)
		m.mintMethods = append(m.mintMethods, r.Method)
		m.mintPaths = append(m.mintPaths, r.URL.Path)
		status := m.mintStatuses[len(m.mintStatuses)-1]
		if attempt < len(m.mintStatuses) {
			status = m.mintStatuses[attempt]
		}
		if status == http.StatusUnauthorized && m.hold401 {
			entry := time.Now().Unix()
			for time.Now().Unix() <= entry {
				time.Sleep(10 * time.Millisecond)
			}
		}
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(m.body)
		m.lastBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(buf.Bytes())
	})
	mux.HandleFunc("/installation/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE on /installation/token, got %s", r.Method)
		}
		m.mu.Lock()
		m.deleteAuth = append(m.deleteAuth, r.Header.Get("Authorization"))
		drop, status := m.revokeDrop, m.revokeStatus
		m.mu.Unlock()
		if drop {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("response writer cannot hijack")
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		w.WriteHeader(status)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// script replaces the response body and clears everything recorded.
func (m *landingMock) script(body any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.body = body
	m.mintAuth, m.mintBodies, m.mintMethods, m.mintPaths, m.deleteAuth = nil, nil, nil, nil, nil
}

func (m *landingMock) deletes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.deleteAuth...)
}

func (m *landingMock) mintCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.mintAuth)
}

const landingInstallID = int64(12345)

// --- perturbations: one field of a matching response, changed ---

type perturbation struct {
	name  string
	apply func(r *rand.Rand, m map[string]any, repo string)
}

func randHex(r *rand.Rand) string { return fmt.Sprintf("%08x", r.Uint32()) }

func perms(m map[string]any) map[string]string { return m["permissions"].(map[string]string) }

func swapCase(s string) string {
	return strings.Map(func(c rune) rune {
		if unicode.IsUpper(c) {
			return unicode.ToLower(c)
		}
		return unicode.ToUpper(c)
	}, s)
}

func landingPerturbations() []perturbation {
	setRepos := func(m map[string]any, names ...string) {
		repos := make([]map[string]any, 0, len(names))
		for _, n := range names {
			repos = append(repos, map[string]any{"name": n, "private": false})
		}
		m["repositories"] = repos
	}
	return []perturbation{
		{"extra permission key", func(r *rand.Rand, m map[string]any, repo string) {
			perms(m)["extra-"+randHex(r)] = "lvl-" + randHex(r)
		}},
		{"missing permission key", func(r *rand.Rand, m map[string]any, repo string) {
			delete(perms(m), contractKeys[r.IntN(len(contractKeys))])
		}},
		{"changed level (distinct string)", func(r *rand.Rand, m map[string]any, repo string) {
			perms(m)[contractKeys[r.IntN(len(contractKeys))]] = "lvl-" + randHex(r)
		}},
		{"level flipped read<->write", func(r *rand.Rand, m map[string]any, repo string) {
			k := contractKeys[r.IntN(len(contractKeys))]
			if perms(m)[k] == "read" {
				perms(m)[k] = "write"
			} else {
				perms(m)[k] = "read"
			}
		}},
		{"permissions missing", func(r *rand.Rand, m map[string]any, repo string) {
			delete(m, "permissions")
		}},
		{"repository_selection not selected", func(r *rand.Rand, m map[string]any, repo string) {
			opts := []string{"all", "", "Selected", "selected ", "none"}
			m["repository_selection"] = opts[r.IntN(len(opts))]
		}},
		{"repository_selection missing", func(r *rand.Rand, m map[string]any, repo string) {
			delete(m, "repository_selection")
		}},
		{"repository count 0", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m) }},
		{"repository count 2", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, repo, "other-"+randHex(r)) }},
		{"repository count 3", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, repo, repo, repo) }},
		{"repositories missing", func(r *rand.Rand, m map[string]any, repo string) { delete(m, "repositories") }},
		{"name differs only in letter case", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, swapCase(repo)) }},
		{"name has a suffix", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, repo+"x") }},
		{"name is owner/repo", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, "test-owner/"+repo) }},
		{"name empty", func(r *rand.Rand, m map[string]any, repo string) { setRepos(m, "") }},
	}
}

// mixedCaseRepo makes the letter-case perturbation a real difference.
const mixedCaseRepo = "MixedCase-Repo.v2"

// --- R-4: the verifier ---

// TestVerifyLandingGrant_MatchingResponseAccepted: R-4, a matching response is
// accepted for a public repository and for a private one (no public-repository
// condition).
func TestVerifyLandingGrant_MatchingResponseAccepted(t *testing.T) {
	for _, private := range []bool{false, true} {
		resp := decodeLandingResponse(t, landingResponse("unused", mixedCaseRepo, private))
		if err := verifyLandingGrant(resp, mixedCaseRepo); err != nil {
			t.Errorf("private=%v: matching grant rejected: %v", private, err)
		}
	}
}

// TestVerifyLandingGrant_AnySingleFieldPerturbationRejected: R-4 as a
// property. Starting from a matching response, any single-field perturbation
// is rejected, and the unperturbed control is accepted every time, so the
// perturbation is the only cause of the rejection.
func TestVerifyLandingGrant_AnySingleFieldPerturbationRejected(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	perts := landingPerturbations()
	for i := 0; i < 400; i++ {
		p := perts[i%len(perts)]
		private := r.IntN(2) == 0

		control := decodeLandingResponse(t, landingResponse("unused", mixedCaseRepo, private))
		if err := verifyLandingGrant(control, mixedCaseRepo); err != nil {
			t.Fatalf("iteration %d: control (unperturbed) rejected: %v", i, err)
		}

		m := landingResponse("unused", mixedCaseRepo, private)
		p.apply(r, m, mixedCaseRepo)
		err := verifyLandingGrant(decodeLandingResponse(t, m), mixedCaseRepo)
		if err == nil {
			t.Fatalf("iteration %d: perturbation %q accepted: %v", i, p.name, m)
		}
		if !errors.Is(err, errLandingGrantMismatch) {
			t.Errorf("iteration %d: perturbation %q: error %v does not wrap errLandingGrantMismatch", i, p.name, err)
		}
		if strings.Contains(err.Error(), "lvl-") {
			t.Errorf("iteration %d: perturbation %q: error carries a granted permission level: %v", i, p.name, err)
		}
	}
}

// --- R-3: the request body ---

// TestGetLandingToken_RequestBodyEqualsContract: R-3, the decoded body equals
// the API Contract body for the named repository, field for field, with no
// other field. The only caller input is the repository name.
func TestGetLandingToken_RequestBodyEqualsContract(t *testing.T) {
	const repo = "some-repo"
	token := runtimeToken(t)
	mock := newLandingMock(t, landingResponse(token, repo, false))

	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", repo, mock.srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != token {
		t.Errorf("token: got %q, want %q", got, token)
	}

	if mock.mintCount() != 1 {
		t.Fatalf("mint requests: got %d, want 1", mock.mintCount())
	}
	const want = `{"permissions":{"contents":"write","pull_requests":"write","checks":"read","workflows":"write","metadata":"read"},"repositories":["some-repo"]}`
	var gotDecoded, wantDecoded any
	if err := json.Unmarshal(mock.mintBodies[0], &gotDecoded); err != nil {
		t.Fatalf("decode request body %s: %v", mock.mintBodies[0], err)
	}
	if err := json.Unmarshal([]byte(want), &wantDecoded); err != nil {
		t.Fatalf("decode expected body: %v", err)
	}
	if !reflect.DeepEqual(gotDecoded, wantDecoded) {
		t.Errorf("request body: got %s, want %s", mock.mintBodies[0], want)
	}
	if mock.mintMethods[0] != http.MethodPost || mock.mintPaths[0] != "/app/installations/12345/access_tokens" {
		t.Errorf("request: got %s %s, want POST /app/installations/12345/access_tokens", mock.mintMethods[0], mock.mintPaths[0])
	}
	if !strings.HasPrefix(mock.mintAuth[0], "Bearer ") || len(mock.mintAuth[0]) <= len("Bearer ") {
		t.Errorf("mint request not authenticated with an App JWT: %q", mock.mintAuth[0])
	}
}

// --- R-4: success, mismatch, revoke ---

// TestGetLandingToken_MatchingResponseReturnsToken: R-4, public and private
// repositories each return the token, and nothing is revoked.
func TestGetLandingToken_MatchingResponseReturnsToken(t *testing.T) {
	for _, private := range []bool{false, true} {
		token := runtimeToken(t)
		mock := newLandingMock(t, landingResponse(token, "some-repo", private))
		got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
		if err != nil {
			t.Fatalf("private=%v: unexpected error: %v", private, err)
		}
		if got != token {
			t.Errorf("private=%v: token: got %q, want %q", private, got, token)
		}
		if d := mock.deletes(); len(d) != 0 {
			t.Errorf("private=%v: matching grant was revoked: %v", private, d)
		}
	}
}

// TestGetLandingToken_EachNamedDifferenceRevokesOnce: R-4 and the scenario
// "GitHub widens the grant". Each difference the spec names returns no token,
// an error containing `grant mismatch`, and exactly one DELETE carrying the
// minted token.
func TestGetLandingToken_EachNamedDifferenceRevokesOnce(t *testing.T) {
	const repo = mixedCaseRepo
	named := map[string]func(m map[string]any){
		"extra permission key": func(m map[string]any) { perms(m)["issues"] = "write" },
		"missing key":          func(m map[string]any) { delete(perms(m), "checks") },
		"changed level":        func(m map[string]any) { perms(m)["contents"] = "read" },
		"selection all":        func(m map[string]any) { m["repository_selection"] = "all" },
		"repository count 0":   func(m map[string]any) { m["repositories"] = []map[string]any{} },
		"repository count 2":   func(m map[string]any) { m["repositories"] = []map[string]any{{"name": repo}, {"name": "second"}} },
		"name differs by case": func(m map[string]any) { m["repositories"] = []map[string]any{{"name": swapCase(repo)}} },
	}
	key := newTestKey(t)
	for name, mutate := range named {
		t.Run(name, func(t *testing.T) {
			token := runtimeToken(t)
			m := landingResponse(token, repo, false)
			mutate(m)
			mock := newLandingMock(t, m)

			got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), key, "test-owner", repo, mock.srv.URL)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "grant mismatch") {
				t.Errorf("error %q does not contain %q", err, "grant mismatch")
			}
			if got != "" {
				t.Errorf("token returned on mismatch: %q", got)
			}
			d := mock.deletes()
			if len(d) != 1 || d[0] != "Bearer "+token {
				t.Errorf("DELETE /installation/token: got %v, want exactly one carrying the minted token", d)
			}
		})
	}
}

// TestGetLandingToken_AnySingleFieldPerturbationRevokesOnce: R-4 as a
// property through the mint function. Every perturbation of a matching
// response yields no token, `grant mismatch`, exactly one revoke carrying that
// response's token, and an error holding neither the token nor a granted level.
func TestGetLandingToken_AnySingleFieldPerturbationRevokesOnce(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	perts := landingPerturbations()
	key := newTestKey(t)
	cfg := newTestConfig(landingInstallID)
	mock := newLandingMock(t, nil)

	for i := 0; i < 60; i++ {
		p := perts[i%len(perts)]
		token := runtimeToken(t)
		m := landingResponse(token, mixedCaseRepo, r.IntN(2) == 0)
		p.apply(r, m, mixedCaseRepo)
		mock.script(m)

		got, err := getLandingToken(context.Background(), cfg, key, "test-owner", mixedCaseRepo, mock.srv.URL)
		if err == nil {
			t.Fatalf("iteration %d (%s): expected an error, got token %q", i, p.name, got)
		}
		if got != "" {
			t.Errorf("iteration %d (%s): token returned on mismatch", i, p.name)
		}
		if !strings.Contains(err.Error(), "grant mismatch") {
			t.Errorf("iteration %d (%s): error %q does not contain %q", i, p.name, err, "grant mismatch")
		}
		d := mock.deletes()
		if len(d) != 1 || d[0] != "Bearer "+token {
			t.Errorf("iteration %d (%s): DELETE calls %v, want exactly one carrying the minted token", i, p.name, d)
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("iteration %d (%s): error carries the token", i, p.name)
		}
		if strings.Contains(err.Error(), "lvl-") {
			t.Errorf("iteration %d (%s): error carries a granted permission level: %v", i, p.name, err)
		}
	}
}

// TestGetLandingToken_RevokeFailureReportsRevokeFailed: R-4 and the scenario
// "the revoke fails", over a non-204 status and over a transport failure.
func TestGetLandingToken_RevokeFailureReportsRevokeFailed(t *testing.T) {
	key := newTestKey(t)
	for _, tc := range []struct {
		name  string
		setup func(m *landingMock)
	}{
		{"status 500", func(m *landingMock) { m.revokeStatus = http.StatusInternalServerError }},
		{"status 200", func(m *landingMock) { m.revokeStatus = http.StatusOK }},
		{"transport failure", func(m *landingMock) { m.revokeDrop = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := runtimeToken(t)
			m := landingResponse(token, "some-repo", false)
			m["repository_selection"] = "all"
			mock := newLandingMock(t, m)
			tc.setup(mock)

			got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), key, "test-owner", "some-repo", mock.srv.URL)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "revoke failed") {
				t.Errorf("error %q does not contain %q", err, "revoke failed")
			}
			if got != "" {
				t.Errorf("token returned although the grant mismatched: %q", got)
			}
			if strings.Contains(err.Error(), token) {
				t.Errorf("error carries the token: %v", err)
			}
			if len(mock.deletes()) == 0 {
				t.Error("no revoke was attempted")
			}
		})
	}
}

// TestGetLandingToken_EmptyTokenErrorsWithoutRevoke: R-4, a 201 with an empty
// token is an error and sends no revoke, even when the grant matches.
func TestGetLandingToken_EmptyTokenErrorsWithoutRevoke(t *testing.T) {
	mock := newLandingMock(t, landingResponse("", "some-repo", false))
	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil {
		t.Fatal("expected an error for an empty token, got nil")
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if mock.mintCount() != 1 {
		t.Errorf("mint requests: got %d, want 1 (an empty token is not a 401)", mock.mintCount())
	}
	if d := mock.deletes(); len(d) != 0 {
		t.Errorf("revoke sent for an empty token: %v", d)
	}
}

// TestGetLandingToken_EmptyTokenWithMismatchedGrantIsNotARevokeCase: an empty
// token is rejected before the grant is looked at. With a mismatched grant as
// well, the result is still an error with no DELETE, and the error does not
// claim a grant mismatch (there was no live credential to revoke).
func TestGetLandingToken_EmptyTokenWithMismatchedGrantIsNotARevokeCase(t *testing.T) {
	m := landingResponse("", "some-repo", false)
	m["repository_selection"] = "all"
	mock := newLandingMock(t, m)
	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if strings.Contains(err.Error(), "grant mismatch") {
		t.Errorf("error %q reports a grant mismatch for a response with no token", err)
	}
	if d := mock.deletes(); len(d) != 0 {
		t.Errorf("revoke sent for an empty token: %v", d)
	}
}

// TestGetLandingToken_Non201IsAnErrorWithoutRevoke: the API Contract's "any
// other status exits non-zero". There is no token to revoke.
func TestGetLandingToken_Non201IsAnErrorWithoutRevoke(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusOK} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			token := runtimeToken(t)
			mock := newLandingMock(t, landingResponse(token, "some-repo", false))
			mock.mintStatuses = []int{status}
			got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
			if err == nil {
				t.Fatal("expected an error, got nil (only 201 is success)")
			}
			if got != "" {
				t.Errorf("token: got %q, want empty", got)
			}
			if mock.mintCount() != 1 {
				t.Errorf("mint requests: got %d, want exactly 1", mock.mintCount())
			}
			if strings.Contains(err.Error(), token) {
				t.Errorf("error carries the token: %v", err)
			}
			if d := mock.deletes(); len(d) != 0 {
				t.Errorf("revoke sent without a 201: %v", d)
			}
		})
	}
}

// TestGetLandingToken_201WithUndecodableBodyRevokesTheDecodedToken: a 201 whose
// body carries a token but fails to decode (a type error in another field)
// must not leave that token live: one DELETE carrying it, and an error.
func TestGetLandingToken_201WithUndecodableBodyRevokesTheDecodedToken(t *testing.T) {
	token := runtimeToken(t)
	m := landingResponse(token, "some-repo", false)
	m["repository_selection"] = 42 // type error for a string field
	mock := newLandingMock(t, m)
	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error carries the token: %v", err)
	}
	if d := mock.deletes(); len(d) != 1 || d[0] != "Bearer "+token {
		t.Errorf("DELETE calls: got %v, want exactly one carrying the decoded token", d)
	}
}

// assertValidAppJWT parses an Authorization header as an App JWT signed by the
// test key, unexpired at the moment of the check, issued for appID 1.
func assertValidAppJWT(t *testing.T, label, header string, key any) {
	t.Helper()
	raw := strings.TrimPrefix(header, "Bearer ")
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(time.Now))
	if err != nil || !tok.Valid {
		t.Errorf("%s: not a valid, unexpired RS256 App JWT: %v", label, err)
		return
	}
	if iss, _ := claims["iss"].(float64); iss != 1 {
		t.Errorf("%s: iss claim %v, want the App ID 1", label, claims["iss"])
	}
}

// TestGetLandingToken_401RetriedExactlyOnceWithValidJWT: R-4, a 401 is retried
// exactly once and each attempt presents a valid App JWT. The retried
// response still goes through the verifier.
//
// The retry's JWT must be minted anew: mintAppJWT is deterministic within a
// clock second, so the mock holds the 401 until the second advances (as
// TestGetInstallationToken_401RetryUsesFreshlyMintedJWT does) and the two
// Authorization headers then differ only if the retry re-minted.
func TestGetLandingToken_401RetriedExactlyOnceWithValidJWT(t *testing.T) {
	key := newTestKey(t)
	token := runtimeToken(t)
	mock := newLandingMock(t, landingResponse(token, "some-repo", false))
	mock.mintStatuses = []int{http.StatusUnauthorized, http.StatusCreated}
	mock.hold401 = true

	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), key, "test-owner", "some-repo", mock.srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != token {
		t.Errorf("token: got %q, want %q", got, token)
	}
	if mock.mintCount() != 2 {
		t.Fatalf("mint requests: got %d, want 2 (one 401 and one retry)", mock.mintCount())
	}
	if mock.mintAuth[0] == mock.mintAuth[1] {
		t.Error("the retry reused the first JWT; it must be minted anew")
	}
	for i, h := range mock.mintAuth {
		assertValidAppJWT(t, fmt.Sprintf("attempt %d", i+1), h, &key.PublicKey)
	}
	for i, b := range mock.mintBodies {
		if !bytes.Contains(b, []byte(`"repositories":["some-repo"]`)) {
			t.Errorf("attempt %d: body %s lost the repository", i+1, b)
		}
	}
}

// TestGetLandingToken_Persistent401StopsAfterOneRetry: "retried exactly once"
// means a second 401 is an error, not a third attempt.
func TestGetLandingToken_Persistent401StopsAfterOneRetry(t *testing.T) {
	mock := newLandingMock(t, landingResponse(runtimeToken(t), "some-repo", false))
	mock.mintStatuses = []int{http.StatusUnauthorized}
	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil {
		t.Fatal("expected an error after two 401s, got nil")
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if mock.mintCount() != 2 {
		t.Errorf("mint requests: got %d, want exactly 2", mock.mintCount())
	}
}

// TestGetLandingToken_RetriedResponseIsStillVerified: the retried 201 is not
// trusted outright.
func TestGetLandingToken_RetriedResponseIsStillVerified(t *testing.T) {
	token := runtimeToken(t)
	m := landingResponse(token, "some-repo", false)
	perms(m)["issues"] = "write"
	mock := newLandingMock(t, m)
	mock.mintStatuses = []int{http.StatusUnauthorized, http.StatusCreated}

	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil || !strings.Contains(err.Error(), "grant mismatch") {
		t.Fatalf("expected a grant mismatch error, got %v", err)
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if d := mock.deletes(); len(d) != 1 || d[0] != "Bearer "+token {
		t.Errorf("DELETE calls: got %v, want one carrying the retried token", d)
	}
}

// TestGetLandingToken_ErrorsCarryNeitherTokenNorLevel: R-4's last criterion.
// The control proves the check can see the token: the same substring test
// finds it in the mock's response body.
func TestGetLandingToken_ErrorsCarryNeitherTokenNorLevel(t *testing.T) {
	token := runtimeToken(t)
	m := landingResponse(token, "some-repo", false)
	perms(m)["contents"] = "lvl-granted-a"
	perms(m)["extra-key"] = "lvl-granted-b"
	mock := newLandingMock(t, m)

	_, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "test-owner", "some-repo", mock.srv.URL)
	if err == nil {
		t.Fatal("expected a mismatch error, got nil")
	}
	mock.mu.Lock()
	sent := mock.lastBody
	mock.mu.Unlock()
	if !strings.Contains(sent, token) {
		t.Fatalf("control failed: the mock's response body does not contain the token, so the check below proves nothing")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error carries the token: %v", err)
	}
	for _, lvl := range []string{"lvl-granted-a", "lvl-granted-b"} {
		if strings.Contains(err.Error(), lvl) {
			t.Errorf("error carries granted level %q: %v", lvl, err)
		}
	}
}

// --- R-2 (function level): routing before any network call ---

// TestGetLandingToken_UnknownOwnerFailsBeforeAnyRequest: fail-closed routing.
func TestGetLandingToken_UnknownOwnerFailsBeforeAnyRequest(t *testing.T) {
	mock := newLandingMock(t, landingResponse(runtimeToken(t), "r", false))
	got, err := getLandingToken(context.Background(), newTestConfig(landingInstallID), newTestKey(t), "unknown-owner", "r", mock.srv.URL)
	if err == nil || !strings.Contains(err.Error(), "unknown owner") {
		t.Fatalf("expected an unknown owner error, got %v", err)
	}
	if got != "" {
		t.Errorf("token: got %q, want empty", got)
	}
	if mock.mintCount() != 0 {
		t.Errorf("mint requests: got %d, want 0", mock.mintCount())
	}
}

// --- R-5: the one channel, and the cache ---

// TestGetLandingToken_WritesToNeitherStream: R-5 under the captureStd swap,
// the mint function returns the token and writes to neither stream.
func TestGetLandingToken_WritesToNeitherStream(t *testing.T) {
	token := runtimeToken(t)
	mock := newLandingMock(t, landingResponse(token, "some-repo", false))
	cfg, key := newTestConfig(landingInstallID), newTestKey(t)

	var got string
	var err error
	stdout, stderr := captureStd(t, func() {
		got, err = getLandingToken(context.Background(), cfg, key, "test-owner", "some-repo", mock.srv.URL)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != token {
		t.Errorf("token: got %q, want %q", got, token)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("writes: stdout %q, stderr %q, want both empty", stdout, stderr)
	}
}

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// TestGetLandingToken_CacheDirUnchangedByMintOrMismatch: R-5, a seeded cache
// directory holds the same files, byte for byte, before and after a mint, and
// after a refused one. With no seed, no cache directory appears.
func TestGetLandingToken_CacheDirUnchangedByMintOrMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDir := defaultCacheDir()
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := runtimeToken(t)
	if err := cachePut(cacheDir, landingInstallID, CachedToken{Token: seed, Expiry: time.Now().Add(55 * time.Minute)}); err != nil {
		t.Fatalf("cachePut seed: %v", err)
	}
	before := snapshotDir(t, cacheDir)
	if len(before) == 0 {
		t.Fatal("seed produced no cache file; the comparison below would prove nothing")
	}

	cfg, key := newTestConfig(landingInstallID), newTestKey(t)
	token := runtimeToken(t)
	mock := newLandingMock(t, landingResponse(token, "some-repo", false))
	if got, err := getLandingToken(context.Background(), cfg, key, "test-owner", "some-repo", mock.srv.URL); err != nil || got != token {
		t.Fatalf("mint: got (%q, %v), want (%q, nil)", got, err, token)
	}
	if after := snapshotDir(t, cacheDir); !reflect.DeepEqual(before, after) {
		t.Errorf("cache dir changed by a successful mint:\nbefore=%v\nafter=%v", before, after)
	}

	widened := landingResponse(runtimeToken(t), "some-repo", false)
	widened["repository_selection"] = "all"
	mock.script(widened)
	if _, err := getLandingToken(context.Background(), cfg, key, "test-owner", "some-repo", mock.srv.URL); err == nil {
		t.Fatal("expected a mismatch error")
	}
	if after := snapshotDir(t, cacheDir); !reflect.DeepEqual(before, after) {
		t.Errorf("cache dir changed by a refused mint:\nbefore=%v\nafter=%v", before, after)
	}

	fresh := t.TempDir()
	t.Setenv("HOME", fresh)
	mock.script(landingResponse(runtimeToken(t), "some-repo", false))
	if _, err := getLandingToken(context.Background(), cfg, key, "test-owner", "some-repo", mock.srv.URL); err != nil {
		t.Fatalf("mint with an empty HOME: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, ".cache")); !os.IsNotExist(err) {
		t.Errorf("a mint created a cache directory under HOME (stat err: %v)", err)
	}
}

// --- R-1: dispatchToken ---

const dispatchUsageProbe = "usage"

// emptyConfigHome points HOME at a directory with no config, so a call that
// reaches config load fails with "open config" and a call refused earlier
// fails with "usage".
func emptyConfigHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func assertRefusedAsUsage(t *testing.T, args []string) {
	t.Helper()
	err := dispatchToken(args)
	if err == nil {
		t.Fatalf("dispatchToken(%q): expected a refusal, got nil", args)
	}
	if !strings.Contains(err.Error(), dispatchUsageProbe) {
		t.Errorf("dispatchToken(%q): error %q does not contain %q", args, err, dispatchUsageProbe)
	}
	if strings.Contains(err.Error(), "open config") {
		t.Errorf("dispatchToken(%q): reached config load before refusing: %v", args, err)
	}
}

// TestDispatchToken_LandingFormAcceptedByExactEquality: R-1, `--landing
// <owner>/<repo>` reaches the landing command (here observed as the config
// load failing, with no usage text).
func TestDispatchToken_LandingFormAcceptedByExactEquality(t *testing.T) {
	emptyConfigHome(t)
	err := dispatchToken([]string{"--landing", "myorg/myrepo"})
	if err == nil {
		t.Fatal("expected the config load to fail in an empty HOME, got nil")
	}
	if !strings.Contains(err.Error(), "open config") {
		t.Errorf("error %q does not contain %q: the form was not accepted", err, "open config")
	}
	if strings.Contains(err.Error(), dispatchUsageProbe) {
		t.Errorf("accepted form was refused as usage: %v", err)
	}
}

// TestDispatchToken_LandingRefusedShapes: R-1, each refused shape is refused
// before config load with usage text.
func TestDispatchToken_LandingRefusedShapes(t *testing.T) {
	emptyConfigHome(t)
	for name, args := range map[string][]string{
		"equals form":                 {"--landing=o/r"},
		"two slashes":                 {"--landing", "a/b/c"},
		"flag alone":                  {"--landing"},
		"empty owner":                 {"--landing", "/r"},
		"empty repo":                  {"--landing", "o/"},
		"no slash":                    {"--landing", "owner"},
		"extra positional":            {"--landing", "o/r", "extra"},
		"flag after positional":       {"o/r", "--landing"},
		"case differs":                {"--Landing", "o/r"},
		"prefix match is not a match": {"--landing-x", "o/r"},
		"typo":                        {"--landng", "o/r"},
		"single dash argument":        {"-x"},
		"bare dash":                   {"-"},
		"double dash":                 {"--"},
	} {
		t.Run(name, func(t *testing.T) { assertRefusedAsUsage(t, args) })
	}
}

// TestDispatchToken_AnySingleDashPrefixedArgumentRefused: R-1's one change to
// the single-argument form, as a property over generated arguments.
func TestDispatchToken_AnySingleDashPrefixedArgumentRefused(t *testing.T) {
	emptyConfigHome(t)
	r := rand.New(rand.NewPCG(5, 6))
	const alphabet = "abcXYZ019-_./=@ "
	for i := 0; i < 300; i++ {
		n := r.IntN(12)
		b := []byte{'-'}
		for j := 0; j < n; j++ {
			b = append(b, alphabet[r.IntN(len(alphabet))])
		}
		assertRefusedAsUsage(t, []string{string(b)})
	}
}

// TestDispatchToken_SingleArgumentWithoutDashStillReachesOwnerForm: R-1, the
// existing form is otherwise unchanged (and T3's TP-L10 in process).
func TestDispatchToken_SingleArgumentWithoutDashStillReachesOwnerForm(t *testing.T) {
	emptyConfigHome(t)
	for _, arg := range []string{"someowner", "someowner/repo", "o"} {
		err := dispatchToken([]string{arg})
		if err == nil || !strings.Contains(err.Error(), "open config") {
			t.Errorf("dispatchToken(%q): got %v, want a config load failure (the owner form)", arg, err)
		}
		if err != nil && strings.Contains(err.Error(), dispatchUsageProbe) {
			t.Errorf("dispatchToken(%q): refused as usage: %v", arg, err)
		}
	}
}

// TestDispatchToken_UsageNamesAllThreeForms: R-1, the usage text names the
// owner form, the rate-limit form and the landing form.
func TestDispatchToken_UsageNamesAllThreeForms(t *testing.T) {
	err := dispatchToken([]string{})
	if err == nil {
		t.Fatal("expected a usage error for no arguments")
	}
	msg := err.Error()
	for _, want := range []string{"sigil token <owner>", "--rate-limit-only <owner>/<repo>", "--landing <owner>/<repo>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("usage %q does not name %q", msg, want)
		}
	}
}

// --- R-2: order through dispatchToken ---

func writeLandingConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "sigil")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := "op_key_ref = \"op://vault/item/field\"\napp_id = 42\n\n[installations]\n\"known-owner\" = 100\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SIGIL_GITHUB_API_URL", "")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
}

// TestDispatchToken_LandingOrder: R-2, config load, scheme check, routing,
// key read, in that order, and no terminal check and no cache directory.
func TestDispatchToken_LandingOrder(t *testing.T) {
	t.Run("unknown owner is a routing error", func(t *testing.T) {
		writeLandingConfig(t)
		err := dispatchToken([]string{"--landing", "unknown-owner/r"})
		if err == nil || !strings.Contains(err.Error(), "unknown owner") {
			t.Errorf("got %v, want an unknown owner error", err)
		}
	})
	t.Run("scheme check runs before routing", func(t *testing.T) {
		writeLandingConfig(t)
		t.Setenv("SIGIL_GITHUB_API_URL", "http://example.invalid")
		err := dispatchToken([]string{"--landing", "unknown-owner/r"})
		if err == nil || !strings.Contains(err.Error(), "https scheme required") {
			t.Fatalf("got %v, want an https scheme error", err)
		}
		if strings.Contains(err.Error(), "unknown owner") {
			t.Errorf("routing ran before the scheme check: %v", err)
		}
	})
	t.Run("known owner reaches the key read, with no terminal check", func(t *testing.T) {
		writeLandingConfig(t)
		err := dispatchToken([]string{"--landing", "known-owner/r"})
		if err == nil || !strings.Contains(err.Error(), "OP_SERVICE_ACCOUNT_TOKEN is not set") {
			t.Fatalf("got %v, want the key-read error", err)
		}
		if strings.Contains(err.Error(), "stdout is not a terminal") {
			t.Errorf("the landing form ran the terminal check: %v", err)
		}
		if _, statErr := os.Stat(defaultCacheDir()); !os.IsNotExist(statErr) {
			t.Errorf("the landing form touched the cache directory (stat err: %v)", statErr)
		}
	})
}
