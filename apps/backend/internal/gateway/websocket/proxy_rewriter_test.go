package websocket

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const proxyPrefix = "/port-proxy/abc/3001"

func TestRewriteAbsolutePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"path-absolute", "/foo/bar.js", proxyPrefix + "/foo/bar.js"},
		{"root", "/", proxyPrefix + "/"},
		{"network-relative skipped", "//cdn.example.com/x", "//cdn.example.com/x"},
		{"absolute http skipped", "http://example.com/x", "http://example.com/x"},
		{"relative skipped", "foo.js", "foo.js"},
		{"dot-relative skipped", "./foo.js", "./foo.js"},
		{"parent-relative skipped", "../foo.js", "../foo.js"},
		{"data-uri skipped", "data:image/png;base64,xyz", "data:image/png;base64,xyz"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewriteAbsolutePath(c.in, proxyPrefix, "")
			if got != c.want {
				t.Fatalf("rewriteAbsolutePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRewriteHTMLURLs(t *testing.T) {
	in := `<!DOCTYPE html>
<html>
<head>
<link rel="stylesheet" href="/styles/main.css">
<link rel="stylesheet" href="//cdn.example.com/lib.css">
<script src="/static/app.js"></script>
<style>body { background: url(/img/bg.png); }</style>
</head>
<body>
<img src="/img/logo.png" srcset="/img/logo@2x.png 2x, https://cdn.example.com/logo.png 3x">
<a href="/about">About</a>
<a href="http://external.example.com/x">External</a>
<form action="/submit"><input formaction="/quick"></form>
<div style="background: url('/bg.jpg');"></div>
</body>
</html>`

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	mustContain(t, got, `href="/port-proxy/abc/3001/styles/main.css"`)
	mustContain(t, got, `href="//cdn.example.com/lib.css"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/static/app.js"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/img/logo.png"`)
	mustContain(t, got, `/port-proxy/abc/3001/img/logo@2x.png 2x`)
	mustContain(t, got, `https://cdn.example.com/logo.png 3x`)
	mustContain(t, got, `href="/port-proxy/abc/3001/about"`)
	mustContain(t, got, `href="http://external.example.com/x"`)
	mustContain(t, got, `action="/port-proxy/abc/3001/submit"`)
	mustContain(t, got, `formaction="/port-proxy/abc/3001/quick"`)
	// Inline style="url('/bg.jpg')" — html package HTML-escapes single quotes
	// on serialization, so check for the rewritten path without the quote.
	mustContain(t, got, `/port-proxy/abc/3001/bg.jpg`)
	// Inline <style> block should be rewritten via rewriteCSSFragment.
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
}

func TestRewriteCSSURLs(t *testing.T) {
	in := `@import "/theme.css";
@import url("/print.css");
.bg { background: url(/img/bg.png) no-repeat; }
.cdn { background: url("//cdn.example.com/x.png"); }
.abs { background: url(http://example.com/x.png); }
.rel { background: url(foo.png); }`

	got := string(rewriteCSSURLsAt([]byte(in), proxyPrefix, "", proxyPrefix+"/"))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/print.css")`)
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
	mustContain(t, got, `url("//cdn.example.com/x.png")`)
	mustContain(t, got, `url(http://example.com/x.png)`)
	mustContain(t, got, `url(foo.png)`)
}

func TestRewriteProxyResponse_HTML(t *testing.T) {
	body := `<a href="/x">x</a>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `href="/port-proxy/abc/3001/x"`) {
		t.Fatalf("HTML not rewritten: %q", got)
	}
	if resp.ContentLength != int64(len(got)) {
		t.Fatalf("ContentLength mismatch: %d vs %d", resp.ContentLength, len(got))
	}
}

func TestRewriteProxyResponse_CSS(t *testing.T) {
	body := `body { background: url(/bg.png); }`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/css"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `url(/port-proxy/abc/3001/bg.png)`) {
		t.Fatalf("CSS not rewritten: %q", got)
	}
}

func TestRewriteProxyResponse_OtherContentTypeUnchanged(t *testing.T) {
	body := `{"href":"/foo"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("non-HTML/CSS response was modified: %q", got)
	}
}

func TestRewriteHTMLURLs_InjectsRuntimeShim(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	// The shim script tag must appear exactly once, immediately after the
	// `<head>` open tag (so it executes before any other script that may
	// follow), and load from the proxy's reserved same-origin path.
	const marker = `<script src="/port-proxy/abc/3001/__kandev_runtime_shim.js"></script>`
	if strings.Count(got, marker) != 1 {
		t.Fatalf("expected exactly one runtime shim tag, got %d copies\n%s",
			strings.Count(got, marker), got)
	}
	headIdx := strings.Index(got, "<head>")
	titleIdx := strings.Index(got, "<title>")
	shimIdx := strings.Index(got, marker)
	if headIdx >= shimIdx || shimIdx >= titleIdx {
		t.Fatalf("shim must come between <head> and <title>: head=%d shim=%d title=%d\n%s",
			headIdx, shimIdx, titleIdx, got)
	}
	// With a capability minted, the shim src carries it so the shim loads even
	// in cookie-less contexts.
	withCap := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-shim", ""))
	if !strings.Contains(withCap, `__kandev_runtime_shim.js?kandev_cap=cap-shim`) {
		t.Fatalf("capability-bearing shim src missing the capability:\n%s", withCap)
	}
	// The prefix must be baked into the served shim body.
	shim := runtimeShim(proxyPrefix, "")
	if !strings.Contains(shim, `var P="/port-proxy/abc/3001";`) {
		t.Fatalf("shim body missing the baked-in prefix:\n%s", shim)
	}
}

func TestRewriteHTMLURLs_NoHeadStillRewritesURLs(t *testing.T) {
	// Documents without a <head> are rare but possible. We don't bother
	// injecting the shim in that case (no good anchor point), but URL
	// rewriting must still work.
	in := `<a href="/foo">x</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))
	if !strings.Contains(got, `href="/port-proxy/abc/3001/foo"`) {
		t.Fatalf("URL not rewritten: %q", got)
	}
	if strings.Contains(got, "window.fetch=") {
		t.Fatalf("unexpected shim in headless document: %q", got)
	}
}

func TestRewriteHTMLURLs_PreservesScriptContentVerbatim(t *testing.T) {
	// Inline scripts must not be HTML-escaped — `&`, `<`, `>` are valid JS
	// tokens (bitwise/logical operators, comparisons, JSON characters in
	// embedded payloads, etc.) and escaping them corrupts the JS.
	in := `<!DOCTYPE html><html><head></head><body>` +
		`<script>var a = 1 & 2; var b = a && true; var c = "<x>"; var d = {"k":"&"};</script>` +
		`<script src="/static/app.js"></script>` +
		`</body></html>`

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "", ""))

	// Inline script body must come through unescaped.
	for _, needle := range []string{
		`var a = 1 & 2;`,
		`var b = a && true;`,
		`var c = "<x>";`,
		`var d = {"k":"&"};`,
	} {
		mustContain(t, got, needle)
	}

	// External script `src` is still rewritten.
	mustContain(t, got, `src="/port-proxy/abc/3001/static/app.js"`)

	// Sanity: none of the inline-script characters got HTML-escaped.
	for _, forbidden := range []string{`&amp;`, `&lt;x&gt;`} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("script body was HTML-escaped (%q present):\n%s", forbidden, got)
		}
	}
}

func TestRuntimeShim_InstallsMutationObserver(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// MutationObserver is installed so dynamically-inserted DOM nodes (e.g.
	// Next.js `ReactDOM.preload()` for fonts) get their URL attributes
	// rewritten too, not just whatever was in the initial HTML.
	mustContain(t, shim, `new MO(function(rs)`)
	mustContain(t, shim, `.observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:OBS})`)

	// The attribute list mirrors the static HTML rewriter's coverage so the
	// runtime path doesn't silently miss attributes the static path catches.
	mustContain(t, shim, `'href','src','action','formaction','cite','data','poster','background','manifest','srcset'`)

	// rel changes are observed so link classification (metadata vs fetching)
	// stays current; style and meta content are observed and runtime-rewritten;
	// srcdoc is intentionally not observed (static-only, documented).
	mustContain(t, shim, `'rel','style','content','http-equiv'`)
	if strings.Contains(shim, "'srcdoc']") {
		t.Fatal("srcdoc must not be observed (static-only contract)")
	}

	// srcset has its own splitter (whitespace-separated descriptors).
	mustContain(t, shim, `if(a==='srcset'||a==='imagesrcset')`)
}

func TestRuntimeShim_ExposesProxyPrefixToInspector(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// The inspector script uses this to report app-local routes in annotation
	// prompts instead of the gateway's /port-proxy/... path.
	mustContain(t, shim, `window.__kandevProxyPrefix=P;`)
}

func TestRuntimeShim_ForwardsConsoleToParent(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// Console levels are intercepted so iframe diagnostics surface in the
	// parent window's console alongside other preview events.
	mustContain(t, shim, `var LV=['log','warn','error','info','debug'];`)
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}}`)

	// Original method is still invoked so the iframe's own DevTools shows
	// the same output.
	mustContain(t, shim, `return orig.apply(console,arguments)`)
}

func TestRuntimeShim_BindsConsoleToParentOrigin(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-console")

	// The child announces readiness without sending console data. The parent
	// replies with its origin, and the child uses that origin for console data.
	mustContain(t, shim, "console-ready")
	mustContain(t, shim, "console-bind")
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console-ready',payload:{}},'*')`)
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}},PO)`)
}

func TestRuntimeShim_PatchesNavigationAPIs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")

	// Patches history.pushState and history.replaceState so SPA routers keep
	// the proxy prefix in the URL bar on client-side navigation.
	mustContain(t, shim, `'pushState','replaceState'`)
	mustContain(t, shim, `history[op]=function(s,t,u)`)

	// Patches location.assign and location.replace so imperative navigation
	// goes through the same rewriter.
	mustContain(t, shim, `'assign','replace'`)
	mustContain(t, shim, `location[op]=function(u)`)

	// Both patches must reuse the prefix-only navigation rewriter rn() rather
	// than rolling their own prefix logic — and must NOT carry the capability
	// (the subtree cookie covers same-origin navigations; a bearer in the
	// address bar/history would leak).
	for _, needle := range []string{
		`history[op]=function(s,t,u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(this,s,t,u)}`, // history APIs (any DOMString-able arg)
		`location[op]=function(u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(location,u)}`,    // location APIs
	} {
		mustContain(t, shim, needle)
	}
}

func TestRewriteSrcSet(t *testing.T) {
	in := "/a.png 1x, /b.png 2x, //cdn.example.com/c.png 3x"
	got := rewriteSrcSet(in, proxyPrefix, "", proxyPrefix+"/")
	want := "/port-proxy/abc/3001/a.png 1x, /port-proxy/abc/3001/b.png 2x, //cdn.example.com/c.png 3x"
	if got != want {
		t.Fatalf("rewriteSrcSet = %q, want %q", got, want)
	}
}

// The subtree capability must ride on every rewritten SUBRESOURCE URL: plain
// URLs get it as the first query parameter, URLs that already carry a query
// get it appended. Network-relative and absolute URLs stay untouched, and
// navigation references (anchor href) get the prefix without the capability.
func TestRewriteHTMLURLs_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	in := `<a href="/foo">x</a>` +
		`<script type="module" src="/src/main.tsx?t=123"></script>` +
		`<img srcset="/a.png 1x, //cdn.example.com/b.png 2x">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<a href="/docs#installation">docs</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-123", ""))

	// Navigation hrefs are prefixed but capability-free.
	mustContain(t, got, `href="/port-proxy/abc/3001/foo"`)
	// The tokenizer HTML-escapes the separator inside the attribute value.
	mustContain(t, got, `src="/port-proxy/abc/3001/src/main.tsx?t=123&amp;kandev_cap=cap-123"`)
	mustContain(t, got, `href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-123"`)
	mustContain(t, got, `srcset="/port-proxy/abc/3001/a.png?kandev_cap=cap-123 1x, //cdn.example.com/b.png 2x"`)
	// A fragment must stay after the capability query on subresources.
	mustContain(t, got, `href="/port-proxy/abc/3001/docs#installation"`)
}

// The capability is appended to rewritten CSS url() and @import references too,
// so cookie-less CSS loads stay authorized in the same contexts as the
// manifest fetch. Relative references get the capability appended as well;
// scheme-bearing and network-relative references stay untouched.
func TestRewriteCSSURLs_AppendsCapability(t *testing.T) {
	in := `@import "/theme.css"; .x { background: url("/img/bg.png"); }` +
		`.y { background: url(rel.png); } @import "print.css";` +
		`.z { background: url(http://cdn.example.com/x.png); }`
	got := string(rewriteCSSURLsAt([]byte(in), proxyPrefix, "cap-456", proxyPrefix+"/"))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css?kandev_cap=cap-456"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/img/bg.png?kandev_cap=cap-456")`)
	mustContain(t, got, `url(rel.png?kandev_cap=cap-456)`)
	mustContain(t, got, `@import "print.css?kandev_cap=cap-456"`)
	mustContain(t, got, `url(http://cdn.example.com/x.png)`)
}

// hasURLScheme follows RFC 3986 scheme grammar: a scheme starts with an ASCII
// letter and continues with letters, digits, +, -, or . until ":". A colon in
// a non-scheme position (digit first, or after a path/query delimiter) means
// the reference is relative.
func TestHasURLScheme(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"http://x", true},
		{"https://x", true},
		{"data:image/png;base64,AAA", true},
		{"javascript:alert(1)", true},
		{"mailto:x@y.dev", true},
		{"v1:chunk.js", true}, // RFC-valid scheme
		{"C:foo", true},
		{"foo", false},
		{"./foo", false},
		{"../foo", false},
		{"foo/bar:baz", false}, // colon after a path delimiter
		{"1x:y", false},        // scheme must start with a letter
		{"?a=b", false},
		{"#frag", false},
	}
	for _, tc := range cases {
		if got := hasURLScheme(tc.raw); got != tc.want {
			t.Errorf("hasURLScheme(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// stripCapabilityParam removes only the capability pair from a raw query,
// matching percent-encoded spellings and preserving every other parameter's
// bytes and order.
func TestStripCapabilityParam(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"absent", "a=1&b=2", "a=1&b=2"},
		{"plain", "a=1&kandev_cap=x&b=2", "a=1&b=2"},
		{"first", "kandev_cap=x&a=1", "a=1"},
		{"last", "a=1&kandev_cap=x", "a=1"},
		{"encoded key", "a=1&%6bandev_cap=x", "a=1"},
		{"encoded key full", "a=1&%6B%61%6E%64%65%76%5F%63%61%70=x", "a=1"},
		{"no value", "kandev_cap", ""},
		{"preserves order and bytes", "b=2&a=%2F&kandev_cap=x&c=3", "b=2&a=%2F&c=3"},
		{"value contains cap name", "a=kandev_cap=x", "a=kandev_cap=x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripCapabilityParam(tc.in); got != tc.want {
				t.Fatalf("stripCapabilityParam(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// stripReservedProxyParams additionally removes the ?token= PAT credential the
// gateway consumes, in any encoding, while preserving everything else.
func TestStripReservedProxyParams(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"absent", "a=1&b=2", "a=1&b=2"},
		{"cap", "a=1&kandev_cap=x", "a=1"},
		{"token", "a=1&token=pat-secret&b=2", "a=1&b=2"},
		{"encoded token", "a=1&%74oken=pat&b=2", "a=1&b=2"},
		{"both", "token=pat&a=1&kandev_cap=x", "a=1"},
		{"value contains names", "a=kandev_cap=x&b=token=y", "a=kandev_cap=x&b=token=y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripReservedProxyParams(tc.in); got != tc.want {
				t.Fatalf("stripReservedProxyParams(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Same-origin relative references (a relative <link rel="manifest">, relative
// srcset entries) resolve inside the proxy subtree but carry no cookie on
// fetch; they must still get the capability appended. Scheme-bearing,
// network-relative, and fragment-only values stay untouched.
func TestRewriteHTMLURLs_AppendsCapabilityToRelativeReferences(t *testing.T) {
	in := `<link rel="manifest" href="manifest.webmanifest">` +
		`<a href="page">x</a>` +
		`<img srcset="a.png 1x, ./b.png 2x">` +
		`<a href="javascript:alert(1)">js</a>` +
		`<a href="mailto:x@y.dev">mail</a>` +
		`<a href="//cdn.example.com/lib.js">cdn</a>` +
		`<a href="#section">frag</a>` +
		`<img src="data:image/png;base64,AAA">` +
		`<a href=" https://evil.example/x">spaced-scheme</a>` +
		`<a href="&#10;//evil.example/y">newline-netrel</a>` +
		`<a href=" /rooted">spaced-root</a>` +
		`<form action="/submit"><button formaction="/alt"></button></form>` +
		`<a href="ht&#10;tps://evil2.example/z">embedded-tab-scheme</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-789", ""))

	mustContain(t, got, `href="manifest.webmanifest?kandev_cap=cap-789"`)
	// Navigation references: prefixed, capability-free.
	mustContain(t, got, `<a href="page">x</a>`)
	mustContain(t, got, `action="/port-proxy/abc/3001/submit"`)
	mustContain(t, got, `formaction="/port-proxy/abc/3001/alt"`)
	mustContain(t, got, `srcset="a.png?kandev_cap=cap-789 1x, ./b.png?kandev_cap=cap-789 2x"`)
	mustContain(t, got, `href="javascript:alert(1)"`)
	mustContain(t, got, `href="mailto:x@y.dev"`)
	mustContain(t, got, `href="//cdn.example.com/lib.js"`)
	mustContain(t, got, `href="#section"`)
	mustContain(t, got, `src="data:image/png;base64,AAA"`)
	// Leading whitespace must not make an external URL look relative: the
	// browser trims it and goes external, so the capability must not be
	// appended.
	mustContain(t, got, `href=" https://evil.example/x"`)
	// The tokenizer decodes &#10; to a literal newline; the network-relative
	// URL behind it must still be left untouched.
	mustContain(t, got, "href=\"\n//evil.example/y\"")
	// A spaced root-absolute anchor reference is rewritten from its trimmed
	// form, capability-free (navigation).
	mustContain(t, got, `href="/port-proxy/abc/3001/rooted"`)
	// Embedded tab/newline inside the scheme: WHATWG normalization reveals an
	// external https URL, so the capability must not be appended.
	mustContain(t, got, "href=\"ht\ntps://evil2.example/z\"")
}

// The runtime shim appends the capability to every URL its network path
// rewriter produces: fragments stay last, existing query strings are preserved,
// and the append is skipped only for an exact kandev_cap query key. WebSocket
// URLs (string and same-origin URL-object inputs) go through the same rewriter.
// Navigation APIs (history/location) use a prefix-only rewriter so the bearer
// never lands in the address bar, history, or cross-origin Referers.
func TestRuntimeShim_AppendsCapabilityToRewrittenURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `var K="cap-shim";`)
	// Fragment split: the query lands before any #fragment.
	mustContain(t, shim, `var fh=u.indexOf('#');if(fh!==-1){fr=u.slice(fh);u=u.slice(0,fh)}`)
	// The append is unconditional: an app's own kandev_cap parameter must not
	// suppress the issued capability (the gateway accepts any valid value
	// among duplicates).
	mustContain(t, shim, `u+=(u.indexOf('?')===-1?'?':'&')+"kandev_cap="+K`)
	if strings.Contains(shim, `if(!/([?&])kandev_cap=/.test(x))`) {
		t.Fatal("the shim must not let an app kandev_cap key suppress the issued capability")
	}
	// WebSocket wrapper rewrites string AND URL-object inputs through norm(),
	// which carries the capability; same-origin ws/wss origins are compared
	// as http/https via the URL API and the scheme is swapped for the
	// matching ws/wss form.
	mustContain(t, shim, `'//'+l.host`)
	mustContain(t, shim, `s.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i,w)`)
	mustContain(t, shim, `x.origin.replace(/^ws:/,'http:').replace(/^wss:/,'https:')`)
	mustContain(t, shim, `new URL(s,(typeof document!=='undefined'&&document.baseURI)||window.location.href)`)
	// fetch/XHR/WS all route URL-like inputs through norm() (URL objects,
	// same-origin absolute strings).
	mustContain(t, shim, `Reflect.get(URL.prototype,'href',i)`)
	mustContain(t, shim, `arguments[1]=u;return oo.apply(this,arguments)`)
	// Navigation APIs use the prefix-only rewriter, never the capability.
	mustContain(t, shim, `if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(this,s,t,u)`)
	mustContain(t, shim, `if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(location,u)`)
	mustContain(t, shim, `function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#')return u;if(u.indexOf('//')===0)return u;try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}u=nz(u);if(u.charAt(0)==='/'){u=sc(u);if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u;if(u.indexOf('/.')!==-1){var dd2=rd(u);if(!(dd2===P||dd2.indexOf(P+'/')===0||dd2.indexOf(P+'?')===0||dd2.indexOf(P+'#')===0))return u;u=dd2}return u}if(/^[a-z][a-z0-9+.-]*:\/\//i.test(u)){var o=ru.origin;var pn=ru.pathname;var tail=sc(pn+(ru.search||''));if(!(tail===P||tail.indexOf(P+'/')===0||tail.indexOf(P+'?')===0||tail.indexOf(P+'#')===0))tail=P+tail;return o+tail+(ru.hash||'')}return sc(u)}`)
	// Anchor navigation needs no click interception: the MutationObserver
	// prefixes hrefs, so the browser's own default navigation stays inside
	// the subtree and app click handlers keep control.
	if strings.Contains(shim, "addEventListener('click'") {
		t.Fatal("click interception must not be installed (it breaks app delegation)")
	}
	// norm() rewrites only http/https/ws/wss inputs; other schemes pass through.
	mustContain(t, shim, `var p=x.protocol;if(p!=='http:'&&p!=='https:'&&p!=='ws:'&&p!=='wss:')return u;`)
	// MutationObserver distinguishes navigation attributes (rn, incl. metadata
	// links with no fetching rel token via lf()) from subresource attributes
	// (r), and scans the full attribute set on inserted subtrees.
	mustContain(t, shim, `el.tagName==='A'||el.tagName==='AREA'||el.tagName==='BASE'||(el.tagName==='LINK'&&!lf(el))`)
	mustContain(t, shim, `function lf(el){var rel=el.rel;if(typeof rel!=='string')return true;var toks=rel.toLowerCase().split(/\s+/);`)
	mustContain(t, shim, `var rr=nav?rn:r;`)
	mustContain(t, shim, `'[href],[src],[action],[formaction],[cite],[data],[poster],[background],[manifest],[srcset],[imagesrcset],[ping],[style],[content]'`)
}

// The navigation rewriter must not carry the capability even when one is
// minted: the bearer stays out of the address bar and browser history.
func TestRuntimeShim_NavigationRewriterOmitsCapability(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// rn() is the prefix-only form: it strips any previously issued capability
	// and never appends a new one.
	mustContain(t, shim, `function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#')return u;if(u.indexOf('//')===0)return u;try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}u=nz(u);if(u.charAt(0)==='/'){u=sc(u);if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u;if(u.indexOf('/.')!==-1){var dd2=rd(u);if(!(dd2===P||dd2.indexOf(P+'/')===0||dd2.indexOf(P+'?')===0||dd2.indexOf(P+'#')===0))return u;u=dd2}return u}if(/^[a-z][a-z0-9+.-]*:\/\//i.test(u)){var o=ru.origin;var pn=ru.pathname;var tail=sc(pn+(ru.search||''));if(!(tail===P||tail.indexOf(P+'/')===0||tail.indexOf(P+'?')===0||tail.indexOf(P+'#')===0))tail=P+tail;return o+tail+(ru.hash||'')}return sc(u)}`)
	// history and location rewrite through rn(), never r().
	mustContain(t, shim, `history[op]=function(s,t,u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(this,s,t,u)}`)
	mustContain(t, shim, `location[op]=function(u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(location,u)}`)
	if strings.Contains(shim, `history[op]=function(s,t,u){if(typeof u==='string')u=r(u)`) {
		t.Fatal("history APIs must not use the capability-bearing rewriter")
	}
}

// The auth-off contract is "no ISSUED capability": with an empty capability
// the shim carries zero issued-capability logic — no capability constant, no
// exact-match helper, no query append. The unconditional sc()/rn() still
// reference the reserved KEY NAME (d!=='kandev_cap') because stripping a
// reserved-named parameter from navigation URLs is the gateway's documented
// behavior in every mode; the contract is that no capability is minted,
// embedded, or appended when auth is off.
func TestRuntimeShim_NoIssuedCapabilityLogic(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "")
	if strings.Contains(shim, `"kandev_cap="+K`) {
		t.Fatalf("shim without capability appends kandev_cap:\n%s", shim)
	}
	if strings.Contains(shim, "function hcp(") {
		t.Fatalf("shim without capability carries the exact-match helper:\n%s", shim)
	}
	if strings.Contains(shim, `var K=`) {
		t.Fatalf("shim without capability carries an issued-capability constant:\n%s", shim)
	}
}

// Bodyless responses (1xx/204/304) must pass through untouched: no body
// rewrite, no synthesized Content-Length, no capability query rewriting.
func TestRewriteProxyResponse_LeavesBodylessResponsesUntouched(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified, http.StatusContinue} {
		resp := &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp.Header.Set("Content-Length", "100") // 304: describes the selected representation
		if err := rewriteProxyResponse(resp, proxyPrefix, "cap-304"); err != nil {
			t.Fatalf("status %d: unexpected error: %v", status, err)
		}
		if got := resp.Header.Get("Content-Length"); got != "100" {
			t.Fatalf("status %d: Content-Length = %q, want original %q", status, got, "100")
		}
	}
}

// Capability-bearing rewritten responses must be uncacheable (per-user body +
// Set-Cookie) and must not leak the embedded capability through the Referer
// header to external origins. The headers are applied by the gateway's
// ModifyResponse for every response class (JS/JSON/redirects included), which
// is covered by the end-to-end proxy test; the rewriter itself performs the
// body rewrite only.
func TestRewriteProxyResponse_DoesNotSetCacheHeadersItself(t *testing.T) {
	body := `<!doctype html><html><head><link rel="manifest" href="/manifest.webmanifest"></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	if err := rewriteProxyResponse(resp, proxyPrefix, "cap-cache"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Header.Get("Cache-Control") != "" || resp.Header.Get("Referrer-Policy") != "" {
		t.Fatalf("cache/referrer headers must be applied by ModifyResponse, not the rewriter: %v", resp.Header)
	}
}

// Inline iframe documents (srcdoc) inherit the proxy origin; their
// root-absolute references must be rewritten like a nested page, with the
// capability on subresources and none on navigation.
func TestRewriteHTMLURLs_RewritesSrcdocDocuments(t *testing.T) {
	in := `<iframe srcdoc="&lt;a href=&quot;/page&quot;&gt;x&lt;/a&gt;&lt;img src=&quot;/logo.png&quot;&gt;"></iframe>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-srcdoc", ""))
	// The nested document is rewritten (navigation no cap, subresource cap);
	// the serializer re-escapes the srcdoc attribute value.
	mustContain(t, got, `srcdoc="&lt;a href=&#34;/port-proxy/abc/3001/page&#34;&gt;x&lt;/a&gt;&lt;img src=&#34;/port-proxy/abc/3001/logo.png?kandev_cap=cap-srcdoc&#34;&gt;"`)
}

// Meta refresh navigation targets must be re-anchored on the proxy subtree
// without a capability.
func TestRewriteHTMLURLs_RewritesMetaRefresh(t *testing.T) {
	in := `<meta http-equiv="refresh" content="5; url=/next">` +
		`<meta http-equiv="refresh" content="0;url='https://external.example/x'">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-meta", ""))

	mustContain(t, got, `content="5; url=/port-proxy/abc/3001/next"`)
	mustContain(t, got, `content="0;url=&#39;https://external.example/x&#39;"`)
}

// Metadata links (rel=canonical and other non-fetching rels) must not carry
// the capability; fetching rels (stylesheet, manifest, preload, …) keep it.
func TestRewriteHTMLURLs_CanonicalLinkOmitsCapability(t *testing.T) {
	in := `<link rel="canonical" href="/canonical">` +
		`<link rel="stylesheet" href="/theme.css">` +
		`<link rel="manifest" href="/manifest.webmanifest">` +
		`<link rel="alternate" href="/feed.xml">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-link", ""))

	mustContain(t, got, `rel="canonical" href="/port-proxy/abc/3001/canonical"`)
	mustContain(t, got, `rel="alternate" href="/port-proxy/abc/3001/feed.xml"`)
	mustContain(t, got, `rel="stylesheet" href="/port-proxy/abc/3001/theme.css?kandev_cap=cap-link"`)
	mustContain(t, got, `rel="manifest" href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-link"`)
}

// When the app's Content-Security-Policy uses a script nonce, the injected
// shim tag must carry it so nonce- and strict-dynamic-based policies allow the
// shim; the meta-tag form is honored too.
func TestRewriteHTMLURLs_ShimTagCarriesCSPNonce(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	// Header-based policy.
	headerResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {"default-src 'self'; script-src 'self' 'nonce-app123'"},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(headerResp, proxyPrefix, "cap-nonce"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(headerResp.Body)
	mustContain(t, string(got), `<script src="/port-proxy/abc/3001/__kandev_runtime_shim.js?kandev_cap=cap-nonce" nonce="app123"></script>`)

	// Meta-tag policy.
	metaResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html"}},
		Body: io.NopCloser(strings.NewReader(
			`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="script-src 'self' 'nonce-meta456'"><title>x</title></head><body></body></html>`)),
	}
	if err := rewriteProxyResponse(metaResp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotMeta, _ := io.ReadAll(metaResp.Body)
	mustContain(t, string(gotMeta), `nonce="meta456"`)

	// No nonce in the policy: the tag stays plain.
	plain := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {"default-src 'self'"},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(plain, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotPlain, _ := io.ReadAll(plain.Body)
	if strings.Contains(string(gotPlain), "nonce=") {
		t.Fatalf("shim tag must not carry a nonce when the policy has none:\n%s", gotPlain)
	}

	// A malformed nonce-shaped token from a hostile policy must not be
	// interpolated into the injected markup.
	hostile := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'self' 'nonce-x"><script>alert(1)</script>'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(hostile, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotHostile, _ := io.ReadAll(hostile.Body)
	if strings.Contains(string(gotHostile), `<script>alert(1)`) {
		t.Fatalf("hostile nonce injected markup:\n%s", gotHostile)
	}
	if strings.Contains(string(gotHostile), `nonce="x"`) {
		t.Fatalf("malformed nonce accepted into the shim tag:\n%s", gotHostile)
	}
}

// CSP meta tags are scanned in any attribute order, and every meta is examined
// so a nonce-free policy cannot hide a later nonce-bearing one.
func TestRewriteHTMLURLs_CSPMetaAnyAttributeOrder(t *testing.T) {
	in := `<!DOCTYPE html><html><head><meta content="script-src 'self'" http-equiv="Content-Security-Policy"><meta http-equiv="Content-Security-Policy" content="script-src 'nonce-later99'"><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html"}},
		Body:       io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	mustContain(t, string(got), `nonce="later99"`)
}

// Console forwarding binds to the embedding parent's origin. Only the
// data-free readiness message uses a wildcard target.
func TestRuntimeShim_ConsoleForwardingTargetsBoundParent(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-console")
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console-ready',payload:{}},'*')`)
	mustContain(t, shim, `window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}},PO)`)
	if strings.Contains(shim, `type:'console',payload:{level:lv,args:out}},window.location.origin`) {
		t.Fatal("console forwarding must not assume the gateway origin is the parent origin")
	}
}

// Meta refresh targets: quoted URLs with embedded semicolons or spaces must be
// preserved; unquoted targets stop at the delimiter.
func TestRewriteMetaRefresh_QuoteAndSemicolonAware(t *testing.T) {
	in := `<meta http-equiv="refresh" content="0; url='/next;v=1'">` +
		`<meta http-equiv="refresh" content="3; url=/plain">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-mq", ""))

	mustContain(t, got, `content="0; url=&#39;/port-proxy/abc/3001/next;v=1&#39;"`)
	mustContain(t, got, `content="3; url=/port-proxy/abc/3001/plain"`)
}

// srcdoc recursion is depth-bounded even through ordinary start tags: at the
// bound the nested value is preserved unchanged.
func TestRewriteHTMLURLs_SrcdocDepthBoundAppliesToStartTags(t *testing.T) {
	in := `<iframe srcdoc="<img src=&quot;/logo.png&quot;>"></iframe>`
	// At the bound, rewriteHTMLURLsAtDepth must leave the srcdoc untouched.
	atBound := string(rewriteHTMLURLsAtDepth([]byte(in), proxyPrefix, "cap-d", "", maxSRCDocDepth))
	mustContain(t, atBound, `srcdoc="&lt;img src=&#34;/logo.png&#34;&gt;"`)
	// A chain of ordinary start-tag iframes nested past the bound completes
	// without unbounded recursion: every level is rewritten while depth is
	// below the bound, and the innermost content is preserved at it.
	inner := `<img src="/deep.png">`
	for i := 0; i < maxSRCDocDepth+2; i++ {
		inner = `<iframe srcdoc="` + html.EscapeString(inner) + `">`
	}
	got := string(rewriteHTMLURLs([]byte(inner), proxyPrefix, "cap-d", ""))
	if !strings.Contains(got, "/deep.png") {
		t.Fatalf("deep srcdoc chain lost the innermost content:\n%s", got)
	}
	if strings.Count(got, "kandev_cap=") > maxSRCDocDepth {
		t.Fatalf("deep srcdoc chain exceeded the depth bound:\n%s", got)
	}
}

// Entity-encoded CSP meta policies are decoded by the tokenizer before nonce
// extraction; decoy attribute keys (data-content) do not match.
func TestRewriteHTMLURLs_CSPMetaEntityEncodedNonce(t *testing.T) {
	in := `<!DOCTYPE html><html><head><meta data-content="x" http-equiv="Content-Security-Policy" content="script-src &#39;self&#39; &#39;nonce-ent123&#39;"><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html"}},
		Body:       io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	mustContain(t, string(got), `nonce="ent123"`)
}

// Meta refresh targets are recognized only at a field boundary: unrelated
// text containing "url=" must not be rewritten.
func TestRewriteMetaRefresh_OnlyAtFieldBoundary(t *testing.T) {
	in := `<meta http-equiv="refresh" content="0; noturl=/evil">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nb", ""))
	mustContain(t, got, `content="0; noturl=/evil"`)
}

// CSP nonces must contain at least one non-padding character, and padding
// must be trailing only.
func TestValidCSPNonce(t *testing.T) {
	for _, bad := range []string{"", "=", "==", "a b", "a\"b", "ab=c", "a=b", "=ab", "a==b"} {
		if validCSPNonce(bad) {
			t.Errorf("validCSPNonce(%q) = true, want false", bad)
		}
	}
	for _, good := range []string{"abc123", "a_b-c", "ab+/", "abc=", "x=="} {
		if !validCSPNonce(good) {
			t.Errorf("validCSPNonce(%q) = false, want true", good)
		}
	}
}

// The capability append is idempotent for the EXACT issued capability: a URL
// already carrying kandev_cap=K is untouched (stopping the MutationObserver
// from looping on its own rewrites), while an app's different kandev_cap
// value still gets the issued capability appended.
func TestRuntimeShim_AppendIsIdempotentForIssuedCapability(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(!hcp(u))u+=(u.indexOf('?')===-1?'?':'&')+"kandev_cap="+K`)
	mustContain(t, shim, `function hcp(u){var qi=u.indexOf('?');if(qi===-1)return false;var ps=u.slice(qi+1).split('&');for(var i=0;i<ps.length;i++){var p=ps[i];var e=p.indexOf('=');var k=e===-1?p:p.slice(0,e);var v=e===-1?'':p.slice(e+1);var d;try{d=decodeURIComponent(k)}catch(x){d=k}var vd;try{vd=decodeURIComponent(v)}catch(x){vd=v}if(d==='kandev_cap'&&vd===K)return true}return false}`)
	// r() leaves network-relative values unchanged.
	mustContain(t, shim, `u.indexOf('//')===0`)
	// sc() decodes query keys before stripping.
	mustContain(t, shim, `try{d=decodeURIComponent(k)}catch(e){d=k}`)
}

// The runtime shim appends the capability to safe same-origin relative
// subresources too (e.g. a dynamically inserted relative manifest link), not
// just root-absolute ones.
func TestRuntimeShim_CapsRelativeSubresources(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-rel")
	// r() prefixes root-absolute URLs and falls through to the capability
	// splice for safe same-origin relative references.
	mustContain(t, shim, `if(u.charAt(0)==='/'){if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u;if(u.indexOf('/.')!==-1){var dd=rd(u);if(!(dd===P||dd.indexOf(P+'/')===0||dd.indexOf(P+'?')===0||dd.indexOf(P+'#')===0))return u;u=dd}}`)
	// The splice appends to u (prefixed or relative) with fragment handling.
	mustContain(t, shim, `if(!hcp(u))u+=(u.indexOf('?')===-1?'?':'&')+"kandev_cap="+K`)
	mustContain(t, shim, `function hcp(u){var qi=u.indexOf('?');if(qi===-1)return false;var ps=u.slice(qi+1).split('&');for(var i=0;i<ps.length;i++){var p=ps[i];var e=p.indexOf('=');var k=e===-1?p:p.slice(0,e);var v=e===-1?'':p.slice(e+1);var d;try{d=decodeURIComponent(k)}catch(x){d=k}var vd;try{vd=decodeURIComponent(v)}catch(x){vd=v}if(d==='kandev_cap'&&vd===K)return true}return false}`)
}

// Link rel values are whitespace-separated token lists: ANY fetching token
// makes the link a subresource; metadata-only lists (any case, extra tokens)
// stay capability-free.
func TestRewriteHTMLURLs_LinkRelTokenClassification(t *testing.T) {
	in := `<link rel="canonical nofollow" href="/canonical">` +
		`<link rel="Manifest" href="/manifest.webmanifest">` +
		`<link rel="stylesheet preload" href="/theme.css">` +
		`<link rel="alternate" href="/feed.xml">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-rel-tok", ""))

	mustContain(t, got, `rel="canonical nofollow" href="/port-proxy/abc/3001/canonical"`)
	mustContain(t, got, `rel="Manifest" href="/port-proxy/abc/3001/manifest.webmanifest?kandev_cap=cap-rel-tok"`)
	mustContain(t, got, `rel="stylesheet preload" href="/port-proxy/abc/3001/theme.css?kandev_cap=cap-rel-tok"`)
	mustContain(t, got, `rel="alternate" href="/port-proxy/abc/3001/feed.xml"`)
}

// A malformed first nonce must not hide a later valid one.
func TestRewriteHTMLURLs_CSPNonceSkipsInvalidCandidates(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-bad"<x>' 'nonce-good99'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	mustContain(t, string(got), `nonce="good99"`)
}

// Meta refresh url= is recognized at ANY semicolon field boundary, not just
// the first one.
func TestRewriteMetaRefresh_MultiField(t *testing.T) {
	in := `<meta http-equiv="refresh" content="5;foo=bar; url=/next">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-mf", ""))
	mustContain(t, got, `content="5;foo=bar; url=/port-proxy/abc/3001/next"`)
}

// The runtime r() classifies via the URL API: control/whitespace-obfuscated
// external URLs never receive the capability.
func TestRuntimeShim_NoCapForObfuscatedExternalURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-obf")
	mustContain(t, shim, `try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}`)
}

// The idempotency check is exact-parameter, not substring: a path like
// /foo/kandev_cap=K or an unrelated parameter ?note=kandev_cap=K must NOT
// suppress the issued capability (a cookie-less fetch would 401), while the
// exact issued kandev_cap=K parameter still stops the observer loop.
func TestRuntimeShim_ExactCapIdempotencyRejectsSubstrings(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(d==='kandev_cap'&&vd===K)return true`)
	// A substring elsewhere in the URL must not satisfy hcp: the append
	// guard tests the query parameter's decoded key AND exact value.
	mustContain(t, shim, `var e=p.indexOf('=');var k=e===-1?p:p.slice(0,e);var v=e===-1?'':p.slice(e+1);var d;try{d=decodeURIComponent(k)}catch(x){d=k}`)
}

// r()'s prefix check is boundary-exact: a sibling path that merely STARTS
// with the prefix (/port-proxy/s/51730/foo vs /port-proxy/s/5173) is treated
// as an ordinary root-absolute URL (prefixed again, capped), and an
// already-prefixed target WITHOUT the exact cap still falls through to the
// capability splice instead of returning unauthenticated.
func TestRuntimeShim_PrefixBoundaryAndPrefixedCap(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u`)
}

// rn() strips previously issued capabilities from EVERY same-origin form —
// relative, root-absolute, and absolute http/https — before applying the
// prefix-only navigation logic, so a fetching link reclassified to metadata
// never retains its bearer in the address bar, history, or Referer.
func TestRuntimeShim_RnStripsCapabilityOnAllForms(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(/^[a-z][a-z0-9+.-]*:\/\//i.test(u)){var o=ru.origin;var pn=ru.pathname;var tail=sc(pn+(ru.search||''));if(!(tail===P||tail.indexOf(P+'/')===0||tail.indexOf(P+'?')===0||tail.indexOf(P+'#')===0))tail=P+tail;return o+tail+(ru.hash||'')}return sc(u)}`)
	// rn() classifies via the URL API like r(): cross-origin and
	// network-relative navigation targets pass through untouched.
	mustContain(t, shim, `function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#')return u;if(u.indexOf('//')===0)return u;try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);`)
}

// norm() (fetch/XHR/WS) rejects raw network-relative input before URL
// normalization, matching r(): //host references never become proxied,
// capability-bearing paths.
func TestRuntimeShim_NormRejectsNetworkRelative(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(typeof s==='string'&&s.indexOf('//')===0)return u;`)
}

// The runtime style rewriter is a tokenizer, not a regex: CSS strings
// (content:'url(/x)') and url(var(--x)) are preserved, while real url()
// tokens are rewritten. The srcset splitter preserves commas inside data:
// URLs.
func TestRuntimeShim_StyleTokenizerAndSrcSetDataURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// Scanner, not regex: string-state tracking + var() skip.
	mustContain(t, shim, `function rwaStyle(v){var o='';var i=0;var n=v.length;`)
	mustContain(t, shim, `if(d2.toLowerCase().indexOf('var(')!==0){var rw2=r(d2);var em2=d2===rw2?tok2:cssEscTok(rw2);o+='url('+sp+em2`)
	mustContain(t, shim, `function srcsetParts(v){var parts=[];var cur='';var inData=false;`)
	mustContain(t, shim, `if(!inData&&curEmpty&&c!==' '&&c!=='\t'&&c!=='\n'&&c!=='\r'&&c!=='\f'&&v.slice(i,i+5).toLowerCase()==='data:')inData=true`)
	// content rewriting is guarded to META http-equiv=refresh; non-refresh
	// meta content and non-META content attributes are left unchanged.
	mustContain(t, shim, `else if(a==='content'){if(el.tagName==='META'){var he=el.getAttribute('http-equiv');if(he&&String(he).trim().toLowerCase()==='refresh')nv=mref(v)}}`)
	// cite is navigation (prefix-only), never capability-bearing.
	mustContain(t, shim, `||a==='cite';var rr=nav?rn:r;`)
}

// cite (q/blockquote/del/ins) is copyable metadata the browser never fetches:
// prefix-only, no capability — statically and at runtime.
func TestRewriteHTMLURLs_CiteOmitsCapability(t *testing.T) {
	in := `<blockquote cite="/source">x</blockquote>` +
		`<q cite="rel-source">y</q>` +
		`<img src="/img.png">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-cite", ""))

	mustContain(t, got, `<blockquote cite="/port-proxy/abc/3001/source">x</blockquote>`)
	mustContain(t, got, `<q cite="rel-source">y</q>`)
	mustContain(t, got, `<img src="/port-proxy/abc/3001/img.png?kandev_cap=cap-cite">`)
}

// srcset candidates split on commas, but commas INSIDE a data: URL belong to
// the URL: the data candidate is preserved whole (no fabricated second
// candidate, no capability on its fragments) and later candidates still get
// prefixed and capped.
func TestRewriteHTMLURLs_SrcSetDataURLCommasPreserved(t *testing.T) {
	in := `<img srcset="data:image/svg+xml,%3Csvg%3E 1x, /img.png 2x">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-ss", ""))

	mustContain(t, got, `srcset="data:image/svg+xml,%3Csvg%3E 1x, /port-proxy/abc/3001/img.png?kandev_cap=cap-ss 2x"`)
}

// The CSS tokenizer must not rewrite CSS strings (content:'url(/x)'), CSS
// variables (url(var(--x))), or comments; real url() tokens — quoted,
// unquoted, root-absolute, relative — and @import strings still are, and
// data: URLs keep their interior commas and semicolons.
func TestRewriteCSSFragment_StringVarCommentData(t *testing.T) {
	css := `a{content:'url(/literal)';background:url(var(--x));` +
		`b{background:url('/b.png');}c{background:url(c.png);}` +
		`d{background:url(data:image/png;base64,AAA,BB);}` +
		`/* url(/commented) */` +
		`@import "/theme.css";@import "rel.css";}`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")

	mustContain(t, got, `content:'url(/literal)'`)
	mustContain(t, got, `url(var(--x))`)
	mustContain(t, got, `/* url(/commented) */`)
	mustContain(t, got, `url('/port-proxy/abc/3001/b.png?kandev_cap=cap-css')`)
	mustContain(t, got, `url(c.png?kandev_cap=cap-css)`)
	mustContain(t, got, `url(data:image/png;base64,AAA,BB)`)
	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css?kandev_cap=cap-css";`)
	mustContain(t, got, `@import "rel.css?kandev_cap=cap-css";`)
}

// Navigation rewriting strips a previously issued capability from every URL
// form: an anchor whose href somehow carries kandev_cap must not keep the
// bearer in the DOM/address bar/history.
func TestRewriteHTMLURLs_NavStripsIssuedCapability(t *testing.T) {
	in := `<a href="/page?kandev_cap=stale">x</a>` +
		`<a href="rel?kandev_cap=stale">y</a>` +
		`<a href="/p?kandev_cap=stale&%6bandev_cap=stale2&keep=1">z</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nav", ""))

	mustContain(t, got, `<a href="/port-proxy/abc/3001/page?">x</a>`)
	mustContain(t, got, `<a href="rel?">y</a>`)
	mustContain(t, got, `<a href="/port-proxy/abc/3001/p?keep=1">z</a>`)
}

// 205 Reset Content is bodyless per RFC 7231 and must pass through untouched
// like 204/304: no body rewrite, no synthesized Content-Length.
func TestRewriteProxyResponse_Leaves205ResetContentUntouched(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusResetContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Length", "0")
	if err := rewriteProxyResponse(resp, proxyPrefix, "cap-205"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.Header.Get("Content-Length"); got != "0" {
		t.Fatalf("205: Content-Length = %q, want original %q", got, "0")
	}
}

// With multiple ENFORCING policies using different nonces, no single nonce
// satisfies all of them: the shim tag must not claim one. A policy that
// already allows the shim ('self') does not constrain the choice.
func TestRewriteHTMLURLs_CSPNonceMultipleEnforcingPolicies(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`

	// Two headers, different nonces, neither with 'self': intersection empty.
	conflict := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-aaa'`, `script-src 'nonce-bbb'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(conflict, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotConflict, _ := io.ReadAll(conflict.Body)
	if strings.Contains(string(gotConflict), "nonce=") {
		t.Fatalf("conflicting policies must not claim a nonce:\n%s", gotConflict)
	}

	// Header requires nonce-aaa; meta policy only has 'self': aaa wins.
	headerMeta := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-aaa'`},
		},
		Body: io.NopCloser(strings.NewReader(
			`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="script-src 'self'"><title>x</title></head><body></body></html>`)),
	}
	if err := rewriteProxyResponse(headerMeta, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotHeaderMeta, _ := io.ReadAll(headerMeta.Body)
	mustContain(t, string(gotHeaderMeta), `nonce="aaa"`)

	// Both policies carry the same nonce: it is emitted.
	shared := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-shared'`, `script-src 'nonce-other' 'nonce-shared'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(shared, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotShared, _ := io.ReadAll(shared.Body)
	mustContain(t, string(gotShared), `nonce="shared"`)
}

// 'strict-dynamic' ignores 'self' for script elements (CSP3): a policy with
// both must still get the nonce, or the shim is blocked.
func TestRewriteHTMLURLs_CSPNonceStrictDynamicRequiresNonce(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'self' 'strict-dynamic' 'nonce-sd123'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	mustContain(t, string(got), `nonce="sd123"`)
}

// script-src-elem governs script elements when present (CSP3): its nonce is
// the required one even when script-src carries a different nonce.
func TestRewriteHTMLURLs_CSPNonceScriptSrcElemPrecedence(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-srcA'; script-src-elem 'nonce-elemB'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	mustContain(t, string(got), `nonce="elemB"`)
}

// The runtime shim emits WHATWG-normalized forms: sc() preserves URL
// fragments while stripping capability parameters, r()/rn() normalize the
// emitted value (leading C0/space trimmed, embedded tabs/newlines removed)
// so space-prefixed dynamic references cannot escape the subtree.
func TestRuntimeShim_FragmentPreservingAndNormalizedForms(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// sc() splits the fragment off, filters the query, and re-appends it.
	mustContain(t, shim, `function sc(u){var h='';var hi=u.indexOf('#');if(hi!==-1){h=u.slice(hi);u=u.slice(0,hi)}var qi=u.indexOf('?');if(qi===-1)return u+h;var b=u.slice(0,qi);var q=u.slice(qi+1).split('&').filter(function(p){var k=p.split('=')[0];var d;try{d=decodeURIComponent(k)}catch(e){d=k}return d!=='kandev_cap'});if(q.length===0)return b+'?'+h;return b+'?'+q.join('&')+h}`)
	// r() and rn() normalize the emitted form after URL-API classification.
	mustContain(t, shim, `function nz(u){return u.replace(/[\\\t\n\r]/g,function(m){return m==='\\'?'/':''}).replace(/^[\u0000-\u0020]+|[\u0000-\u0020]+$/g,'')}`)
	mustContain(t, shim, `u=nz(u);if(u.charAt(0)==='/'){if(!(u===P||`)
	mustContain(t, shim, `u=nz(u);if(u.charAt(0)==='/'){u=sc(u);if(!(u===P||`)
}

// hcp decodes the parameter VALUE before comparing, so a percent-encoded
// spelling of the issued capability is recognized (idempotent, no duplicate
// append) instead of being treated as an app value.
func TestRuntimeShim_HcpDecodesValue(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap.ABC-_123")
	mustContain(t, shim, `var vd;try{vd=decodeURIComponent(v)}catch(x){vd=v}if(d==='kandev_cap'&&vd===K)return true`)
}

// The runtime style tokenizer is escape-aware for CSS strings and requires a
// token boundary before url(: an escaped quote inside a string cannot end it,
// and noturl(...) is never rewritten.
func TestRuntimeShim_StyleTokenizerEscapeAndBoundary(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// Escape-aware string scan: backslash + next byte skipped before looking
	// for the closing quote.
	mustContain(t, shim, `if(c==='\''||c==='"'){var j=i+1;while(j<n){if(v.charAt(j)==='\\'&&j+1<n){j+=2;continue}if(v.charAt(j)===c)break;j++}`)
	// Identifier boundary: previous char must not be ident-continuation.
	mustContain(t, shim, `(j2=cssFn(v,i,'url'))>=0`)
}

// Static CSS: url( only at a token boundary (noturl(/x) untouched) and
// var() skip is case-insensitive (url(VAR(--x)) untouched).
func TestRewriteCSSFragment_IdentBoundaryAndVarCase(t *testing.T) {
	css := `a{content:noturl(/literal);b:url(VAR(--x));c:url(ok.png);}`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")

	mustContain(t, got, `content:noturl(/literal)`)
	mustContain(t, got, `url(VAR(--x))`)
	mustContain(t, got, `url(ok.png?kandev_cap=cap-css)`)
}

// strict-dynamic later in the same directive is not masked by an earlier
// 'self': the policy still constrains the nonce choice and participates in
// the intersection — with a conflicting second policy, no nonce is claimed.
func TestRewriteHTMLURLs_CSPNonceStrictDynamicNotMaskedBySelf(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	// First policy: 'self' then 'strict-dynamic' with nonce-sdA; second
	// policy: nonce-sdB. The first policy IS constraining (strict-dynamic
	// beats 'self'), so the intersection is empty and NO nonce may be
	// claimed — claiming sdB would be blocked by policy 1.
	conflict := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'self' 'strict-dynamic' 'nonce-sdA'`, `script-src 'nonce-sdB'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(conflict, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotConflict, _ := io.ReadAll(conflict.Body)
	if strings.Contains(string(gotConflict), "nonce=") {
		t.Fatalf("strict-dynamic policy must participate in the intersection:\n%s", gotConflict)
	}

	// Shared nonce across both (one strict-dynamic, one plain): emitted.
	shared := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'self' 'strict-dynamic' 'nonce-sdA'`, `script-src 'nonce-sdA' 'nonce-x'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(shared, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotShared, _ := io.ReadAll(shared.Body)
	mustContain(t, string(gotShared), `nonce="sdA"`)
}

// When script-src-elem exists it is the governing directive for script
// elements: script-src nonces are NOT used for it, even when elem carries no
// nonce tokens (strict-dynamic without a nonce cannot be satisfied, so no
// nonce is claimed).
func TestRewriteHTMLURLs_CSPNonceElemIsolatesSrcNonces(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            {"text/html"},
			"Content-Security-Policy": {`script-src 'nonce-srcA'; script-src-elem 'strict-dynamic'`},
		},
		Body: io.NopCloser(strings.NewReader(in)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(got), `nonce=`) {
		t.Fatalf("script-src nonce must not be claimed for an elem-governed policy:\n%s", got)
	}
}

// stripCapability preserves URL fragments while removing capability
// parameters (runtime sc() matches; the static navigation path must too).
func TestRewriteHTMLURLs_NavStripsCapPreservesFragment(t *testing.T) {
	in := `<a href="/p?kandev_cap=stale&keep=1#frag">x</a>` +
		`<a href="rel?kandev_cap=stale#top">y</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nav", ""))

	mustContain(t, got, `<a href="/port-proxy/abc/3001/p?keep=1#frag">x</a>`)
	mustContain(t, got, `<a href="rel?#top">y</a>`)
}

// stripCapability splits the fragment off FIRST: a '?' inside a fragment
// (#state?x=1) is fragment payload, and data URLs whose payload contains '#'
// or '?' are never treated as gateway queries. Scheme-bearing and
// network-relative navigation references are preserved verbatim.
func TestRewriteHTMLURLs_NavStripPreservesFragmentPayload(t *testing.T) {
	in := `<a href="/x#state?kandev_cap=stale">a</a>` +
		`<a href="data:text/plain,#payload?kandev_cap=x">b</a>` +
		`<a href="/p?a=1&kandev_cap=stale#f?x=2">c</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nav", ""))

	mustContain(t, got, `<a href="/port-proxy/abc/3001/x#state?kandev_cap=stale">a</a>`)
	mustContain(t, got, `<a href="data:text/plain,#payload?kandev_cap=x">b</a>`)
	mustContain(t, got, `<a href="/port-proxy/abc/3001/p?a=1#f?x=2">c</a>`)
}

// The CSS tokenizer consumes CSS escapes when recognizing function names:
// \75rl(/x) decodes to url(/x) and must be rewritten (browser preprocessing
// does the same). @import accepts comments as whitespace, so
// @import/*c*/"/theme.css" is a valid import and must be rewritten.
func TestRewriteCSSFragment_EscapedFunctionsAndImportComments(t *testing.T) {
	css := `a{background:\75rl(/asset.css);}` +
		`@import/*c*/"/theme.css";`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")

	mustContain(t, got, `\75rl(/port-proxy/abc/3001/asset.css?kandev_cap=cap-css)`)
	mustContain(t, got, `@import/*c*/"/port-proxy/abc/3001/theme.css?kandev_cap=cap-css"`)
}

// Runtime rwaStyle recognizes escaped url( forms too (\75rl(...)), matching
// the static tokenizer.
func TestRuntimeShim_StyleTokenizerEscapedUrl(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `function cssEsc(s,i){`)
	mustContain(t, shim, `function cssFn(s,i,nm){`)
	mustContain(t, shim, `(j2=cssFn(v,i,'url'))>=0`)
}

// CSS escapes inside url() TOKEN values decode before classification:
// url(\2f asset.css) is url(/asset.css) to the browser, so the rewritten
// token must carry the proxy prefix + capability (and be re-escaped only
// when the decoded value contains CSS-significant characters).
func TestRewriteCSSFragment_EscapedSlashToken(t *testing.T) {
	css := `a{background:url(\2f asset.css);}` +
		`b{background:url('/q.css');}` +
		`c{background:url(foo\20 bar.png);}`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")

	mustContain(t, got, `url(/port-proxy/abc/3001/asset.css?kandev_cap=cap-css)`)
	mustContain(t, got, `url('/port-proxy/abc/3001/q.css?kandev_cap=cap-css')`)
	// The decoded space is re-escaped so the emitted token still parses.
	mustContain(t, got, `url(foo\20 bar.png?kandev_cap=cap-css)`)
}

// At most SIX hex digits form a CSS escape: \0000075rl( decodes \000007 then
// '5rl(' — not a url() function — so it must pass through untouched.
func TestRewriteCSSFragment_SevenHexDigitsAreNotAFunction(t *testing.T) {
	css := `a{background:\0000075rl(/x);}`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")
	mustContain(t, got, `\0000075rl(/x)`)
}

// Stripping the ONLY query pair preserves the explicit empty query marker
// (?), so the reference does not inherit the current document's query.
func TestRewriteHTMLURLs_NavStripKeepsEmptyQueryMarker(t *testing.T) {
	in := `<a href="/p?kandev_cap=stale">x</a>` +
		`<a href="/p?kandev_cap=stale#f">y</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nav", ""))

	mustContain(t, got, `<a href="/port-proxy/abc/3001/p?">x</a>`)
	mustContain(t, got, `<a href="/port-proxy/abc/3001/p?#f">y</a>`)
}

// Round-4 runtime shim contracts: rwa initializes nv to the current value
// (a non-refresh content attribute is never overwritten with "undefined"),
// navigation wrappers normalize URL-object arguments, and cssDecTok/cssEscTok
// decode/encode token escapes.
func TestRuntimeShim_Round4Contracts(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	// rwa defaults nv to v so guarded branches leave the attribute alone.
	mustContain(t, shim, `var rr=nav?rn:r;var nv=v;if(a==='srcset'||a==='imagesrcset')`)
	// URL-object navigation arguments are stringified through rn().
	mustContain(t, shim, `if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}`)
	// Token escape decode/encode helpers exist.
	mustContain(t, shim, `function cssDecTok(t){`)
	mustContain(t, shim, `function cssEscTok(t){`)
	// sc keeps the empty query marker when all pairs were stripped.
	mustContain(t, shim, `if(q.length===0)return b+'?'+h;`)
}

// Escaped root slashes inside QUOTED @import strings decode before
// classification too: @import"\2f theme.css" is @import"/theme.css" to the
// browser, so the rewritten import must carry the proxy prefix + capability.
func TestRewriteCSSFragment_EscapedQuotedImport(t *testing.T) {
	css := `@import"\2f theme.css";` +
		`@import"\61 b.css";`
	got := rewriteCSSFragment(css, proxyPrefix, "cap-css", proxyPrefix+"/")

	mustContain(t, got, `@import"/port-proxy/abc/3001/theme.css?kandev_cap=cap-css";`)
	// Relative decoded import: stays in the subtree, capability appended.
	mustContain(t, got, `@import"ab.css?kandev_cap=cap-css";`)
}

// Navigation wrappers funnel ANY DOMString-able URL argument through rn():
// objects with toString (not just URL objects), String objects, numbers —
// the native API stringifies them, so the shim must rewrite the stringified
// form.
func TestRuntimeShim_DOMStringNavigationArguments(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(this,s,t,u)`)
	mustContain(t, shim, `if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(location,u)`)
}

// Meta refresh honors leading whitespace before url= at the value start:
// content="  url=/next" is a browser-accepted field boundary and must be
// rewritten (matching the runtime mref()).
func TestRewriteHTMLURLs_MetaRefreshLeadingWhitespace(t *testing.T) {
	in := `<meta http-equiv="refresh" content="  url=/next">` +
		"<meta http-equiv=\"refresh\" content=\"\turl=/tabbed\">"
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-mw", ""))

	mustContain(t, got, `content="  url=/port-proxy/abc/3001/next"`)
	mustContain(t, got, "content=\"\turl=/port-proxy/abc/3001/tabbed\"")
}

// The runtime navigation wrappers throw TypeError on Symbol arguments, like
// native DOMString conversion (String(Symbol) would silently stringify).
func TestRuntimeShim_SymbolNavigationThrows(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string')`)
}

// Meta refresh matching is a length-preserving ASCII fold over the ORIGINAL
// bytes: a Unicode character whose lowercase fold changes byte length (U+212A
// KELVIN, 3 bytes to 1) before the field must not corrupt the offsets.
func TestRewriteHTMLURLs_MetaRefreshUnicodeBeforeField(t *testing.T) {
	in := "<meta http-equiv=\"refresh\" content=\"\u212A; url=/next\">"
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-mu", ""))

	mustContain(t, got, "content=\"\u212A; url=/port-proxy/abc/3001/next\"")
}

// The reserved capability cookie is stripped from upstream Set-Cookie
// headers: a proxied app must not be able to override kandev_port_proxy.
func TestFilterReservedCookies(t *testing.T) {
	h := make(http.Header)
	h.Add("Set-Cookie", "kandev_port_proxy=invalid; Path=/port-proxy/s/1/3000")
	h.Add("Set-Cookie", "session=ok; Path=/")
	h.Add("Set-Cookie", "other=1")
	filterReservedCookies(h)

	got := h.Values("Set-Cookie")
	if len(got) != 2 {
		t.Fatalf("Set-Cookie count = %d, want 2 (reserved stripped): %v", len(got), got)
	}
	for _, c := range got {
		if cookieName(c) == proxyCapabilityCookieName {
			t.Fatalf("reserved cookie survived filtering: %q", c)
		}
	}

	// All-reserved: the header is removed entirely.
	h2 := make(http.Header)
	h2.Add("Set-Cookie", "kandev_port_proxy=x")
	filterReservedCookies(h2)
	if len(h2.Values("Set-Cookie")) != 0 {
		t.Fatalf("all-reserved Set-Cookie not removed: %v", h2)
	}
}

// Large srcset values are split in a single pass (no per-byte rescan of the
// accumulated candidate): correctness must hold on a large input.
func TestRewriteSrcSet_LargeInputLinear(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "/img-%d.png 1x, ", i)
	}
	b.WriteString("/last.png 2x")
	got := rewriteSrcSet(b.String(), proxyPrefix, "cap-ss", proxyPrefix+"/")

	if !strings.HasPrefix(got, "/port-proxy/abc/3001/img-0.png?kandev_cap=cap-ss 1x, ") {
		t.Fatalf("first candidate not rewritten: %.80s", got)
	}
	if !strings.HasSuffix(got, "/port-proxy/abc/3001/last.png?kandev_cap=cap-ss 2x") {
		t.Fatalf("last candidate not rewritten: %.80s", got[len(got)-80:])
	}
}

// A candidate with a very long leading-whitespace prefix must not trigger a
// per-byte suffix rescan: the data: prefix is tested only at the first
// non-whitespace byte, so this stays linear and produces the correct
// candidates.
func TestRewriteSrcSet_LongWhitespacePrefix(t *testing.T) {
	prefix := strings.Repeat(" ", 20000)
	in := prefix + `data:image/svg+xml,%3Csvg%3E 1x, /img.png 2x`
	got := rewriteSrcSet(in, proxyPrefix, "cap-ss", proxyPrefix+"/")

	mustContain(t, got, `data:image/svg+xml,%3Csvg%3E 1x, /port-proxy/abc/3001/img.png?kandev_cap=cap-ss 2x`)
}

// Static navigation rewrites must strip a stale capability from http(s)
// ABSOLUTE references (the browser resolves them as gateway HTTP queries),
// while data:/mailto: payloads and network-relative references are preserved
// verbatim. The same path covers redirect Location headers.
func TestRewriteHTMLURLs_NavStripsCapFromHTTPAbsolute(t *testing.T) {
	in := `<a href="http://127.0.0.1:44444/page?kandev_cap=stale">x</a>` +
		`<a href="https://app.example/feed?kandev_cap=stale">y</a>` +
		`<a href="mailto:x@y.z?kandev_cap=stale">m</a>` +
		`<a href="data:text/plain,#p?kandev_cap=x">d</a>` +
		`<a href="//cdn.example/a?kandev_cap=stale">n</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nav", ""))

	mustContain(t, got, `<a href="http://127.0.0.1:44444/page?">x</a>`)
	mustContain(t, got, `<a href="https://app.example/feed?">y</a>`)
	mustContain(t, got, `<a href="mailto:x@y.z?kandev_cap=stale">m</a>`)
	mustContain(t, got, `<a href="data:text/plain,#p?kandev_cap=x">d</a>`)
	mustContain(t, got, `<a href="//cdn.example/a?kandev_cap=stale">n</a>`)

	// Redirect Location headers go through the same navigation rewrite.
	loc := rewriteURLReference("http://127.0.0.1:44444/next?kandev_cap=stale", proxyPrefix, "")
	if loc != "http://127.0.0.1:44444/next?" {
		t.Fatalf("Location strip = %q, want stripped absolute", loc)
	}
}

// Unquoted meta-refresh targets terminate at ALL five ASCII whitespace bytes,
// matching the runtime mref() and CSS whitespace semantics: an LF after the
// target must not be joined into it.
func TestRewriteHTMLURLs_MetaRefreshNewlineDelimiter(t *testing.T) {
	in := "<meta http-equiv=\"refresh\" content=\"0; url=/next\nfoo\">"
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-nl", ""))

	mustContain(t, got, "content=\"0; url=/port-proxy/abc/3001/next\nfoo\"")
}

// The srcset data: prefix test is bounded to five bytes: many candidates
// must not rescan the remaining suffix (correctness on a large input).
func TestRewriteSrcSet_ManyCandidatesBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "  %d.png 1x, ", i)
	}
	b.WriteString("/last.png 2x")
	got := rewriteSrcSet(b.String(), proxyPrefix, "cap-ss", proxyPrefix+"/")

	if !strings.HasPrefix(got, "0.png?kandev_cap=cap-ss 1x, ") {
		t.Fatalf("first candidate not rewritten: %.60s", got)
	}
	if !strings.HasSuffix(got, "/port-proxy/abc/3001/last.png?kandev_cap=cap-ss 2x") {
		t.Fatalf("last candidate not rewritten: %.60s", got[len(got)-60:])
	}
}

// XHR.open and WebSocket convert DOMString-able URL arguments to strings
// BEFORE normalization (native Web IDL stringifies after the wrapper, which
// would leave object args unrewritten at the gateway root); Symbols throw
// like native DOMString conversion.
func TestRuntimeShim_DOMStringXHRAndWebSocket(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `XMLHttpRequest.prototype.open=function(m,u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=norm(u)}arguments[1]=u;return oo.apply(this,arguments)}`)
	mustContain(t, shim, `function W(u,p){if(!new.target)throw new TypeError("Failed to construct 'WebSocket': Please use the 'new' operator, this DOM object constructor cannot be called as a function.");if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u)}var n=norm(u);`)
}

// fetch rewrites only BRANDED URL/Request instances (Symbol.toStringTag,
// cross-realm safe): plain duck-typed {href}/{url} objects are left for
// native coercion, which rejects them with TypeError like unfetched fetch
// input. WebSocket is a constructor: a plain call without 'new' throws like
// the native API.
func TestRuntimeShim_FetchBrandChecksAndWSConstructor(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `Reflect.get(URL.prototype,'href',i)`)
	mustContain(t, shim, `Reflect.get(Request.prototype,'url',i)`)
	mustContain(t, shim, `Failed to construct 'WebSocket': Please use the 'new' operator`)
}

// Backslashes are slashes for special-URL references per WHATWG: static
// href/src="\foo" must be prefixed (the browser resolves it to /foo), while
// "\/evil" becomes the network-relative //evil and stays untouched.
func TestRewriteHTMLURLs_BackslashPaths(t *testing.T) {
	in := `<img src="\logo.png">` +
		`<a href="\page">x</a>` +
		`<img src="\/evil.png">` +
		`<link rel="stylesheet" href="\theme.css">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-bs", ""))

	mustContain(t, got, `<img src="/port-proxy/abc/3001/logo.png?kandev_cap=cap-bs">`)
	mustContain(t, got, `<a href="/port-proxy/abc/3001/page">x</a>`)
	mustContain(t, got, `<img src="\/evil.png">`)
	mustContain(t, got, `<link rel="stylesheet" href="/port-proxy/abc/3001/theme.css?kandev_cap=cap-bs">`)
}

// The WebSocket wrapper preserves subclass identity (Reflect.construct with
// new.target) and passes an explicit empty protocols argument through (native
// rejects empty subprotocols; the wrapper must not collapse it to one arg).
func TestRuntimeShim_WSSubclassAndProtocols(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `Reflect.construct(OW,[s,p],new.target||W)`)
	mustContain(t, shim, `if(arguments.length>1)return Reflect.construct(OW,[s,p],new.target||W);return Reflect.construct(OW,[s],new.target||W)`)
}

// fetch must not rewrite credentialed URL objects: native Request
// construction rejects credentials, so the shim leaves the URL untouched and
// native coercion produces its usual error.
func TestRuntimeShim_FetchSkipsCredentialedURLs(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(ib&&!i.username&&!i.password){var hu=norm(i.href);`)
}

// Under an external <base href>, every path-absolute and relative reference
// resolves OUTSIDE the proxy subtree: the capability must not be embedded
// (bearer exfiltration), and rewriting is suppressed entirely so the app's
// intended resolution is preserved. A same-origin (path) base keeps normal
// subtree rewriting.
func TestRewriteHTMLURLs_ExternalBaseSuppressesCapability(t *testing.T) {
	external := `<base href="https://evil.example/">` +
		`<script src="x.js"></script>` +
		`<img src="/logo.png">` +
		`<a href="/page">nav</a>` +
		`<div style="background:url(bg.png)"></div>`
	got := string(rewriteHTMLURLs([]byte(external), proxyPrefix, "cap-base", ""))

	if strings.Contains(got, "kandev_cap") {
		t.Fatalf("capability leaked under an external base:\n%s", got)
	}
	mustContain(t, got, `<script src="x.js"></script>`)
	mustContain(t, got, `<img src="/logo.png">`)
	mustContain(t, got, `<a href="/page">nav</a>`)
	mustContain(t, got, `style="background:url(bg.png)"`)
	mustContain(t, got, `<base href="https://evil.example/">`)

	// Network-relative base is external too.
	netRel := `<base href="//cdn.example/">` + `<img src="/logo.png">`
	gotNet := string(rewriteHTMLURLs([]byte(netRel), proxyPrefix, "cap-base2", ""))
	if strings.Contains(gotNet, "kandev_cap") {
		t.Fatalf("capability leaked under a network-relative base:\n%s", gotNet)
	}
	mustContain(t, gotNet, `<img src="/logo.png">`)

	// Same-origin (path) base: refs resolve inside the subtree, cap applies.
	sameOrigin := `<base href="/cdn/">` + `<img src="a.png">` + `<script src="/s.js"></script>`
	gotSame := string(rewriteHTMLURLs([]byte(sameOrigin), proxyPrefix, "cap-base3", ""))
	mustContain(t, gotSame, `<base href="/port-proxy/abc/3001/cdn/">`)
	mustContain(t, gotSame, `<img src="a.png?kandev_cap=cap-base3">`)
	mustContain(t, gotSame, `<script src="/port-proxy/abc/3001/s.js?kandev_cap=cap-base3"></script>`)
}

// 206 Partial Content bodies are byte ranges described by Content-Range:
// rewriting would desynchronize length/range metadata, so they pass through.
func TestRewriteProxyResponse_Leaves206Untouched(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusPartialContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("<html>partial</html>")),
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Range", "bytes 0-21/100")
	resp.Header.Set("Content-Length", "22")
	if err := rewriteProxyResponse(resp, proxyPrefix, "cap-206"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.Header.Get("Content-Length"); got != "22" {
		t.Fatalf("206: Content-Length = %q, want original %q", got, "22")
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 0-21/100" {
		t.Fatalf("206: Content-Range = %q, want original", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<html>partial</html>" {
		t.Fatalf("206 body changed: %q", body)
	}
}

// imagesrcset (link preload) is a fetching srcset attribute: it gets the
// capability like srcset, statically and at runtime.
func TestRewriteHTMLURLs_Imagesrcset(t *testing.T) {
	in := `<link rel="preload" as="image" imagesrcset="/hero.png 1x, /hero@2x.png 2x">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-is", ""))

	mustContain(t, got, `imagesrcset="/port-proxy/abc/3001/hero.png?kandev_cap=cap-is 1x, /port-proxy/abc/3001/hero@2x.png?kandev_cap=cap-is 2x"`)
}

// norm() rejects credentialed URLs (native Request/fetch would reject too),
// leaving the input untouched so native validation runs.
func TestRuntimeShim_NormRejectsCredentials(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `if(x.username||x.password)return u;`)
}

// A srcdoc child under an external parent base can declare its OWN <base>,
// resetting the inherited external policy: its refs then resolve inside the
// subtree and get the capability (dispatch happens before the generic
// suppression, since a srcdoc value looks relative).
func TestRewriteHTMLURLs_ExternalBaseSrcdocOwnBase(t *testing.T) {
	in := `<base href="https://evil.example/"><iframe srcdoc="&lt;base href=&quot;/cdn/&quot;&gt;&lt;img src=&quot;a.png&quot;&gt;&lt;img src=&quot;/logo.png&quot;&gt;"></iframe>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-sd", ""))

	mustContain(t, got, `srcdoc="&lt;base href=&#34;/port-proxy/abc/3001/cdn/&#34;&gt;&lt;img src=&#34;a.png?kandev_cap=cap-sd&#34;&gt;&lt;img src=&#34;/port-proxy/abc/3001/logo.png?kandev_cap=cap-sd&#34;&gt;"`)
}

// imagesrcset is observed at runtime: it is in the MutationObserver attribute
// filter and the descendant selector.
func TestRuntimeShim_ImagesrcsetObserved(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `'srcset','imagesrcset','ping','rel'`)
	mustContain(t, shim, `[srcset],[imagesrcset],[ping],[style]`)
}

// <style> element BODIES under an external base must not be rewritten:
// url() references inside resolve against the external origin and would
// leak the capability (same policy as inline style attributes).
func TestRewriteHTMLURLs_ExternalBaseStyleBody(t *testing.T) {
	external := `<base href="https://evil.example/">` +
		`<style>body{background:url(/x.png)}</style>`
	got := string(rewriteHTMLURLs([]byte(external), proxyPrefix, "cap-sb", ""))

	if strings.Contains(got, "kandev_cap") {
		t.Fatalf("capability leaked from a style body under an external base:\n%s", got)
	}
	mustContain(t, got, `<style>body{background:url(/x.png)}</style>`)

	// Network-relative base: same suppression.
	netRel := `<base href="//cdn.example/">` + `<style>.a{background:url(/y.png)}</style>`
	gotNet := string(rewriteHTMLURLs([]byte(netRel), proxyPrefix, "cap-sb2", ""))
	if strings.Contains(gotNet, "kandev_cap") {
		t.Fatalf("capability leaked under a network-relative base:\n%s", gotNet)
	}

	// No base: style bodies are still rewritten normally.
	plain := `<style>body{background:url(/z.png)}</style>`
	gotPlain := string(rewriteHTMLURLs([]byte(plain), proxyPrefix, "cap-sb3", ""))
	mustContain(t, gotPlain, `<style>body{background:url(/port-proxy/abc/3001/z.png?kandev_cap=cap-sb3)}</style>`)
}

// a[ping] is a whitespace-separated URL list POSTed on activation: every
// candidate gets the capability-bearing subresource rewrite, statically and
// at runtime, and ping is observed.
func TestRewriteHTMLURLs_PingList(t *testing.T) {
	in := `<a href="/ok" ping="/track /more">x</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-ping", ""))

	mustContain(t, got, `<a href="/port-proxy/abc/3001/ok" ping="/port-proxy/abc/3001/track?kandev_cap=cap-ping /port-proxy/abc/3001/more?kandev_cap=cap-ping">x</a>`)
}

func TestRuntimeShim_PingObservedAndRewritten(t *testing.T) {
	shim := runtimeShim(proxyPrefix, "cap-shim")
	mustContain(t, shim, `'ping','rel'`)
	mustContain(t, shim, `else if(a==='ping'){nv=v.split(/\s+/).map(function(u){return r(u)}).join(' ')}`)
}

// WHATWG strips leading AND trailing C0/space for special URLs: a trailing
// space must not survive into the emitted prefixed URL (it would become %20
// in the pathname and 404).
func TestRewriteHTMLURLs_TrailingWhitespace(t *testing.T) {
	in := `<img src="/x ">` + `<a href="/page ">n</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-tw", ""))

	mustContain(t, got, `<img src="/port-proxy/abc/3001/x?kandev_cap=cap-tw">`)
	mustContain(t, got, `<a href="/port-proxy/abc/3001/page">n</a>`)
}

// Relative capability-bearing references emit the WHATWG-normalized form: a
// trailing space must not survive into `x ?kandev_cap=…` (the browser would
// percent-encode it into the pathname). Covers HTML and quoted CSS url().
func TestRewriteHTMLURLs_RelativeTrailingSpace(t *testing.T) {
	in := `<img src="x ">` + "<img src=\"y\t\">"
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-rt", ""))

	mustContain(t, got, `<img src="x?kandev_cap=cap-rt">`)
	mustContain(t, got, `<img src="y?kandev_cap=cap-rt">`)

	css := `a{background:url('z ')}`
	gotCSS := rewriteCSSFragment(css, proxyPrefix, "cap-rt", proxyPrefix+"/")
	mustContain(t, gotCSS, `url('z?kandev_cap=cap-rt')`)
}

// Relative references whose dot-segment resolution escapes the proxy
// subtree must not carry the capability: ../x resolves to a sibling
// gateway path, and a relative base with .. escapes the prefix entirely.
func TestRewriteHTMLURLs_DotSegmentEscapesSuppressCapability(t *testing.T) {
	in := `<img src="../x.png">` +
		`<img src="./../y.png">` +
		`<img srcset="../a.png 1x, ok.png 2x">` +
		`<div style="background:url(../z.png)"></div>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-dot", ""))

	mustContain(t, got, `<img src="../x.png">`)
	mustContain(t, got, `<img src="./../y.png">`)
	// The escaping ../ candidate stays uncapped; the in-subtree ok.png caps.
	mustContain(t, got, `<img srcset="../a.png 1x, ok.png?kandev_cap=cap-dot 2x">`)
	mustContain(t, got, `style="background:url(../z.png)"`)

	// A relative base that escapes P is treated as external: everything
	// path/relative is suppressed.
	escaping := `<base href="../../outside/">` + `<img src="x.png">` + `<script src="/s.js"></script>`
	gotEsc := string(rewriteHTMLURLs([]byte(escaping), proxyPrefix, "cap-dot2", ""))
	if strings.Contains(gotEsc, "kandev_cap") {
		t.Fatalf("capability leaked under an escaping relative base:\n%s", gotEsc)
	}
	mustContain(t, gotEsc, `<img src="x.png">`)

	// An in-subtree base keeps capping refs that resolve inside it, but a
	// ../ ref escaping even that base stays uncapped.
	inSub := `<base href="sub/">` + `<img src="ok.png">` + `<img src="../up.png">`
	gotIn := string(rewriteHTMLURLs([]byte(inSub), proxyPrefix, "cap-dot3", ""))
	// The relative base stays as authored (nav relative refs are never
	// prefixed); the browser resolves it to P/sub/.
	mustContain(t, gotIn, `<base href="sub/">`)
	mustContain(t, gotIn, `<img src="ok.png?kandev_cap=cap-dot3">`)
	mustContain(t, gotIn, `<img src="../up.png?kandev_cap=cap-dot3">`)
}

// A query or fragment attached to a dot segment (../?q, ../#f) must not
// hide the escape: the path-only resolution leaves such references uncapped.
func TestRewriteHTMLURLs_DotSegmentWithQueryOrFragment(t *testing.T) {
	in := `<img src="../?q">` +
		`<img src="../#f">` +
		`<img src="../?q#f">` +
		`<img src="ok.png?x=1">`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix, "cap-qf", ""))

	mustContain(t, got, `<img src="../?q">`)
	mustContain(t, got, `<img src="../#f">`)
	mustContain(t, got, `<img src="../?q#f">`)
	mustContain(t, got, `<img src="ok.png?x=1&amp;kandev_cap=cap-qf">`)
}

// HTML documents served BELOW the proxy root resolve relative references
// against their own directory (stashed in the request context), so ../ refs
// that stay in-subtree keep the capability.
func TestRewriteProxyResponse_HTMLUsesDocumentDirectoryBase(t *testing.T) {
	body := `<img src="../logo.png">` + `<img src="ok.png">`
	req := httptest.NewRequest(http.MethodGet, "/port-proxy/s/5173/pages/index.html", nil)
	req = req.WithContext(context.WithValue(req.Context(), proxyCSSBaseContextKey{}, "/port-proxy/s/5173/pages/"))
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	if err := rewriteProxyResponse(resp, "/port-proxy/s/5173", "cap-htmlbase"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	// ../logo.png resolves to /port-proxy/s/5173/logo.png (in-subtree): capped.
	mustContain(t, string(got), `<img src="../logo.png?kandev_cap=cap-htmlbase">`)
	mustContain(t, string(got), `<img src="ok.png?kandev_cap=cap-htmlbase">`)
}

// A response still content-encoded after transport processing (Go could not
// decode it) must not be rewritten: rewriting compressed bytes and deleting
// Content-Encoding would corrupt the document.
func TestRewriteProxyResponse_SkipsUndecodedEncodings(t *testing.T) {
	for _, enc := range []string{"br", "deflate", "zstd"} {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("\x1f\x8b-compressed")),
		}
		resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		resp.Header.Set("Content-Encoding", enc)
		if err := rewriteProxyResponse(resp, proxyPrefix, "cap-enc"); err != nil {
			t.Fatalf("%s: unexpected error: %v", enc, err)
		}
		if got := resp.Header.Get("Content-Encoding"); got != enc {
			t.Fatalf("%s: Content-Encoding = %q, want preserved", enc, got)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "\x1f\x8b-compressed" {
			t.Fatalf("%s: body changed: %q", enc, body)
		}
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output missing %q\noutput: %s", needle, haystack)
	}
}
