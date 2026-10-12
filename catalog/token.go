package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// httpError is a non-200 answer from an API. Body holds the start of the reply,
// which usually says what was wrong.
type httpError struct {
	Status int
	URL    string
	Body   string
}

func (e *httpError) Error() string {
	msg := fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}

	return msg
}

// tokenSource caches an OAuth client-credentials access token.
type tokenSource struct {
	client       *http.Client
	url          string
	clientID     string
	clientSecret string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (t *tokenSource) get(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Now().Add(30*time.Second).Before(t.expires) {
		return t.token, nil
	}

	form := url.Values{"grant_type": {"client_credentials"}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(t.clientID, t.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"token request: HTTP %d (check the client ID and secret): %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("token request: unexpected response")
	}

	t.token = tok.AccessToken
	t.expires = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)

	return t.token, nil
}

func (t *tokenSource) invalidate() {
	t.mu.Lock()
	t.token = ""
	t.mu.Unlock()
}

// getJSON performs an authorized GET and decodes the JSON body. A 401 refreshes
// the token once and retries.
func (t *tokenSource) getJSON(ctx context.Context, rawURL, accept string, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := t.get(ctx)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+tok)

		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		resp, err := t.client.Do(req)
		if err != nil {
			return err
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			t.invalidate()
			continue
		case resp.StatusCode == http.StatusTooManyRequests:
			return fmt.Errorf("rate limited by the service; retry in %ss", resp.Header.Get("Retry-After"))
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("artist not found")
		case resp.StatusCode != http.StatusOK:
			text := strings.TrimSpace(string(body))
			if len(text) > 200 {
				text = text[:200]
			}

			return &httpError{Status: resp.StatusCode, URL: rawURL, Body: text}
		}

		return json.Unmarshal(body, out)
	}
}
