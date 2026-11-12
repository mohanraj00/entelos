package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

const testIssuer = "https://issuer.test"
const testAudience = "entelos-approval"
const testTenant = "demo"

// testNow is the fixed claim clock of these tests.
var testNow = time.Unix(1800000000, 0)

type testKey struct {
	kid  string
	priv ed25519.PrivateKey
}

func newKey(t *testing.T, kid string) testKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return testKey{kid, priv}
}

func (k testKey) jwk() lifecycle.Object {
	pub := k.priv.Public().(ed25519.PublicKey)
	return lifecycle.Object{"kty": "OKP", "crv": "Ed25519", "kid": k.kid, "x": base64.RawURLEncoding.EncodeToString(pub)}
}

// writeIssuer writes an issuer file with the keys and with each principal
// as {subject, roles}.
func writeIssuer(t *testing.T, path string, keys []testKey, principals lifecycle.Array) {
	t.Helper()
	set := lifecycle.Array{}
	for _, k := range keys {
		set = append(set, k.jwk())
	}
	writePrivate(t, path, lifecycle.Encode(lifecycle.Object{"format": IssuerFormat, "issuer": testIssuer,
		"audience": testAudience, "max_lifetime_seconds": lifecycle.Int{Text: "3600"}, "keys": set, "principals": principals}))
}

func segment(v lifecycle.Object) string {
	return base64.RawURLEncoding.EncodeToString(lifecycle.Encode(v))
}

func num(n int64) lifecycle.Int { return lifecycle.Int{Text: strconv.FormatInt(n, 10)} }

// claimBody gives a valid payload for the subject at the time now.
func claimBody(subject string, now time.Time) lifecycle.Object {
	return lifecycle.Object{"iss": testIssuer, "aud": testAudience, "sub": subject, "tenant": testTenant,
		"iat": num(now.Unix()), "exp": num(now.Unix() + 300)}
}

func sign(k testKey, header, body lifecycle.Object) string {
	input := segment(header) + "." + segment(body)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(k.priv, []byte(input)))
}

func mint(k testKey, body lifecycle.Object) string {
	return sign(k, lifecycle.Object{"alg": "EdDSA", "kid": k.kid, "typ": "JWT"}, body)
}

// claimHost records the principal of each request.
type claimHost struct{ calls *[]string }

func (h claimHost) Handle(any) (any, error) { return lifecycle.Object{}, nil }

func (h claimHost) HandleAs(actor string, roles []string, q any) (any, error) {
	*h.calls = append(*h.calls, actor+":"+strings.Join(roles, ","))
	return lifecycle.Object{"actor": actor}, nil
}

type httpFixture struct {
	url   string
	key   testKey
	calls *[]string
	logs  *[]string
}

func startHTTP(t *testing.T, config HTTPConfig, adapter *Adapter) string {
	t.Helper()
	l, loopback, err := ListenHTTP("127.0.0.1:0", false)
	if err != nil {
		t.Fatal(err)
	}
	config.Loopback = loopback
	handler, err := NewHTTPHandler(adapter, config)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	server := NewHTTPServer(handler)
	go server.Serve(l)
	t.Cleanup(func() { server.Shutdown() })
	return "http://" + l.Addr().String() + HTTPPath
}

func claimFixture(t *testing.T) httpFixture {
	t.Helper()
	inTempDir(t)
	key := newKey(t, "k1")
	writeIssuer(t, "issuer.json", []testKey{key}, lifecycle.Array{
		lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	calls, logs := []string{}, []string{}
	url := startHTTP(t, HTTPConfig{IssuerFile: "issuer.json", Tenant: testTenant,
		Now: func() time.Time { return testNow }, Log: func(s string) { logs = append(logs, s) }},
		NewAdapter(claimHost{&calls}, loadTestSchemas(t)))
	return httpFixture{url, key, &calls, &logs}
}

var proposalsRequest = []byte(`{"body":{},"operation":"proposals","protocol":"1.1","request_id":"h-1"}`)

// post sends one request. An empty bearer sends no Authorization header.
func post(t *testing.T, url, bearer string, payload []byte, edit func(*http.Request)) (int, lifecycle.Object) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if edit != nil {
		edit(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Content-Type") != "application/json" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("response headers: %v", resp.Header)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatalf("response is not JSON: %q", raw)
	}
	return resp.StatusCode, value.(lifecycle.Object)
}

func wantRefusal(t *testing.T, name string, status int, got lifecycle.Object, wantStatus int, code, stage string) {
	t.Helper()
	d, _ := got["diagnostic"].(lifecycle.Object)
	if status != wantStatus || got["status"] != code || d["code"] != code || d["stage"] != stage || got["protocol"] != "1.0" {
		t.Errorf("%s: got %d %s, want %d %s at %s", name, status, lifecycle.Encode(got), wantStatus, code, stage)
	}
}

func TestHTTPClaimGivesThePrincipal(t *testing.T) {
	f := claimFixture(t)
	status, got := post(t, f.url, mint(f.key, claimBody("reviewer-a", testNow)), proposalsRequest, nil)
	if status != 200 || got["status"] != "ok" || got["protocol"] != "1.1" || got["request_id"] != "h-1" {
		t.Fatalf("got %d %s", status, lifecycle.Encode(got))
	}
	if strings.Join(*f.calls, " ") != "reviewer-a:reviewer" {
		t.Fatalf("host calls %v", *f.calls)
	}
	// The body cannot name another actor: the Host gets the claim subject.
	status, _ = post(t, f.url, mint(f.key, claimBody("reviewer-a", testNow)),
		[]byte(`{"body":{"actor":"author-a"},"operation":"proposals","protocol":"1.1","request_id":"h-2"}`), nil)
	if status != 400 || len(*f.calls) != 1 {
		t.Fatalf("a body actor got %d, host calls %v", status, *f.calls)
	}
}

func TestHTTPRefusesBadClaims(t *testing.T) {
	f := claimFixture(t)
	other := newKey(t, "k1")
	header := lifecycle.Object{"alg": "EdDSA", "kid": "k1", "typ": "JWT"}
	with := func(edit func(lifecycle.Object)) lifecycle.Object {
		b := claimBody("reviewer-a", testNow)
		edit(b)
		return b
	}
	headerWith := func(k, v string) lifecycle.Object {
		h := lifecycle.Object{"alg": "EdDSA", "kid": "k1"}
		h[k] = v
		return h
	}
	good := mint(f.key, claimBody("reviewer-a", testNow))
	parts := strings.Split(good, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[0] ^= 1
	flipped := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)
	cases := []struct{ name, bearer, reason string }{
		{"missing claim", "", "exactly one Authorization"},
		{"bad signature", flipped, "does not verify"},
		{"other key with the same kid", sign(other, header, claimBody("reviewer-a", testNow)), "does not verify"},
		{"payload changed after signing", parts[0] + "." + segment(claimBody("author-a", testNow)) + "." + parts[2], "does not verify"},
		{"wrong issuer", mint(f.key, with(func(b lifecycle.Object) { b["iss"] = "https://other.test" })), "issuer"},
		{"expired", mint(f.key, with(func(b lifecycle.Object) { b["exp"] = num(testNow.Unix() - 1) })), "expired"},
		{"expires now", mint(f.key, with(func(b lifecycle.Object) { b["exp"] = num(testNow.Unix()) })), "expired"},
		{"no exp", mint(f.key, with(func(b lifecycle.Object) { delete(b, "exp") })), "exp and iat"},
		{"exp is a string", mint(f.key, with(func(b lifecycle.Object) { b["exp"] = "1800000300" })), "exp and iat"},
		{"iat in the future", mint(f.key, with(func(b lifecycle.Object) {
			b["iat"] = num(testNow.Unix() + 61)
			b["exp"] = num(testNow.Unix() + 300)
		})), "future"},
		{"lifetime too long", mint(f.key, with(func(b lifecycle.Object) { b["exp"] = num(testNow.Unix() + 3601) })), "lifetime"},
		{"lifetime overflow", mint(f.key, with(func(b lifecycle.Object) {
			b["iat"] = lifecycle.Int{Text: "-9223372036854775808"}
			b["exp"] = lifecycle.Int{Text: "9223372036854775807"}
		})), "iat is negative"},
		{"negative iat", mint(f.key, with(func(b lifecycle.Object) { b["iat"] = num(-1) })), "iat is negative"},
		{"nbf in the future", mint(f.key, with(func(b lifecycle.Object) { b["nbf"] = num(testNow.Unix() + 61) })), "not valid yet"},
		{"wrong audience", mint(f.key, with(func(b lifecycle.Object) { b["aud"] = lifecycle.Array{"other"} })), "audience"},
		{"wrong tenant", mint(f.key, with(func(b lifecycle.Object) { b["tenant"] = "other" })), "tenant"},
		{"no tenant", mint(f.key, with(func(b lifecycle.Object) { delete(b, "tenant") })), "tenant"},
		{"unknown subject", mint(f.key, claimBody("reviewer-z", testNow)), "subject"},
		{"unknown kid", mint(newKey(t, "k2"), claimBody("reviewer-a", testNow)), "kid"},
		{"alg none", segment(lifecycle.Object{"alg": "none", "kid": "k1"}) + "." + segment(claimBody("reviewer-a", testNow)) + ".", "EdDSA"},
		{"alg HS256", sign(f.key, lifecycle.Object{"alg": "HS256", "kid": "k1"}, claimBody("reviewer-a", testNow)), "EdDSA"},
		{"crit header", sign(f.key, headerWith("crit", "exp"), claimBody("reviewer-a", testNow)), "not permitted"},
		{"jku header", sign(f.key, headerWith("jku", "https://x.test/keys"), claimBody("reviewer-a", testNow)), "not permitted"},
		{"padded segment", parts[0] + "=." + parts[1] + "." + parts[2], "base64url"},
		{"two segments", parts[0] + "." + parts[1], "compact"},
		{"static token in claim mode", testToken, "compact"},
		{"too long", strings.Repeat("a", MaxClaim+1), "byte bound"},
	}
	for _, c := range cases {
		before := len(*f.logs)
		status, got := post(t, f.url, c.bearer, proposalsRequest, nil)
		wantRefusal(t, c.name, status, got, 401, "unauthenticated", "authentication")
		if len(*f.logs) != before+1 || !strings.Contains((*f.logs)[before], c.reason) {
			t.Errorf("%s: log %v does not give the reason %q", c.name, (*f.logs)[before:], c.reason)
		}
		if msg := got["diagnostic"].(lifecycle.Object)["message"]; msg != errClaim.Error() {
			t.Errorf("%s: the response shows the reason: %v", c.name, msg)
		}
	}
	for _, scheme := range []string{"Basic ", "Bearer  ", ""} {
		status, got := post(t, f.url, "", proposalsRequest, func(r *http.Request) { r.Header.Set("Authorization", scheme+good) })
		wantRefusal(t, "scheme "+scheme, status, got, 401, "unauthenticated", "authentication")
	}
	status, got := post(t, f.url, "", proposalsRequest, func(r *http.Request) {
		r.Header.Add("Authorization", "Bearer "+good)
		r.Header.Add("Authorization", "Bearer "+good)
	})
	wantRefusal(t, "two headers", status, got, 401, "unauthenticated", "authentication")
	if len(*f.calls) != 0 {
		t.Fatalf("the host got a refused request: %v", *f.calls)
	}
}

func TestHTTPReadsTheIssuerFileForEachRequest(t *testing.T) {
	f := claimFixture(t)
	claim := mint(f.key, claimBody("reviewer-a", testNow))
	if status, _ := post(t, f.url, claim, proposalsRequest, nil); status != 200 {
		t.Fatalf("got %d", status)
	}
	// Key rotation: the old key leaves the set.
	writeIssuer(t, "issuer.json", []testKey{newKey(t, "k2")}, lifecycle.Array{
		lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	status, got := post(t, f.url, claim, proposalsRequest, nil)
	wantRefusal(t, "removed key", status, got, 401, "unauthenticated", "authentication")
	// A file that is not private gives unauthenticated, not a fallback.
	writeIssuer(t, "issuer.json", []testKey{f.key}, lifecycle.Array{
		lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	if err := os.Chmod("issuer.json", 0o644); err != nil {
		t.Fatal(err)
	}
	status, got = post(t, f.url, claim, proposalsRequest, nil)
	wantRefusal(t, "public issuer file", status, got, 401, "unauthenticated", "authentication")
	if len(*f.calls) != 1 {
		t.Fatalf("host calls %v", *f.calls)
	}
}

func TestHTTPStaticTokenOnlyInLocalMode(t *testing.T) {
	inTempDir(t)
	tokens := writeCredentials(t, "credentials.json", lifecycle.Object{
		"a": lifecycle.Object{"actor": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	calls := []string{}
	adapter := NewAdapter(claimHost{&calls}, loadTestSchemas(t))
	url := startHTTP(t, HTTPConfig{CredentialsFile: "credentials.json", LocalTokens: true}, adapter)
	status, got := post(t, url, tokens["a"], proposalsRequest, nil)
	if status != 200 || got["status"] != "ok" || strings.Join(calls, " ") != "reviewer-a:reviewer" {
		t.Fatalf("got %d %s, calls %v", status, lifecycle.Encode(got), calls)
	}
	for name, bearer := range map[string]string{"unknown token": randomToken(t), "uppercase": strings.ToUpper(tokens["a"]),
		"short": tokens["a"][:63], "missing": ""} {
		status, got := post(t, url, bearer, proposalsRequest, nil)
		wantRefusal(t, name, status, got, 401, "unauthenticated", "authentication")
	}
	// A claim is not a static token.
	key := newKey(t, "k1")
	status, got = post(t, url, mint(key, claimBody("reviewer-a", testNow)), proposalsRequest, nil)
	wantRefusal(t, "claim in local mode", status, got, 401, "unauthenticated", "authentication")
	if len(calls) != 1 {
		t.Fatalf("host calls %v", calls)
	}
	// Local mode needs a loopback address, and one mode only.
	writeIssuer(t, "issuer.json", []testKey{key}, lifecycle.Array{
		lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	for name, config := range map[string]HTTPConfig{
		"not loopback": {CredentialsFile: "credentials.json", LocalTokens: true},
		"two modes":    {CredentialsFile: "credentials.json", LocalTokens: true, IssuerFile: "issuer.json", Loopback: true, Tenant: testTenant},
		"no mode":      {CredentialsFile: "credentials.json", Loopback: true},
		"no tenant":    {IssuerFile: "issuer.json", Loopback: true},
	} {
		if _, err := NewHTTPHandler(adapter, config); err == nil {
			t.Errorf("%s: NewHTTPHandler accepted the configuration", name)
		}
	}
}

func TestHTTPRefusesAnActorWithOtherRoles(t *testing.T) {
	inTempDir(t)
	writeCredentials(t, "credentials.json", lifecycle.Object{
		"a": lifecycle.Object{"actor": "reviewer-a", "roles": lifecycle.Array{"reviewer"}}})
	writeIssuer(t, "issuer.json", []testKey{newKey(t, "k1")}, lifecycle.Array{
		lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer", "author"}}})
	config := HTTPConfig{IssuerFile: "issuer.json", CredentialsFile: "credentials.json", Loopback: true, Tenant: testTenant}
	if _, err := NewHTTPHandler(NewAdapter(stubHost{}, loadTestSchemas(t)), config); err == nil || !strings.Contains(err.Error(), "other roles") {
		t.Fatalf("got %v", err)
	}
}

func TestHTTPRequestBounds(t *testing.T) {
	f := claimFixture(t)
	claim := mint(f.key, claimBody("reviewer-a", testNow))
	big := append([]byte(`{"pad":"`), bytes.Repeat([]byte("a"), MaxFrame)...)
	big = append(big, `"}`...)
	cases := []struct {
		name    string
		payload []byte
		edit    func(*http.Request)
		status  int
		code    string
	}{
		{"wrong content type", proposalsRequest, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415, "invalid_request"},
		{"other charset", proposalsRequest, func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=latin1") }, 415, "invalid_request"},
		{"body over the bound", big, nil, 413, "invalid_frame"},
		{"chunked body over the bound", big, func(r *http.Request) { r.ContentLength = -1 }, 413, "invalid_frame"},
		{"empty body", nil, nil, 400, "invalid_frame"},
		{"not JSON", []byte("{"), nil, 400, "invalid_request"},
		{"other host", proposalsRequest, func(r *http.Request) { r.Host = "attacker.test" }, 421, "invalid_request"},
		{"other path", proposalsRequest, func(r *http.Request) { r.URL.Path = "/v1/other" }, 404, "invalid_request"},
		{"query", proposalsRequest, func(r *http.Request) { r.URL.RawQuery = "a=1" }, 404, "invalid_request"},
		{"GET", proposalsRequest, func(r *http.Request) { r.Method = http.MethodGet }, 405, "invalid_request"},
	}
	for _, c := range cases {
		status, got := post(t, f.url, claim, c.payload, c.edit)
		wantRefusal(t, c.name, status, got, c.status, c.code, "protocol")
	}
	ok := []string{"application/json; charset=utf-8", "application/json"}
	for _, ct := range ok {
		status, _ := post(t, f.url, claim, proposalsRequest, func(r *http.Request) { r.Header.Set("Content-Type", ct) })
		if status != 200 {
			t.Errorf("%s: got %d", ct, status)
		}
	}
	// A refused credential does not get its body read.
	status, got := post(t, f.url, "", big, nil)
	wantRefusal(t, "no claim and a big body", status, got, 401, "unauthenticated", "authentication")
	if len(*f.calls) != len(ok) {
		t.Fatalf("host calls %v", *f.calls)
	}
}

func TestListenHTTPIsLoopbackByDefault(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "[::]:0", "localhost:0", "example.test:0", "8740"} {
		if l, _, err := ListenHTTP(addr, false); err == nil {
			l.Close()
			t.Errorf("%s: ListenHTTP accepted the address", addr)
		}
	}
	l, loopback, err := ListenHTTP("127.0.0.1:0", false)
	if err != nil || !loopback {
		t.Fatalf("loopback %v, %v", loopback, err)
	}
	l.Close()
}

func TestParseIssuerRefusesBadFiles(t *testing.T) {
	key := newKey(t, "k1").jwk()
	principals := lifecycle.Array{lifecycle.Object{"subject": "a", "roles": lifecycle.Array{"reviewer"}}}
	base := func() lifecycle.Object {
		return lifecycle.Object{"format": IssuerFormat, "issuer": testIssuer, "audience": testAudience,
			"max_lifetime_seconds": num(3600), "keys": lifecycle.Array{key}, "principals": principals}
	}
	if _, err := parseIssuer(lifecycle.Encode(base())); err != nil {
		t.Fatal(err)
	}
	edits := map[string]func(lifecycle.Object){
		"extra field": func(d lifecycle.Object) { d["extra"] = true },
		"no keys":     func(d lifecycle.Object) { d["keys"] = lifecycle.Array{} },
		"rsa key": func(d lifecycle.Object) {
			d["keys"] = lifecycle.Array{lifecycle.Object{"kty": "RSA", "crv": "Ed25519", "kid": "k1", "x": key["x"]}}
		},
		"short key": func(d lifecycle.Object) {
			d["keys"] = lifecycle.Array{lifecycle.Object{"kty": "OKP", "crv": "Ed25519", "kid": "k1", "x": "AAAA"}}
		},
		"duplicate kid":     func(d lifecycle.Object) { d["keys"] = lifecycle.Array{key, key} },
		"lifetime too long": func(d lifecycle.Object) { d["max_lifetime_seconds"] = num(maxLifetime + 1) },
		"empty issuer":      func(d lifecycle.Object) { d["issuer"] = "" },
		"unknown role": func(d lifecycle.Object) {
			d["principals"] = lifecycle.Array{lifecycle.Object{"subject": "a", "roles": lifecycle.Array{"admin"}}}
		},
		"duplicate subject": func(d lifecycle.Object) { d["principals"] = append(lifecycle.Array{}, principals[0], principals[0]) },
	}
	for name, edit := range edits {
		d := base()
		edit(d)
		if _, err := parseIssuer(lifecycle.Encode(d)); err == nil {
			t.Errorf("%s: parseIssuer accepted the file", name)
		}
	}
}
