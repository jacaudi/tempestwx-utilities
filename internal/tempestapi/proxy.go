package tempestapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Proxy performs an authenticated GET against path (resolved against the
// client's base URL) with query attached, injecting the server-held token as
// `Authorization: Bearer` -- the one generic authenticated request every
// httpserver proxy handler (station/forecast/almanac) needs, so it is
// written once here rather than once per handler.
//
// Proxy passes the upstream status and body through unchanged on a
// successful round trip -- even a non-200 upstream response (rate limit,
// upstream error) is not turned into a Go error, since the caller's job is
// to relay WeatherFlow's response, not to reinterpret it. err is non-nil
// only for a failure to build or execute the request, or to read its body.
func (c *Client) Proxy(ctx context.Context, path string, query url.Values) (body []byte, contentType string, status int, err error) {
	reqURL := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("build weatherflow proxy request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", 0, scrubTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", 0, fmt.Errorf("read weatherflow proxy response: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}

	return respBody, ct, resp.StatusCode, nil
}

// scrubTransportError returns a token-free, URL-free error for a transport
// failure. The Bearer token itself is never part of the request URL (it
// travels only in the Authorization header), but a failed *http.Client.Do
// commonly returns a *url.Error whose Error() string embeds the full
// request URL -- including any query values. Per design §15, that URL (and,
// transitively, anything it could carry) is scrubbed defensively: a
// *url.Error is replaced with a fully generic message rather than wrapped,
// so neither the URL nor the underlying dial/TLS error text (which can
// itself echo the dialed address) can reach a log or an HTTP response.
func scrubTransportError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return errors.New("weatherflow proxy request failed: upstream unreachable")
	}
	return fmt.Errorf("weatherflow proxy request failed: %w", err)
}
