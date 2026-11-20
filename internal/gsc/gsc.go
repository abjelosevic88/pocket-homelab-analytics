// Package gsc reads Google Search Console: the words people typed into
// Google before they came to a site. A website never sees those words
// itself; Google gives them to the site's owner through this API.
//
// It signs in with a service account key (the JSON file Google Cloud
// makes): no browser sign-in, nothing to renew. The key only works for
// the Search Console properties its email was added to as a user.
package gsc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
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
)

const scope = "https://www.googleapis.com/auth/webmasters.readonly"

// Key is a service account key.
type Key struct {
	Email    string
	TokenURI string
	key      *rsa.PrivateKey
}

// ParseKey reads the JSON file Google Cloud gives for a service account.
func ParseKey(raw string) (*Key, error) {
	var f struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &f); err != nil {
		return nil, errors.New("that is not the key file: paste the whole JSON file, from { to }")
	}
	if f.ClientEmail == "" || f.PrivateKey == "" {
		return nil, errors.New("the key file has no client_email or private_key: it must be a service account key (JSON)")
	}
	block, _ := pem.Decode([]byte(f.PrivateKey))
	if block == nil {
		return nil, errors.New("the private key in the file could not be read")
	}
	var pk *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		pk, _ = k.(*rsa.PrivateKey)
	} else if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		pk = k
	}
	if pk == nil {
		return nil, errors.New("the private key in the file is not an RSA key")
	}
	return &Key{Email: f.ClientEmail, TokenURI: f.TokenURI, key: pk}, nil
}

// Client talks to Google. The addresses can be changed for tests.
type Client struct {
	HTTP     *http.Client
	TokenURL string // "" = the key's own, else Google's
	API      string

	mu     sync.Mutex
	tokens map[string]token
}

type token struct {
	value string
	until time.Time
}

// New is a client for Google's real addresses.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, API: "https://www.googleapis.com/webmasters/v3", tokens: map[string]token{}}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// access is a bearer token for the key (kept until shortly before it ends).
func (c *Client) access(ctx context.Context, k *Key) (string, error) {
	c.mu.Lock()
	if t, ok := c.tokens[k.Email]; ok && time.Now().Before(t.until) {
		c.mu.Unlock()
		return t.value, nil
	}
	c.mu.Unlock()
	aud := "https://oauth2.googleapis.com/token"
	if k.TokenURI != "" {
		aud = k.TokenURI
	}
	at := aud
	if c.TokenURL != "" {
		at = c.TokenURL
	}
	now := time.Now().Unix()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": k.Email, "scope": scope, "aud": aud, "iat": now, "exp": now + 3600})
	signed := b64(head) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {signed + "." + b64(sig)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, at, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Desc        string `json:"error_description"`
	}
	if err := c.do(req, &out); err != nil {
		return "", fmt.Errorf("Google did not accept the key: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("Google did not accept the key: %s", firstOf(out.Desc, out.Error, "no token came back"))
	}
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl < 2*time.Minute {
		ttl = 2 * time.Minute
	}
	c.mu.Lock()
	c.tokens[k.Email] = token{out.AccessToken, time.Now().Add(ttl - time.Minute)}
	c.mu.Unlock()
	return out.AccessToken, nil
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// do sends a request and reads the JSON answer; Google's own words come
// back as the error when it refuses.
func (c *Client) do(req *http.Request, out any) error {
	res, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("Google could not be reached")
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error json.RawMessage `json:"error"`
			Desc  string          `json:"error_description"`
		}
		_ = json.Unmarshal(body, &e)
		var obj struct {
			Message string `json:"message"`
		}
		var str string
		_ = json.Unmarshal(e.Error, &obj)
		_ = json.Unmarshal(e.Error, &str)
		return errors.New(firstOf(obj.Message, e.Desc, str, res.Status))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c *Client) call(ctx context.Context, k *Key, method, path string, in, out any) error {
	tok, err := c.access(ctx, k)
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.API, "/")+path, body)
	req.Header.Set("Authorization", "Bearer "+tok)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out)
}

// Properties are the Search Console properties the key may read
// ("sc-domain:example.com" or "https://example.com/").
func (c *Client) Properties(ctx context.Context, k *Key) ([]string, error) {
	var out struct {
		SiteEntry []struct {
			SiteURL    string `json:"siteUrl"`
			Permission string `json:"permissionLevel"`
		} `json:"siteEntry"`
	}
	if err := c.call(ctx, k, http.MethodGet, "/sites", nil, &out); err != nil {
		return nil, err
	}
	list := []string{}
	for _, s := range out.SiteEntry {
		if s.Permission != "siteUnverifiedUser" {
			list = append(list, s.SiteURL)
		}
	}
	return list, nil
}

// PropertyFor picks the property of a domain from those the key may read:
// the whole domain first, then its https and http addresses.
func PropertyFor(domain string, have []string) string {
	domain = strings.TrimPrefix(strings.ToLower(domain), "www.")
	for _, want := range []string{"sc-domain:" + domain, "https://" + domain + "/", "https://www." + domain + "/", "http://" + domain + "/", "http://www." + domain + "/"} {
		for _, h := range have {
			if strings.EqualFold(h, want) {
				return h
			}
		}
	}
	return ""
}

// Query is what to ask for.
type Query struct {
	From, To string // YYYY-MM-DD, inclusive
	By       string // "query" (the words) or "" (only the totals)
	Limit    int
	PageRE   string // only pages whose address matches (RE2)
	Device   string // DESKTOP | MOBILE | TABLET
}

// Row is one search (or the totals): how often people clicked through,
// how often the site was shown, and the average place in the results.
type Row struct {
	Words       string  `json:"words,omitempty"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`      // percent
	Position    float64 `json:"position"` // 1 = first result
}

// Search asks Search Console about a property.
func (c *Client) Search(ctx context.Context, k *Key, property string, q Query) ([]Row, error) {
	type filter struct {
		Dimension  string `json:"dimension"`
		Operator   string `json:"operator"`
		Expression string `json:"expression"`
	}
	in := map[string]any{"startDate": q.From, "endDate": q.To, "rowLimit": 1}
	if q.By != "" {
		in["dimensions"] = []string{q.By}
		if q.Limit <= 0 || q.Limit > 1000 {
			q.Limit = 10
		}
		in["rowLimit"] = q.Limit
	}
	var fs []filter
	if q.PageRE != "" {
		fs = append(fs, filter{"page", "includingRegex", q.PageRE})
	}
	if q.Device != "" {
		fs = append(fs, filter{"device", "equals", q.Device})
	}
	if len(fs) > 0 {
		in["dimensionFilterGroups"] = []map[string]any{{"filters": fs}}
	}
	var out struct {
		Rows []struct {
			Keys        []string `json:"keys"`
			Clicks      float64  `json:"clicks"`
			Impressions float64  `json:"impressions"`
			CTR         float64  `json:"ctr"`
			Position    float64  `json:"position"`
		} `json:"rows"`
	}
	if err := c.call(ctx, k, http.MethodPost, "/sites/"+url.QueryEscape(property)+"/searchAnalytics/query", in, &out); err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(out.Rows))
	for _, r := range out.Rows {
		row := Row{Clicks: r.Clicks, Impressions: r.Impressions, CTR: float64(int(r.CTR*1000+0.5)) / 10, Position: float64(int(r.Position*10+0.5)) / 10}
		if len(r.Keys) > 0 {
			row.Words = r.Keys[0]
		}
		rows = append(rows, row)
	}
	return rows, nil
}
