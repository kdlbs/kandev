# agentctl API package notes

Scoped guidance for `apps/backend/internal/agentctl/server/api/`. Covers patterns that aren't obvious from reading the code in isolation.

## Reverse-proxy body rewriting (`port_proxy.go`)

The port proxy's `ModifyResponse` hook scans HTML responses to inject the inspector script (`html_injector.go`). For that scan to work, the body must arrive uncompressed.

**Don't decompress manually.** Instead, in the `Rewrite` hook, delete `Accept-Encoding` on the outbound request:

```go
r.Out.Header.Del("Accept-Encoding")
```

`net/http.Transport` then adds its own `Accept-Encoding: gzip` and auto-decompresses *before* `ModifyResponse` runs. Leaving the client's value intact (e.g. `gzip, br, deflate`) skips Transport's auto-decompression — the body arrives compressed and any byte scan corrupts it.

After rewriting the body in `injectScriptsIntoResponse`:

- `resp.Header.Del("Content-Encoding")`
- Set both `resp.Header["Content-Length"]` and `resp.ContentLength` (forgetting either causes chunked-encoding mismatches in the browser).

Tested in `port_proxy_test.go::TestCreatePortProxy_StripsAcceptEncodingAndInjectsHTML`.

## Iframe-blocking headers

`stripIframeSecurityHeaders` removes `Content-Security-Policy`, `Content-Security-Policy-Report-Only`, and `X-Frame-Options` from proxied HTML responses. This is scoped to HTML responses only (gated by `Content-Type` in `ModifyResponse`); other content types pass through with headers intact.
