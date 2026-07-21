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

// scriptTagName is a raw-text element per the HTML spec — its contents are
// not HTML-escaped. The golang.org/x/net/html tokenizer correctly returns the
// raw bytes in `Token.Data`, but `Token.String()` blindly HTML-escapes any
// text token's `Data`, which corrupts inline JS containing characters like
// `&` (e.g. `x & 1`, `a && b`, JSON with `&` in strings). We therefore emit
// `<script>` bodies via the raw `Data` directly. We don't rewrite anything
// inside a script body — the runtime shim handles network-facing APIs at
// runtime, which is more reliable than trying to parse JS statically.
const scriptTagName = "script"

// headTagName is the HTML element after whose opening tag we inject the proxy
// runtime shim, so it installs `fetch`/`XHR`/`WebSocket` overrides before any
// user JS can run.
const headTagName = "head"

// runtimeShimTemplate is the JS bootstrap injected at the top of `<head>` for
// every HTML response we proxy. It overrides the network-facing browser APIs
// (fetch, XMLHttpRequest, WebSocket) plus the URL-mutating navigation APIs
// (history.pushState/replaceState, location.assign/replace) so root-absolute
// URLs requested at runtime — e.g. Next.js dynamic chunk imports, fetch calls,
// HMR WebSocket, and SPA-router pushState transitions — stay on the same proxy
// chain instead of escaping to the host origin.
//
// %q is replaced per-response with the proxy path (no trailing slash) using
// `fmt.Sprintf`, which emits a JS-safe double-quoted string literal.
//
// Concatenated across multiple Go strings only for readability; the resulting
// JS is still a single self-invoking expression with no internal whitespace.
const runtimeShimTemplate = `<script>(function(){` +
	`var P=%q;` +
	`window.__kandevProxyPrefix=P;` +
	// Path rewriter: prefix path-absolute URLs that aren't already prefixed.
	`function r(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)!=='/'||(u.length>1&&u.charAt(1)==='/'))return u;if(u.indexOf(P)===0)return u;return P+u;}` +
	// fetch — string and Request-object input forms.
	`var of=window.fetch;if(of){window.fetch=function(i,n){if(typeof i==='string')i=r(i);else if(i&&typeof i==='object'&&typeof i.url==='string'){var nu=r(i.url);if(nu!==i.url){try{i=new Request(nu,i)}catch(e){}}}return of.call(this,i,n)}}` +
	// XMLHttpRequest.open — 2nd arg is the URL.
	`var oo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){arguments[1]=r(u);return oo.apply(this,arguments)};` +
	// WebSocket — path-absolute ws/wss URLs need an explicit ws:// scheme + host since the constructor doesn't accept bare paths.
	`var OW=window.WebSocket;if(OW){function W(u,p){if(typeof u==='string'&&u.charAt(0)==='/'&&(u.length<2||u.charAt(1)!=='/')){var l=window.location;u=(l.protocol==='https:'?'wss:':'ws:')+'//'+l.host+P+u}return p?new OW(u,p):new OW(u)}W.prototype=OW.prototype;Object.getOwnPropertyNames(OW).forEach(function(k){try{W[k]=OW[k]}catch(e){}});window.WebSocket=W}` +
	// history.pushState / replaceState — SPA routers (Next.js, React Router, etc.) call these to change the URL on client-side navigation. Without rewriting, the URL bar drops the proxy prefix and a reload 404s.
	`['pushState','replaceState'].forEach(function(op){var orig=history[op];if(!orig)return;history[op]=function(s,t,u){if(typeof u==='string')u=r(u);return orig.call(this,s,t,u)}});` +
	// location.assign / location.replace — direct navigation APIs. (Assigning to location.href cannot be intercepted on a same-origin window without redefining a non-configurable property, so we don't try; pushState covers the common SPA-router path.)
	`['assign','replace'].forEach(function(op){var orig=location[op];if(!orig)return;try{location[op]=function(u){if(typeof u==='string')u=r(u);return orig.call(location,u)}}catch(e){}});` +
	// MutationObserver: rewrite root-absolute URL attributes on every element that is inserted or has its URL attribute mutated. Covers the cases the network-API patches miss, notably `ReactDOM.preload()` and any framework that builds DOM nodes with absolute paths after the initial HTML has been parsed.
	`var ATTRS=['href','src','action','formaction','cite','data','poster','background','manifest','srcset'];` +
	`function rwa(el,a){if(!el.getAttribute||!el.hasAttribute(a))return;var v=el.getAttribute(a);if(typeof v!=='string')return;var nv;if(a==='srcset'){nv=v.split(',').map(function(p){var f=p.trim().split(/\s+/);if(f[0])f[0]=r(f[0]);return f.join(' ')}).join(', ')}else{nv=r(v)}if(nv!==v)el.setAttribute(a,nv)}` +
	`function rwe(el){if(!el||el.nodeType!==1)return;for(var i=0;i<ATTRS.length;i++)rwa(el,ATTRS[i])}` +
	`var MO=window.MutationObserver;if(MO&&document.documentElement){try{new MO(function(rs){for(var i=0;i<rs.length;i++){var rec=rs[i];if(rec.type==='attributes'){rwe(rec.target)}else{for(var j=0;j<rec.addedNodes.length;j++){var n=rec.addedNodes[j];rwe(n);if(n.querySelectorAll){var nl=n.querySelectorAll('[href],[src],[action],[srcset],[poster]');for(var k=0;k<nl.length;k++)rwe(nl[k])}}}}}).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:ATTRS})}catch(e){}}` +
	// Console forwarding: pipe iframe console output back to the parent frame via postMessage so it surfaces in the kandev UI alongside other preview events. Errors and stacks are coerced to strings; objects are JSON-cloned where possible. We continue calling the original method so the iframe's own DevTools still shows everything.
	`var LV=['log','warn','error','info','debug'];LV.forEach(function(lv){var orig=console[lv];if(!orig)return;console[lv]=function(){try{var out=[];for(var i=0;i<arguments.length;i++){var a=arguments[i];if(a instanceof Error){out.push('Error: '+a.message+(a.stack?'\n'+a.stack:''))}else if(typeof a==='object'&&a!==null){try{out.push(JSON.parse(JSON.stringify(a)))}catch(e){out.push(String(a))}}else{out.push(a)}}window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}},'*')}catch(e){}return orig.apply(console,arguments)}});` +
	`})();</script>`

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

// htmlRewriteState tracks per-document position during a single
// `rewriteHTMLURLs` pass. Pulled out to its own type so each token-handling
// case stays short and the top-level loop's cyclomatic complexity stays under
// the lint budget.
type htmlRewriteState struct {
	out          *bytes.Buffer
	prefix       string
	shim         string
	inStyle      bool
	inScript     bool
	shimInjected bool
}

// onStartTag emits the (URL-rewritten) start tag and updates raw-text
// element tracking. Also injects the runtime shim immediately after `<head>`.
func (s *htmlRewriteState) onStartTag(token html.Token) {
	rewriteTokenURLs(&token, s.prefix)
	s.out.WriteString(token.String())
	if !s.shimInjected && token.Data == headTagName {
		s.out.WriteString(s.shim)
		s.shimInjected = true
	}
	switch token.Data {
	case styleTagName:
		s.inStyle = true
	case scriptTagName:
		s.inScript = true
	}
}

// onEndTag clears raw-text element tracking, then writes the end tag.
func (s *htmlRewriteState) onEndTag(token html.Token) {
	switch token.Data {
	case styleTagName:
		s.inStyle = false
	case scriptTagName:
		s.inScript = false
	}
	s.out.WriteString(token.String())
}

// onTextToken writes text content with the right escaping for the current
// element context: rewrite CSS URLs inside `<style>`, emit raw bytes inside
// `<script>` (Token.String would HTML-escape and corrupt JS), and otherwise
// fall back to the default Token.String() entity-encoding.
func (s *htmlRewriteState) onTextToken(token html.Token) {
	switch {
	case s.inStyle:
		s.out.WriteString(rewriteCSSFragment(token.Data, s.prefix))
	case s.inScript:
		s.out.WriteString(token.Data)
	default:
		s.out.WriteString(token.String())
	}
}

// rewriteHTMLURLs walks the HTML document and rewrites every rewritable URL
// attribute (`href`, `src`, …) plus `srcset` values and inline `style="…"`
// `url(...)` references. `<style>` content is run through the CSS URL
// rewriter; `<script>` content is emitted unchanged (the runtime shim
// handles network-facing APIs at runtime).
//
// Falls back to returning the input unchanged if tokenization fails midway, so
// a malformed page never blocks the response.
func rewriteHTMLURLs(body []byte, prefix string) []byte {
	tok := html.NewTokenizer(bytes.NewReader(body))
	var out bytes.Buffer
	out.Grow(len(body) + 256 + len(runtimeShimTemplate))
	s := &htmlRewriteState{out: &out, prefix: prefix, shim: runtimeShim(prefix)}
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
			s.onStartTag(token)
		case html.SelfClosingTagToken:
			rewriteTokenURLs(&token, prefix)
			out.WriteString(token.String())
		case html.EndTagToken:
			s.onEndTag(token)
		case html.TextToken:
			s.onTextToken(token)
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
