package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeGoogle answers like Google's token and Search Console addresses and
// checks that the key really signed the request.
func fakeGoogle(t *testing.T, pub *rsa.PublicKey, props []string) (*httptest.Server, *atomic.Int64, *atomic.Value) {
	var searches atomic.Int64
	var last atomic.Value
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if len(parts) != 3 || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, `{"error":"invalid_grant","error_description":"bad assertion"}`, 400)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
			http.Error(w, `{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`, 400)
			return
		}
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		if !strings.Contains(string(claims), "webmasters.readonly") || !strings.Contains(string(claims), "reader@proj.iam.gserviceaccount.com") {
			http.Error(w, `{"error":"invalid_scope"}`, 400)
			return
		}
		io.WriteString(w, `{"access_token":"tok-1","expires_in":3600,"token_type":"Bearer"}`)
	})
	mux.HandleFunc("/sites", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, `{"error":{"message":"no token"}}`, 401)
			return
		}
		entries := []map[string]string{}
		for _, p := range props {
			entries = append(entries, map[string]string{"siteUrl": p, "permissionLevel": "siteRestrictedUser"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"siteEntry": entries})
	})
	mux.HandleFunc("/sites/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/searchAnalytics/query") || !strings.Contains(r.URL.EscapedPath(), "sc-domain%3Ashop.test") {
			http.Error(w, `{"error":{"message":"User does not have sufficient permission for site."}}`, 403)
			return
		}
		searches.Add(1)
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		if in["dimensions"] == nil {
			io.WriteString(w, `{"rows":[{"clicks":90,"impressions":3000,"ctr":0.03,"position":7.26}]}`)
			return
		}
		last.Store(string(body))
		io.WriteString(w, `{"rows":[{"keys":["gluten free cake"],"clicks":40,"impressions":800,"ctr":0.05,"position":3.44},{"keys":["shop test"],"clicks":25,"impressions":30,"ctr":0.8333,"position":1}]}`)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, &searches, &last
}

func TestGoogleSearchWords(t *testing.T) {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(pk)
	a := newApp(t, nil)
	google, searches, last := fakeGoogle(t, &pk.PublicKey, []string{"https://other.test/", "sc-domain:shop.test"})
	a.s.gsc.API = google.URL
	keyFile, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "reader@proj.iam.gserviceaccount.com", "token_uri": google.URL + "/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))})

	shop := id(a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "shop.test"}))
	blog := id(a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "blog.test"}))

	// no key yet
	if kw := a.ok(a.alice, "GET", "/api/v1/sites/"+shop+"/keywords", nil); str(kw, "state") != "off" {
		t.Fatalf("without a key: %v", kw)
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"google_key": "not a key"})
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"google_key": `{"client_email":"x@y","private_key":"nope"}`})

	// trying a key before saving it
	tried := a.ok(a.alice, "POST", "/api/v1/google/test", map[string]any{"key": string(keyFile)})
	if tried["ok"] != true || str(tried, "email") != "reader@proj.iam.gserviceaccount.com" || len(tried["properties"].([]any)) != 2 {
		t.Fatalf("test: %v", tried)
	}
	found := map[string]string{}
	for _, s := range tried["sites"].([]any) {
		found[str(s.(map[string]any), "domain")] = str(s.(map[string]any), "property")
	}
	if found["shop.test"] != "sc-domain:shop.test" || found["blog.test"] != "" {
		t.Fatalf("which sites the key reads: %v", found)
	}

	// saved: the key never comes back, only whose it is
	st := a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"google_key": string(keyFile)})
	if st["google_configured"] != true || str(st, "google_email") != "reader@proj.iam.gserviceaccount.com" {
		t.Fatalf("settings: %v", st)
	}
	if raw, _ := json.Marshal(st); strings.Contains(string(raw), "PRIVATE KEY") || st["google_key"] != nil {
		t.Fatal("the key leaves the server")
	}
	// another save of the form leaves the key alone
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"week_start": 7, "google_email": "x", "google_configured": false})

	kw := a.ok(a.alice, "GET", "/api/v1/sites/"+shop+"/keywords?range=30d", nil)
	rows := kw["rows"].([]any)
	if str(kw, "state") != "ok" || str(kw, "property") != "sc-domain:shop.test" || len(rows) != 2 {
		t.Fatalf("keywords: %v", kw)
	}
	first := rows[0].(map[string]any)
	if str(first, "words") != "gluten free cake" || num(first, "clicks") != 40 || num(first, "ctr") != 5 || num(first, "position") != 3.4 {
		t.Fatalf("first row: %v", first)
	}
	if tot := kw["totals"].(map[string]any); num(tot, "clicks") != 90 || num(tot, "position") != 7.3 {
		t.Fatalf("totals: %v", tot)
	}
	// asked again: from memory, not from Google
	before := searches.Load()
	a.ok(a.alice, "GET", "/api/v1/sites/"+shop+"/keywords?range=30d", nil)
	if searches.Load() != before {
		t.Fatal("the same question went to Google twice")
	}

	// a page and a device go to Google as its own filters; a country cannot
	f := a.ok(a.alice, "GET", "/api/v1/sites/"+shop+"/keywords?range=30d&f=page:/cakes/*&f=device:Phone&f=country:DE", nil)
	if f["partial"] != true {
		t.Fatalf("a filter Google cannot follow: %v", f)
	}
	sent, _ := last.Load().(string)
	if !strings.Contains(sent, `"includingRegex"`) || !strings.Contains(sent, `/cakes/.*`) || !strings.Contains(sent, `"MOBILE"`) {
		t.Fatalf("what went to Google: %s", sent)
	}

	// a site the key was not added to
	no := a.ok(a.alice, "GET", "/api/v1/sites/"+blog+"/keywords", nil)
	if str(no, "state") != "no_property" || str(no, "email") == "" || len(no["seen"].([]any)) != 2 {
		t.Fatalf("no property: %v", no)
	}

	// Bob has no key and cannot see Alice's site; taking the key away switches it off
	a.expect(404, a.bob, "GET", "/api/v1/sites/"+shop+"/keywords", nil)
	if b := a.ok(a.bob, "POST", "/api/v1/google/test", nil); b["ok"] != false {
		t.Fatalf("Bob's test: %v", b)
	}
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"google_key": ""})
	if kw := a.ok(a.alice, "GET", "/api/v1/sites/"+shop+"/keywords", nil); str(kw, "state") != "off" {
		t.Fatalf("after removing the key: %v", kw)
	}
}

func TestPageRE(t *testing.T) {
	for path, want := range map[string]string{
		"/":        `^https?://[^/]+/?(\?.*)?(#.*)?$`,
		"/menu":    `^https?://[^/]+/menu/?(\?.*)?(#.*)?$`,
		"/blog/*":  `^https?://[^/]+/blog/.*/?(\?.*)?(#.*)?$`,
		"/a.b/c+d": `^https?://[^/]+/a\.b/c\+d/?(\?.*)?(#.*)?$`,
	} {
		if got := pageRE(path); got != want {
			t.Errorf("pageRE(%q) = %s, want %s", path, got, want)
		}
	}
}
