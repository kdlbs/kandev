package websocket

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// URL attributes that may carry a path-absolute reference and therefore need
// the proxy path prefix when the document is served through the port proxy.
// Source: https://html.spec.whatwg.org/multipage/indices.html#attributes-3
var rewritableURLAttrs = map[string]bool{
	"href":       true,
	"src":        true,
	"action":     true,
	"formaction": true,
	"cite":       true,
	"data":       true,
	"poster":     true,
	"background": true,
	"manifest":   true,
}

// styleTagName is the HTML element name we treat as a raw-text CSS body so we
// can pipe its contents through `rewriteCSSFragment`.
const styleTagName = "style"

// headTagName is the HTML element after whose opening tag we inject the proxy
// runtime shim, so it installs `fetch`/`XHR`/`WebSocket` overrides before any
// user JS can run.
const headTagName = "head"

// runtimeShimTemplate is the JS bootstrap injected at the top of `<head>` for
// every HTML response we proxy. It overrides the network-facing browser APIs
// (fetch, XMLHttpRequest, WebSocket) so that root-absolute URLs requested at
// runtime — e.g. Next.js dynamic chunk imports, fetch calls, HMR WebSocket —
// stay on the same proxy chain instead of escaping to the host origin.
//
// {{PREFIX}} is replaced per-response with the proxy path (no trailing slash).
const runtimeShimTemplate = `<script>(function(){var P=%q;function r(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)!=='/'||(u.length>1&&u.charAt(1)==='/'))return u;if(u.indexOf(P)===0)return u;return P+u;}var of=window.fetch;if(of){window.fetch=function(i,n){if(typeof i==='string')i=r(i);else if(i&&typeof i==='object'&&typeof i.url==='string'){var nu=r(i.url);if(nu!==i.url){try{i=new Request(nu,i)}catch(e){}}}return of.call(this,i,n)}}var oo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){arguments[1]=r(u);return oo.apply(this,arguments)};var OW=window.WebSocket;if(OW){function W(u,p){if(typeof u==='string'&&u.charAt(0)==='/'&&(u.length<2||u.charAt(1)!=='/')){var l=window.location;u=(l.protocol==='https:'?'wss:':'ws:')+'//'+l.host+P+u}return p?new OW(u,p):new OW(u)}W.prototype=OW.prototype;Object.getOwnPropertyNames(OW).forEach(function(k){try{W[k]=OW[k]}catch(e){}});window.WebSocket=W}})();</script>`

// runtimeShim returns the runtime shim script tag with the given proxy prefix
// baked in. %q produces a JS-safe double-quoted string literal (slashes and
// alphanumerics need no escaping, which matches every prefix we emit).
func runtimeShim(prefix string) string {
	return fmt.Sprintf(runtimeShimTemplate, prefix)
}

// urlInCSSPattern matches `url(...)` invocations in CSS where the argument is
// a path-absolute URL we want to rewrite. The argument can be optionally
// wrapped in single or double quotes and may be surrounded by whitespace.
//
//	url(/foo)
//	url('/foo')
//	url("/foo")
//
// Network-relative (`//host/foo`) and absolute (`http://...`) are left alone.
var urlInCSSPattern = regexp.MustCompile(`url\(\s*(['"]?)(/[^/'"][^'")]*)`)

// importInCSSPattern matches `@import "/foo";` style root-absolute imports.
var importInCSSPattern = regexp.MustCompile(`@import\s+(['"])(/[^/'"][^'"]*)['"]`)

// rewriteProxyResponse mutates an `http.Response` from agentctl in place,
// rewriting root-absolute URLs to be prefixed by `proxyPrefix` so the iframe's
// asset/XHR/import requests come back through the same port proxy instead of
// hitting the host page's origin. Returns nil and leaves the response untouched
// for content types we don't rewrite (everything except HTML and CSS today).
//
// `proxyPrefix` is the public URL path that fronts this proxy on the gateway,
// e.g. "/port-proxy/<sessionId>/<port>" (no trailing slash). It is prepended to
// matched URLs that start with a single "/" — see `rewriteAbsolutePath`.
func rewriteProxyResponse(resp *http.Response, proxyPrefix string) error {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	var rewrite func([]byte, string) []byte
	switch {
	case strings.Contains(ct, "text/html"):
		rewrite = rewriteHTMLURLs
	case strings.Contains(ct, "text/css"):
		rewrite = rewriteCSSURLs
	default:
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	modified := rewrite(body, proxyPrefix)
	resp.Body = io.NopCloser(bytes.NewReader(modified))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
	resp.ContentLength = int64(len(modified))
	return nil
}

// rewriteAbsolutePath turns a path-absolute URL ("/foo") into a proxied path
// ("<prefix>/foo"). Returns the input unchanged for non-rewritable cases:
// empty strings, network-relative URLs (`//host`), schemes (`http:`, `data:`,
// `mailto:`, etc.), or relative paths (`foo`, `./foo`, `../foo`).
func rewriteAbsolutePath(rawURL, prefix string) string {
	if len(rawURL) < 1 || rawURL[0] != '/' {
		return rawURL
	}
	if len(rawURL) >= 2 && rawURL[1] == '/' {
		return rawURL // network-relative
	}
	return prefix + rawURL
}

// rewriteHTMLURLs walks the HTML document and rewrites every rewritable URL
// attribute (`href`, `src`, …) plus `srcset` values and inline `style="…"`
// `url(...)` references. `<style>` and `<script>` element contents are left
// alone (they're handled by `rewriteCSSURLs` and the runtime shim respectively
// — see phase 4).
//
// Falls back to returning the input unchanged if tokenization fails midway, so
// a malformed page never blocks the response.
func rewriteHTMLURLs(body []byte, prefix string) []byte {
	tok := html.NewTokenizer(bytes.NewReader(body))
	var out bytes.Buffer
	out.Grow(len(body) + 256 + len(runtimeShimTemplate))
	shim := runtimeShim(prefix)
	// Track whether we're inside a `<style>` block so we can rewrite CSS
	// `url(...)` references in its TextToken body. The HTML tokenizer emits
	// the entire body of a `<style>` (or `<script>`) element as a single
	// TextToken; we leave `<script>` contents alone — the runtime shim is the
	// one and only piece of script we inject, and it has no URLs to rewrite.
	inStyle := false
	shimInjected := false
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			if tok.Err() == io.EOF {
				return out.Bytes()
			}
			return body
		}
		token := tok.Token()
		switch token.Type {
		case html.StartTagToken:
			rewriteTokenURLs(&token, prefix)
			out.WriteString(token.String())
			if !shimInjected && token.Data == headTagName {
				// Inject right after `<head>` opens so the shim installs before
				// any other script in the document can execute.
				out.WriteString(shim)
				shimInjected = true
			}
			if token.Data == styleTagName {
				inStyle = true
			}
		case html.SelfClosingTagToken:
			rewriteTokenURLs(&token, prefix)
			out.WriteString(token.String())
		case html.EndTagToken:
			if token.Data == styleTagName {
				inStyle = false
			}
			out.WriteString(token.String())
		case html.TextToken:
			if inStyle {
				out.WriteString(rewriteCSSFragment(token.Data, prefix))
			} else {
				out.WriteString(token.String())
			}
		default:
			out.WriteString(token.String())
		}
	}
}

// rewriteTokenURLs walks a single token's attributes and rewrites any URL-
// shaped attribute value in place.
func rewriteTokenURLs(token *html.Token, prefix string) {
	if token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken {
		return
	}
	for i, attr := range token.Attr {
		key := strings.ToLower(attr.Key)
		switch {
		case rewritableURLAttrs[key]:
			token.Attr[i].Val = rewriteAbsolutePath(attr.Val, prefix)
		case key == "srcset":
			token.Attr[i].Val = rewriteSrcSet(attr.Val, prefix)
		case key == "style":
			token.Attr[i].Val = rewriteCSSFragment(attr.Val, prefix)
		}
	}
}

// rewriteSrcSet rewrites each candidate URL in a `srcset` attribute. The
// value format is `url [descriptor], url [descriptor], …` per the HTML spec.
func rewriteSrcSet(value, prefix string) string {
	parts := strings.Split(value, ",")
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		// Split into URL and optional descriptor (separated by whitespace).
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		fields[0] = rewriteAbsolutePath(fields[0], prefix)
		parts[i] = strings.Join(fields, " ")
	}
	return strings.Join(parts, ", ")
}

// rewriteCSSURLs rewrites url(/...) and @import "/..." occurrences inside a
// standalone CSS document.
func rewriteCSSURLs(body []byte, prefix string) []byte {
	return []byte(rewriteCSSFragment(string(body), prefix))
}

// rewriteCSSFragment rewrites CSS URL references inside an arbitrary string
// (either a full CSS file or the contents of an inline style attribute).
func rewriteCSSFragment(css, prefix string) string {
	css = urlInCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := urlInCSSPattern.FindStringSubmatch(match)
		// sub: full match, quote, url
		if len(sub) != 3 {
			return match
		}
		return "url(" + sub[1] + rewriteAbsolutePath(sub[2], prefix)
	})
	css = importInCSSPattern.ReplaceAllStringFunc(css, func(match string) string {
		sub := importInCSSPattern.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		return "@import " + sub[1] + rewriteAbsolutePath(sub[2], prefix) + sub[1]
	})
	return css
}
