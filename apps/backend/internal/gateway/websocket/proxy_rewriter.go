package websocket

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// URL attributes that may carry a path-absolute reference and therefore need
// the proxy path prefix when the document is served through the port proxy.
// Source: https://html.spec.whatwg.org/multipage/indices.html#attributes-3
// `cite` is intentionally NOT here: the browser never fetches it (it is
// copyable metadata on q/blockquote/del/ins), so it is rewritten prefix-only
// like navigation references — a capability in a cite value would sit in
// DOM/copyable metadata for no benefit. See rewriteTokenAttr.
var rewritableURLAttrs = map[string]bool{
	"href":       true,
	"src":        true,
	"action":     true,
	"formaction": true,
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
const runtimeShimTemplate = `(function(){` +
	`var P=%q;` +
	`window.__kandevProxyPrefix=P;` +
	// Path rewriter: prefix path-absolute URLs that aren't already prefixed.
	// The prefix check is boundary-exact (`u===P` or the next byte is
	// `/`, `?`, `#`), so `/port-proxy/s/51730/foo` is NOT mistaken for the
	// `/port-proxy/s/5173` subtree and an already-prefixed URL still falls
	// through to the capability splice. The emitted form is WHATWG-normalized
	// (leading C0/space trimmed, embedded tabs/newlines removed) AFTER the
	// URL-API classification passes, matching what the browser resolves: a
	// space-prefixed dynamic src cannot escape the subtree when the browser
	// trims it at fetch time. %s is the optional capability-append logic
	// (empty when no capability is minted, keeping the auth-disabled output
	// free of capability bytes).
	`function r(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#'||u.indexOf('//')===0)return u;try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}if(nz(u).charAt(0)!=='/'&&!insub(ru))return u;u=nz(u);if(u.charAt(0)==='/'){if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u;if(u.indexOf('/.')!==-1){var dd=rd(u);if(!(dd===P||dd.indexOf(P+'/')===0||dd.indexOf(P+'?')===0||dd.indexOf(P+'#')===0))return u;u=dd}}%sreturn u;}` +
	// insub(ru): reports whether a resolved URL's path is boundary-inside
	// the proxy subtree. Root-absolute references are always rewritten INTO
	// the subtree by the prefix, but relative references resolve against the
	// document base and absolute references resolve as-is: if either lands
	// outside P (e.g. a same-origin <base href="http://gw/outside/">), the
	// capability must not be appended (it would ride the bearer to a
	// non-proxy gateway endpoint).
	`function insub(ru){var p=ru.pathname;return p===P||p.indexOf(P+'/')===0||p.indexOf(P+'?')===0||p.indexOf(P+'#')===0}` +
	// Navigation rewriter: same prefixing WITHOUT the capability. Navigation
	// APIs (history.pushState, location.assign) put the URL in the address bar
	// and browser history; embedding a bearer there would leak it through
	// copied URLs, history, and cross-origin Referers. The subtree cookie
	// covers same-origin navigations instead.
	`function nz(u){return u.replace(/[\\\t\n\r]/g,function(m){return m==='\\'?'/':''}).replace(/^[\u0000-\u0020]+|[\u0000-\u0020]+$/g,'')}` +
	// rd(p): RFC 3986 dot-segment removal (used after prefixing so a
	// root-absolute /../x cannot escape the subtree with a capability).
	`function rd(p){var segs=p.split('/');var out=[];var trailing=/\/$/.test(p)||/\.$/.test(p)||/\.\.$/.test(p);for(var i=0;i<segs.length;i++){var g=segs[i];if(g==='.'){continue}if(g==='..'){if(out.length>1||out.length===1&&out[0]!=='')out.pop();continue}out.push(g)}var j=out.join('/');if(trailing&&j.charAt(j.length-1)!=='/')j+='/';return j}` +
	// sc(u): strips every query parameter whose DECODED key equals the
	// reserved capability key (so percent-encoded key spellings are stripped
	// too), preserving all other parameters AND any URL fragment (SPA
	// hash-state must survive navigation rewriting).
	`function sc(u){var h='';var hi=u.indexOf('#');if(hi!==-1){h=u.slice(hi);u=u.slice(0,hi)}var qi=u.indexOf('?');if(qi===-1)return u+h;var b=u.slice(0,qi);var q=u.slice(qi+1).split('&').filter(function(p){var k=p.split('=')[0];var d;try{d=decodeURIComponent(k)}catch(e){d=k}return d!=='kandev_cap'});if(q.length===0)return b+'?'+h;return b+'?'+q.join('&')+h}` +
	// rn(u): prefix-only navigation rewriter. It strips any previously issued
	// capability from EVERY same-origin form (relative, root-absolute, and
	// absolute http/https — a fetching link reclassified to metadata must not
	// retain its bearer) before applying the boundary-exact prefix logic.
	// Cross-origin, network-relative, other schemes, and fragments pass
	// through untouched; classification uses the URL API like r() so
	// whitespace/control-obfuscated externals are never touched, and the
	// emitted form is WHATWG-normalized (leading C0/space trimmed, embedded
	// tabs/newlines removed) so a space-prefixed navigation cannot escape the
	// subtree when the browser trims it.
	`function rn(u){if(typeof u!=='string')return u;if(!u||u.charAt(0)==='#')return u;if(u.indexOf('//')===0)return u;try{var ru=new URL(u,(typeof document!=='undefined'&&document.baseURI)||window.location.href);if(ru.protocol!=='http:'&&ru.protocol!=='https:'||ru.origin!==window.location.origin)return u}catch(e){return u}u=nz(u);if(u.charAt(0)==='/'){u=sc(u);if(!(u===P||u.indexOf(P+'/')===0||u.indexOf(P+'?')===0||u.indexOf(P+'#')===0))u=P+u;if(u.indexOf('/.')!==-1){var dd2=rd(u);if(!(dd2===P||dd2.indexOf(P+'/')===0||dd2.indexOf(P+'?')===0||dd2.indexOf(P+'#')===0))return u;u=dd2}return u}if(/^[a-z][a-z0-9+.-]*:\/\//i.test(u)){var o=ru.origin;var pn=ru.pathname;var tail=sc(pn+(ru.search||''));if(!(tail===P||tail.indexOf(P+'/')===0||tail.indexOf(P+'?')===0||tail.indexOf(P+'#')===0))tail=P+tail;return o+tail+(ru.hash||'')}return sc(u)}` +
	// norm(u): normalizes any URL-like input through the network rewriter
	// using the URL API, which applies the WHATWG parsing rules (leading
	// C0/space trimmed, embedded tabs/newlines removed, default ports and
	// userinfo normalized). Only http/https/ws/wss inputs that resolve to the
	// page's origin (the gateway, in an iframe) get their pathname rewritten
	// through r() with scheme+host preserved; cross-origin inputs, other
	// schemes (blob:, data:, file:, about:), and non-URL inputs pass through
	// unchanged. ws:/wss: origins are compared as http:/https: so WebSocket
	// URLs match the page origin.
	`function norm(u){var s=typeof u==='string'?u:(u&&typeof u==='object'&&typeof u.href==='string'?u.href:null);if(s===null)return u;if(typeof s==='string'&&s.indexOf('//')===0)return u;var x;try{x=new URL(s,(typeof document!=='undefined'&&document.baseURI)||window.location.href)}catch(e){return u}var p=x.protocol;if(p!=='http:'&&p!=='https:'&&p!=='ws:'&&p!=='wss:')return u;if(x.username||x.password)return u;var o=x.origin.replace(/^ws:/,'http:').replace(/^wss:/,'https:');if(o!==window.location.origin)return u;if(nz(s).charAt(0)!=='/'&&!insub(x))return u;return x.origin+r(x.pathname+(x.search||'')+(x.hash||''));}` +
	// fetch — string, real URL-object, and real Request-object input forms.
	// Only branded URL/Request instances are rewritten: native fetch accepts
	// only USVString or Request, and plain duck-typed {href}/{url} objects
	// (including Symbol.toStringTag spoofs) must be left untouched so native
	// coercion produces its usual TypeError rather than a shim-initiated
	// request. The brand is verified by invoking the prototype accessor with
	// Reflect.get: it reads the internal slot (so it works cross-realm) and
	// throws on objects without one (so it cannot be spoofed).
	`var of=window.fetch;if(of){window.fetch=function(i,n){if(typeof i==='string')i=norm(i);else if(i&&typeof i==='object'){var ib=false;try{Reflect.get(URL.prototype,'href',i);ib=true}catch(e){}if(ib&&!i.username&&!i.password){var hu=norm(i.href);if(hu!==i.href){try{i=new URL(hu)}catch(e){}}}else if(!ib){try{Reflect.get(Request.prototype,'url',i);ib=true}catch(e){}if(ib){var nu=norm(i.url);if(nu!==i.url){try{i=new Request(nu,i)}catch(e){}}}}}return of.call(this,i,n)}}` +
	// XMLHttpRequest.open — 2nd arg is the URL (string, URL object, or any
	// DOMString-able value; native Web IDL stringifies it AFTER the wrapper,
	// so the shim must convert first to rewrite it). Symbols throw like
	// native DOMString conversion; null/undefined pass through.
	`var oo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(m,u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=norm(u)}arguments[1]=u;return oo.apply(this,arguments)};` +
	// WebSocket — path-absolute ws/wss URLs need an explicit ws:// scheme + host since the constructor doesn't accept bare paths. The URL argument may be any DOMString-able value (native stringifies after the wrapper), so it is converted to a string FIRST and then normalized; Symbols throw like native DOMString conversion; null/undefined pass through. Same-origin results are converted to the matching ws/wss scheme, fragments are dropped (WebSocket URLs cannot carry one), and network-relative and cross-origin inputs pass through untouched.
	`var OW=window.WebSocket;if(OW){function W(u,p){if(!new.target)throw new TypeError("Failed to construct 'WebSocket': Please use the 'new' operator, this DOM object constructor cannot be called as a function.");if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u)}var n=norm(u);var l=window.location;var w=(l.protocol==='https:'?'wss:':'ws:')+'//'+l.host;var s=n;if(typeof s==='string'){if(s.charAt(0)==='/'&&s.charAt(1)!=='/'){s=w+s}else if(s!==u&&/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i.test(s)){s=s.replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i,w)}var h=s.indexOf('#');if(h!==-1)s=s.slice(0,h)}if(arguments.length>1)return Reflect.construct(OW,[s,p],new.target||W);return Reflect.construct(OW,[s],new.target||W)}W.prototype=OW.prototype;Object.getOwnPropertyNames(OW).forEach(function(k){try{W[k]=OW[k]}catch(e){}});window.WebSocket=W}` +
	// history.pushState / replaceState — SPA routers (Next.js, React Router, etc.) call these to change the URL on client-side navigation. Without rewriting, the URL bar drops the proxy prefix and a reload 404s. Uses rn() (no capability — see above).
	`['pushState','replaceState'].forEach(function(op){var orig=history[op];if(!orig)return;history[op]=function(s,t,u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(this,s,t,u)}});` +
	// location.assign / location.replace — direct navigation APIs, patched
	// best-effort with the prefix-only rewriter (string and URL-object
	// arguments are both normalized, since the native API stringifies URL
	// objects to absolute hrefs). Chromium exposes these as non-writable own
	// properties, so the patch can silently no-op there.
	// Anchor navigation needs no interception: static and MutationObserver
	// rewriting already prefix anchor hrefs, so the browser's own default
	// navigation stays inside the proxy subtree and application click
	// handlers (including delegated document listeners) keep full control.
	// Known limitation: an anchor created and clicked programmatically
	// (anchor.click()) in the same task can navigate before the
	// MutationObserver microtask rewrites its href; ordinary user clicks are
	// unaffected because the observer delivers before the next task.
	`['assign','replace'].forEach(function(op){var orig=location[op];if(!orig)return;try{location[op]=function(u){if(u!==null&&u!==undefined){if(typeof u==='symbol')throw new TypeError('Cannot convert a Symbol value to a string');if(typeof u!=='string')u=String(u);u=rn(u)}return orig.call(location,u)}}catch(e){}});` +
	// MutationObserver: rewrite URL attributes on every element that is
	// inserted or has its URL attribute mutated. Navigation attributes
	// (anchor/area/base href, form action, button/input formaction, metadata
	// links with no fetching rel token) use the prefix-only rewriter (and any
	// previously issued capability is stripped); subresource attributes use
	// the capability-bearing one. Dynamic style attributes get their
	// root-absolute url() references rewritten, and dynamic meta-refresh
	// content gets its url= target prefixed. Dynamic iframe srcdoc is not
	// runtime-rewritten (static-only, documented): its children inherit the
	// subtree cookies, so only cookie-less dynamic srcdoc escapes — an
	// accepted edge. Covers the cases the network-API patches miss, notably
	// `ReactDOM.preload()` and any framework that builds DOM nodes with
	// absolute paths after the initial HTML has been parsed.
	`var ATTRS=['href','src','action','formaction','cite','data','poster','background','manifest','srcset','imagesrcset','ping','style','content'];` +
	`function lf(el){var rel=el.rel;if(typeof rel!=='string')return true;var toks=rel.toLowerCase().split(/\s+/);for(var i=0;i<toks.length;i++){if(toks[i]==='stylesheet'||toks[i]==='icon'||toks[i]==='manifest'||toks[i]==='preload'||toks[i]==='modulepreload'||toks[i]==='prefetch'||toks[i]==='dns-prefetch'||toks[i]==='preconnect'||toks[i]==='shortcut'||toks[i]==='apple-touch-icon')return true}return false}` +
	`function cssEsc(s,i){if(s.charAt(i)!=='\\'||i+1>=s.length)return null;var c=s.charAt(i+1);if(/[0-9a-fA-F]/.test(c)){var j=i+1;var v=0;while(j<s.length&&j-i<7&&/[0-9a-fA-F]/.test(s.charAt(j))){v=v*16+parseInt(s.charAt(j),16);j++}if(j<s.length&&' \t\n\r\f'.indexOf(s.charAt(j))>=0)j++;if(v===0||v>0x10FFFF||v>=0xD800&&v<=0xDFFF)v=0xFFFD;return{ch:String.fromCodePoint(v),adv:j-i}}if(c==='\n'||c==='\r'||c==='\f')return{ch:'',adv:2};return{ch:c,adv:2}}` +
	`function cssFn(s,i,nm){var j=i;var b='';var first=true;while(j<s.length){var cc=s.charAt(j);if(cc==='\\'){var e=cssEsc(s,j);if(!e||e.ch==='')break;if(!first&&!/[a-zA-Z0-9_\-\u0080-\uFFFF]/.test(e.ch))break;b+=e.ch;j+=e.adv}else if(first&&/[a-zA-Z_\u0080-\uFFFF]/.test(cc)){b+=cc;j++}else if(!first&&/[a-zA-Z0-9_\-\u0080-\uFFFF]/.test(cc)){b+=cc;j++}else break;first=false}if(j===i)return -1;if(b.toLowerCase()!==nm||s.charAt(j)!=='(')return -1;return j+1}` +
	`function cssDecTok(t){var o='';var i=0;while(i<t.length){var c=t.charAt(i);if(c==='\\'){var e=cssEsc(t,i);if(!e)break;o+=e.ch;i+=e.adv}else{o+=c;i++}}return o}` +
	`function cssEscTok(t){var o='';var i=0;while(i<t.length){var c=t.charAt(i);if(c==='\\'||c==='\''||c==='"'||c==='('||c===')'||c===' '||c==='\t'||c==='\n'||c==='\r'||c==='\f'||c.charCodeAt(0)<0x20||c.charCodeAt(0)===0x7F){var h=c.charCodeAt(0).toString(16);if(h.length===1)h='0'+h;o+='\\'+h+' '}else{o+=c}i++}return o}` +
	`function rwaStyle(v){var o='';var i=0;var n=v.length;var j2;while(i<n){var c=v.charAt(i);if((c==='u'||c==='U'||c==='\\')&&!(i>0&&(v.charCodeAt(i-1)>=128||/[a-zA-Z0-9_-]/.test(v.charAt(i-1))))&&(j2=cssFn(v,i,'url'))>=0){var jw=j2;while(jw<n&&' \t\n\r\f'.indexOf(v.charAt(jw))>=0)jw++;var sp=v.slice(j2,jw);if(jw<n&&(v.charAt(jw)==='\''||v.charAt(jw)==='"')){var q=v.charAt(jw);var ce=jw+1;while(ce<n){if(v.charAt(ce)==='\\'&&ce+1<n){ce+=2;continue}if(v.charAt(ce)===q)break;ce++}if(ce>=n){o+=v.slice(i);break}var tok=v.slice(jw+1,ce);var d1=cssDecTok(tok);var rw1=r(d1);var em1=d1===rw1?tok:cssEscTok(rw1);var k=ce+1;while(k<n&&' \t\n\r\f'.indexOf(v.charAt(k))>=0)k++;if(k<n&&v.charAt(k)===')'){o+='url('+sp+q+em1+q+v.slice(ce+1,k)+')';i=k+1;continue}o+=v.slice(i,ce+1);i=ce+1;continue}var k2=jw;while(k2<n&&v.charAt(k2)!==')'){if(v.charAt(k2)==='\\'){var ee=cssEsc(v,k2);if(!ee)break;k2+=ee.adv;continue}if(' \t\n\r\f'.indexOf(v.charAt(k2))>=0)break;k2++}var tok2=v.slice(jw,k2);var d2=cssDecTok(tok2);var l2=k2;while(l2<n&&' \t\n\r\f'.indexOf(v.charAt(l2))>=0)l2++;if(l2<n&&v.charAt(l2)===')'){if(d2.toLowerCase().indexOf('var(')!==0){var rw2=r(d2);var em2=d2===rw2?tok2:cssEscTok(rw2);o+='url('+sp+em2+v.slice(k2,l2)+')';i=l2+1;continue}o+=v.slice(i,l2+1);i=l2+1;continue}o+=v.slice(i,k2);i=k2;continue}if(c==='\\'&&i+1<n){o+=v.slice(i,i+2);i+=2;continue}if(c==='\''||c==='"'){var j=i+1;while(j<n){if(v.charAt(j)==='\\'&&j+1<n){j+=2;continue}if(v.charAt(j)===c)break;j++}if(j>=n){o+=v.slice(i);break}o+=v.slice(i,j+1);i=j+1;continue}o+=c;i++}return o}` +
	`function srcsetParts(v){var parts=[];var cur='';var inData=false;var curEmpty=true;for(var i=0;i<v.length;i++){var c=v.charAt(i);if(c===','){if(inData){cur+=c;continue}parts.push(cur);cur='';curEmpty=true;inData=false;continue}if(!inData&&curEmpty&&c!==' '&&c!=='\t'&&c!=='\n'&&c!=='\r'&&c!=='\f'&&v.slice(i,i+5).toLowerCase()==='data:')inData=true;if(inData&&(c===' '||c==='\t'||c==='\n'||c==='\r'||c==='\f'))inData=false;if(c!==' '&&c!=='\t'&&c!=='\n'&&c!=='\r'&&c!=='\f')curEmpty=false;cur+=c}if(!curEmpty)parts.push(cur);return parts}` +
	`function mref(v){var n=v.length;var mi=-1;var from=0;while(from<=n){var semi=v.indexOf(';',from);var end=semi<0?n:semi;var field=v.slice(from,end);var k=0;while(k<field.length&&' \t\n\r\f'.indexOf(field.charAt(k))>=0)k++;if(field.slice(k,k+4).toLowerCase()==='url='){mi=from+k;break}if(semi<0)break;from=semi+1}if(mi<0)return v;var after=v.slice(mi+4);var lead=after.replace(/^[ \t\n\r\f]+/,'');var off=after.length-lead.length;var t=lead;var q='';var rest='';if(t.charAt(0)==='\''||t.charAt(0)==='"'){q=t.charAt(0);var e=t.indexOf(q,1);if(e<0)return v;t=t.slice(1,e);rest=lead.slice(e)}else{var sp=t.search(/[; \t\n\r\f]/);if(sp>=0){t=t.slice(0,sp);rest=lead.slice(sp)}}return v.slice(0,mi+4)+after.slice(0,off)+q+rn(t)+rest}` +
	`function rwa(el,a){if(!el.getAttribute||!el.hasAttribute(a))return;var v=el.getAttribute(a);if(typeof v!=='string')return;var nav=(a==='href'&&(el.tagName==='A'||el.tagName==='AREA'||el.tagName==='BASE'||(el.tagName==='LINK'&&!lf(el))))||(a==='action'&&el.tagName==='FORM')||(a==='formaction'&&(el.tagName==='BUTTON'||el.tagName==='INPUT'))||a==='cite';var rr=nav?rn:r;var nv=v;if(a==='srcset'||a==='imagesrcset'){nv=srcsetParts(v).map(function(p){var f=p.trim().split(/\s+/);if(f[0])f[0]=rr(f[0]);return f.join(' ')}).join(', ')}else if(a==='ping'){nv=v.split(/\s+/).map(function(u){return r(u)}).join(' ')}else if(a==='style'){nv=rwaStyle(v)}else if(a==='content'){if(el.tagName==='META'){var he=el.getAttribute('http-equiv');if(he&&String(he).trim().toLowerCase()==='refresh')nv=mref(v)}}else{nv=rr(v)}if(nv!==v)el.setAttribute(a,nv)}` +
	`function rwe(el){if(!el||el.nodeType!==1)return;for(var i=0;i<ATTRS.length;i++)rwa(el,ATTRS[i])}` +
	`var OBS=['href','src','action','formaction','cite','data','poster','background','manifest','srcset','imagesrcset','ping','rel','style','content','http-equiv'];` +
	`var MO=window.MutationObserver;if(MO&&document.documentElement){try{new MO(function(rs){for(var i=0;i<rs.length;i++){var rec=rs[i];if(rec.type==='attributes'){rwe(rec.target)}else{for(var j=0;j<rec.addedNodes.length;j++){var n=rec.addedNodes[j];rwe(n);if(n.querySelectorAll){var nl=n.querySelectorAll('[href],[src],[action],[formaction],[cite],[data],[poster],[background],[manifest],[srcset],[imagesrcset],[ping],[style],[content]');for(var k=0;k<nl.length;k++)rwe(nl[k])}}}}}).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:OBS})}catch(e){}}` +
	// Console forwarding: announce readiness without console data, then bind to
	// the parent origin from an explicit response. This supports a UI and gateway
	// on different origins while keeping console data off wildcard messages.
	`var PO='';window.addEventListener('message',function(e){try{var d=e.data;if(e.source===window.parent&&d&&d.source==='kandev-inspector'&&d.type==='console-bind'&&e.origin)PO=e.origin}catch(x){}});try{window.parent.postMessage({source:'kandev-inspector',type:'console-ready',payload:{}},'*')}catch(e){}` +
	`var LV=['log','warn','error','info','debug'];LV.forEach(function(lv){var orig=console[lv];if(!orig)return;console[lv]=function(){try{var out=[];for(var i=0;i<arguments.length;i++){var a=arguments[i];if(a instanceof Error){out.push('Error: '+a.message+(a.stack?'\n'+a.stack:''))}else if(typeof a==='object'&&a!==null){try{out.push(JSON.parse(JSON.stringify(a)))}catch(e){out.push(String(a))}}else{out.push(a)}}if(PO)window.parent.postMessage({source:'kandev-inspector',type:'console',payload:{level:lv,args:out}},PO)}catch(e){}return orig.apply(console,arguments)}});` +
	`})();`

// runtimeShim returns the runtime shim's JavaScript body with the given proxy
// prefix baked in. %q produces a JS-safe double-quoted string literal (slashes
// and alphanumerics need no escaping, which matches every prefix we emit).
//
// When `capability` is non-empty, the shim's network path rewriter appends the
// subtree capability to every URL it rewrites (fetch/XHR/WebSocket and
// dynamically injected DOM), so those requests stay authorized in contexts
// where cookies are not sent. Navigation APIs (history/location) deliberately
// use a prefix-only rewriter: a bearer in the address bar or history would leak
// through copied URLs and cross-origin Referers, and the subtree cookie covers
// same-origin navigations.
//
// The shim is served as an external same-origin JavaScript file by the port
// proxy (reserved path __kandev_runtime_shim.js) and injected as a <script
// src> tag: an app's Content-Security-Policy (script-src 'self') blocks inline
// scripts but allows a same-origin script.
func runtimeShim(prefix, capability string) string {
	return fmt.Sprintf(runtimeShimTemplate, prefix, shimCapabilityJS(capability))
}

// runtimeShimPath is the reserved per-subtree path the port proxy serves the
// runtime shim JavaScript from.
const runtimeShimPath = "__kandev_runtime_shim.js"

// runtimeShimTag is the <script src> element injected at the top of <head> for
// every HTML response we proxy; the browser loads the shim body from the
// proxy's reserved path. When a capability is minted it rides on the src so
// the shim itself loads even in cookie-less contexts. When the response's
// Content-Security-Policy uses a script nonce, the tag carries it (validated
// against the CSP nonce grammar and HTML-escaped) so nonce- and
// strict-dynamic-based policies still allow the shim.
func runtimeShimTag(prefix, capability, nonce string) string {
	src := prefix + "/" + runtimeShimPath
	if capability != "" {
		src += "?" + proxyCapabilityQueryParam + "=" + capability
	}
	nonceAttr := ""
	if nonce != "" && validCSPNonce(nonce) {
		nonceAttr = ` nonce="` + html.EscapeString(nonce) + `"`
	}
	return `<script src="` + src + `"` + nonceAttr + `></script>`
}

// validCSPNonce accepts only the CSP nonce grammar: base64/base64url
// characters with optional padding. A malformed nonce-shaped token from an
// app-controlled policy must never be interpolated into the injected markup.
func validCSPNonce(nonce string) bool {
	if nonce == "" || len(nonce) > 256 {
		return false
	}
	seenContent := false
	padding := 0
	for i := 0; i < len(nonce); i++ {
		if !cspNonceChar(nonce[i]) {
			return false
		}
		if nonce[i] == '=' {
			padding++
			if padding > 2 {
				return false
			}
		} else {
			if padding > 0 {
				return false // padding only at the very end (ab=c invalid)
			}
			seenContent = true
		}
	}
	// An all-padding token ("=", "==") is not a valid nonce value.
	return seenContent
}

// cspNonceChar reports whether a byte is valid in a CSP nonce (base64 /
// base64url alphabet).
func cspNonceChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '+' || c == '/' || c == '-' || c == '_' || c == '='
}

// cspScriptSrcElem is the CSP3 directive that governs script elements; when
// present it wins over script-src for the injected shim.
const cspScriptSrcElem = "script-src-elem"

// scriptNoncesFromCSP returns the valid script nonces a single
// Content-Security-Policy value requires for script elements: from the
// script-src-elem directive when present (per CSP3 it governs script
// elements), else from script-src. When script-src-elem EXISTS it is the
// governing directive for script elements, so script-src nonces are NEVER
// used — even when the elem directive carries no nonce tokens (a nonce from
// script-src is not authorized by the governing directive). Invalid
// nonce-shaped tokens are skipped so a later valid one is used; multiple
// nonces in one directive are all candidates and the caller intersects
// across policies.
func scriptNoncesFromCSP(policy string) []string {
	var elem, src []string
	hasElem := false
	for _, directive := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(directive))
		if len(fields) == 0 {
			continue
		}
		name := strings.ToLower(fields[0])
		if name != "script-src" && name != cspScriptSrcElem {
			continue
		}
		if name == cspScriptSrcElem {
			hasElem = true
		}
		for _, source := range fields[1:] {
			nonce, ok := strings.CutPrefix(source, "'nonce-")
			if !ok {
				continue
			}
			nonce, ok = strings.CutSuffix(nonce, "'")
			if ok && validCSPNonce(nonce) {
				if name == cspScriptSrcElem {
					elem = append(elem, nonce)
				} else {
					src = append(src, nonce)
				}
			}
		}
	}
	if hasElem {
		return elem
	}
	return src
}

// cspScriptNonceSet returns the nonce set a single CSP policy constrains the
// injected script to, or nil when the policy does not constrain the choice:
// no script nonce in its governing directive means a nonce cannot help (the
// shim must match the policy's other sources), and 'self'/'*' already allow
// the same-origin shim regardless of any nonce attribute. 'strict-dynamic'
// inverts that: it disables 'self' for script elements (CSP3), so a nonce is
// REQUIRED even when 'self' is present. All source expressions are scanned
// before deciding, so 'strict-dynamic' later in the same directive is not
// masked by an earlier 'self'.
func cspScriptNonceSet(policy string) []string {
	nonces := scriptNoncesFromCSP(policy)
	if len(nonces) == 0 {
		return nil
	}
	// The governing directive for script elements is script-src-elem when
	// present, else script-src (CSP3 fallback); a nonce in the non-governing
	// directive does not apply to the shim.
	for _, name := range []string{cspScriptSrcElem, "script-src"} {
		for _, directive := range strings.Split(policy, ";") {
			fields := strings.Fields(strings.TrimSpace(directive))
			if len(fields) == 0 || !strings.EqualFold(fields[0], name) {
				continue
			}
			hasStrictDynamic := false
			hasSelfOrStar := false
			for _, source := range fields[1:] {
				switch source {
				case "'strict-dynamic'":
					hasStrictDynamic = true
				case "'self'", "*":
					hasSelfOrStar = true
				}
			}
			if hasStrictDynamic {
				return nonces // ignores 'self' for scripts; nonce required
			}
			if hasSelfOrStar {
				return nil // shim allowed without a nonce
			}
			return nonces
		}
	}
	return nonces
}

// responseScriptNonce returns a script nonce for the injected shim tag that
// is valid under every ENFORCING Content-Security-Policy for the response —
// all CSP headers plus all CSP meta tags, since meta policies append to
// header policies — or "" when no single nonce satisfies all of them. With
// two enforcing policies using different nonces, a nonce valid under only
// one would still leave the injected shim blocked by the other. When no
// policy constrains the choice, the first valid nonce found is still
// attached: it is harmless (a nonce attribute never disables 'self'/host
// matching in other policies) and preserves nonce-friendly app setups. Meta
// tags are scanned with the HTML tokenizer: attributes are decoded
// (entity-encoded policies work), keys match exactly (data-content decoys do
// not), any attribute order is accepted, and every meta is examined.
func responseScriptNonce(resp *http.Response, body []byte) string {
	var constraining [][]string
	fallback := ""
	collect := func(policy string) {
		if set := cspScriptNonceSet(policy); set != nil {
			constraining = append(constraining, set)
			return
		}
		if fallback == "" {
			if ns := scriptNoncesFromCSP(policy); len(ns) > 0 {
				fallback = ns[0]
			}
		}
	}
	for _, policy := range resp.Header.Values("Content-Security-Policy") {
		collect(policy)
	}
	scanMetaCSPPolicies(body, collect)
	if len(constraining) == 0 {
		return fallback
	}
	return intersectNonceSets(constraining)
}

// scanMetaCSPPolicies walks an HTML body and invokes collect for every
// Content-Security-Policy meta tag (attributes decoded, any order, exact
// http-equiv key; every meta examined so a nonce-free policy cannot hide a
// later nonce-bearing one).
func scanMetaCSPPolicies(body []byte, collect func(string)) {
	tok := html.NewTokenizer(bytes.NewReader(body))
	for {
		tt := tok.Next()
		if tt == html.ErrorToken {
			return
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := tok.TagName()
		if !bytes.Equal(name, []byte("meta")) || !hasAttr {
			continue
		}
		var equiv, content string
		for {
			key, val, more := tok.TagAttr()
			switch {
			case bytes.EqualFold(key, []byte("http-equiv")):
				equiv = string(val)
			case bytes.EqualFold(key, []byte("content")):
				content = string(val)
			}
			if !more {
				break
			}
		}
		if strings.EqualFold(strings.TrimSpace(equiv), "content-security-policy") {
			collect(content)
		}
	}
}

// intersectNonceSets returns the first candidate present in every
// constraining set, or "" when the intersection is empty.
func intersectNonceSets(constraining [][]string) string {
	candidates := constraining[0]
	for _, set := range constraining[1:] {
		kept := candidates[:0]
		for _, c := range candidates {
			for _, s := range set {
				if c == s {
					kept = append(kept, c)
					break
				}
			}
		}
		candidates = kept
		if len(candidates) == 0 {
			return ""
		}
	}
	return candidates[0]
}

// shimCapabilityJS returns the JavaScript spliced into the shim's path
// rewriter so runtime-rewritten URLs carry the subtree capability. It appends
// the capability as a query parameter: any URL fragment is split off first so
// the query lands before it and existing query strings are preserved. The
// append is idempotent for the EXACT issued capability: a URL whose query has
// a kandev_cap parameter whose DECODED key matches and whose value equals the
// issued capability is not touched (this stops the MutationObserver from
// looping on its own rewrites), while an app's own different kandev_cap value
// still gets the issued capability appended — the gateway accepts any valid
// value among duplicates. A substring like `/foo/kandev_cap=x` in the path or
// `?note=kandev_cap=x` in another parameter does NOT suppress the append.
// Empty when no capability is minted, so the auth-disabled output carries no
// capability bytes (see TestRuntimeShim_NoCapabilityBytes).
func shimCapabilityJS(capability string) string {
	if capability == "" {
		return ""
	}
	param := proxyCapabilityQueryParam + "="
	return fmt.Sprintf(`var K=%q;`+
		`function hcp(u){var qi=u.indexOf('?');if(qi===-1)return false;var ps=u.slice(qi+1).split('&');for(var i=0;i<ps.length;i++){var p=ps[i];var e=p.indexOf('=');var k=e===-1?p:p.slice(0,e);var v=e===-1?'':p.slice(e+1);var d;try{d=decodeURIComponent(k)}catch(x){d=k}var vd;try{vd=decodeURIComponent(v)}catch(x){vd=v}if(d==='kandev_cap'&&vd===K)return true}return false}`+
		`var fr='';var fh=u.indexOf('#');if(fh!==-1){fr=u.slice(fh);u=u.slice(0,fh)}if(!hcp(u))u+=(u.indexOf('?')===-1?'?':'&')+%q+K;return u+fr;`,
		capability, param)
}

// rewriteProxyResponse mutates an `http.Response` from agentctl in place,
// rewriting root-absolute URLs to be prefixed by `proxyPrefix` so the iframe's
// asset/XHR/import requests come back through the same port proxy instead of
// hitting the host page's origin. Returns nil and leaves the response untouched
// for content types we don't rewrite (everything except HTML and CSS today).
//
// `proxyPrefix` is the public URL path that fronts this proxy on the gateway,
// e.g. "/port-proxy/<sessionId>/<port>" (no trailing slash). It is prepended to
// matched URLs that start with a single "/" — see `rewriteAbsolutePath`.
//
// `capability`, when non-empty, is appended to every rewritten URL as
// `kandev_cap=<capability>`. It is the short-lived subtree credential minted
// after the preview document authenticated; embedding it in the asset URLs
// keeps the browser's subresource fetches authorized even when they cannot
// carry cookies (the <link rel="manifest"> fetch, sandboxed iframes).
func rewriteProxyResponse(resp *http.Response, proxyPrefix, capability string) error {
	// 1xx/204/205/206/304 responses are left untouched: 1xx/204/205/304 carry
	// no body (304's Content-Length describes the selected representation),
	// and 206's body is a byte RANGE whose Content-Range describes it —
	// rewriting would desynchronize the length/range metadata.
	if resp.StatusCode < 200 || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent || resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	// If the upstream response is still content-encoded here, Go's Transport
	// did not (or could not) decode it (e.g. a misbehaving upstream ignoring
	// Accept-Encoding and sending brotli/deflate/zstd): rewriting the
	// compressed bytes and deleting Content-Encoding would corrupt the
	// document. Leave such responses untouched.
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		return nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	// The public directory the response was served from (stashed at the
	// handler before the request path was rewritten to the agentctl form):
	// HTML and CSS resolve their relative references against it, so a
	// document below the proxy root (e.g. .../pages/index.html with
	// ../assets) stays in-subtree and keeps its capability.
	basePath := proxyPrefix + "/"
	if resp.Request != nil {
		if v, ok := resp.Request.Context().Value(proxyCSSBaseContextKey{}).(string); ok && v != "" {
			basePath = v
		}
	}
	var rewrite func([]byte, string, string, string) []byte
	switch {
	case strings.Contains(ct, "text/html"):
		base := basePath
		rewrite = func(body []byte, prefix, capability, nonce string) []byte {
			return rewriteHTMLURLsAtDepthWithBase(body, prefix, capability, nonce, 0, base)
		}
	case strings.Contains(ct, "text/css"):
		base := basePath
		rewrite = func(body []byte, prefix, capability, _ string) []byte {
			return rewriteCSSURLsAt(body, prefix, capability, base)
		}
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

	nonce := responseScriptNonce(resp, body)
	modified := rewrite(body, proxyPrefix, capability, nonce)
	resp.Body = io.NopCloser(bytes.NewReader(modified))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
	resp.ContentLength = int64(len(modified))
	return nil
}

// withCapability appends the capability query parameter to a rewritten URL,
// using "&" when the URL already carries a query string. A URL fragment, when
// present, stays last — the capability must precede it or the browser would
// treat it as part of the fragment and never send it.
func withCapability(rewritten, capability string) string {
	if capability == "" {
		return rewritten
	}
	path, fragment, hasFragment := strings.Cut(rewritten, "#")
	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	path += sep + proxyCapabilityQueryParam + "=" + capability
	if hasFragment {
		path += "#" + fragment
	}
	return path
}

// rewriteAbsolutePath turns a path-absolute URL ("/foo") into a proxied path
// ("<prefix>/foo"). Returns the input unchanged for non-rewritable cases:
// empty strings, network-relative URLs (`//host`), schemes (`http:`, `data:`,
// `mailto:`, etc.), or relative paths (`foo`, `./foo`, `../foo`).
// Dot segments in the path are normalized: a root path whose normalized form
// escapes the subtree (/../x) is returned unchanged rather than emitting a
// capability on a non-proxy path.
func rewriteAbsolutePath(rawURL, prefix, capability string) string {
	if len(rawURL) < 1 || rawURL[0] != '/' {
		return rawURL
	}
	if len(rawURL) >= 2 && rawURL[1] == '/' {
		return rawURL // network-relative
	}
	if strings.Contains(rawURL, "/.") {
		resolved := removeDotSegments(prefix + rawURL)
		if !inSubtreePath(resolved, prefix) {
			return rawURL
		}
		return withCapability(resolved, capability)
	}
	return withCapability(prefix+rawURL, capability)
}

// htmlRewriteState tracks per-document position during a single
// `rewriteHTMLURLs` pass. Pulled out to its own type so each token-handling
// case stays short and the top-level loop's cyclomatic complexity stays under
// the lint budget.
type htmlRewriteState struct {
	out          *bytes.Buffer
	prefix       string
	capability   string
	shim         string
	nonce        string
	depth        int
	inStyle      bool
	inScript     bool
	shimInjected bool
	// basePath is the effective document base DIRECTORY (root-absolute,
	// resolved against the subtree). "" means the effective base resolves
	// OUTSIDE the proxy subtree (scheme-bearing, network-relative, or
	// dot-segment-escaping base): every path-absolute and relative reference
	// in the document resolves against an external origin, so embedding the
	// capability in them would leak the bearer; rewriting is suppressed.
	basePath string
	baseSet  bool
}

// applyBase records the document's effective <base href> (the FIRST base
// element wins per the HTML spec). The href is left for the normal token
// rewrite pass (navigation: prefix-only, no capability); only the RESOLVED
// directory of the nav-rewritten value is captured here. A scheme-bearing,
// network-relative, or dot-segment-escaping base makes every later
// path/relative reference resolve outside the subtree (basePath = "").
func (s *htmlRewriteState) applyBase(token *html.Token) {
	if s.baseSet || token.Data != "base" {
		return
	}
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "href") {
			s.baseSet = true
			rewritten := rewriteURLReference(attr.Val, s.prefix, "")
			norm := normalizeURLForClassification(rewritten)
			if hasURLScheme(norm) || strings.HasPrefix(norm, "//") {
				s.basePath = ""
				return
			}
			if norm == "" || norm[0] == '#' {
				return
			}
			resolved := resolvePathReference(norm, s.basePath)
			if !inSubtreePath(resolved, s.prefix) {
				s.basePath = ""
				return
			}
			s.basePath = resolved
			return
		}
	}
}

// onStartTag emits the (URL-rewritten) start tag and updates raw-text
// element tracking. Also injects the runtime shim immediately after `<head>`.
func (s *htmlRewriteState) onStartTag(token html.Token) {
	s.applyBase(&token)
	rewriteTokenURLs(&token, s.prefix, s.capability, s.nonce, s.depth, s.basePath)
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
		if s.basePath == "" {
			// Under an external <base>, url() references inside the style
			// body resolve against the external origin: rewriting them would
			// leak the capability (same policy as inline style attributes).
			s.out.WriteString(token.Data)
			return
		}
		s.out.WriteString(rewriteCSSFragment(token.Data, s.prefix, s.capability, s.basePath))
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
func rewriteHTMLURLs(body []byte, prefix, capability, nonce string) []byte {
	return rewriteHTMLURLsAtDepth(body, prefix, capability, nonce, 0)
}

// maxSRCDocDepth bounds recursive srcdoc rewriting so a deeply nested inline
// document cannot exhaust the stack or balloon memory.
const maxSRCDocDepth = 5

func rewriteHTMLURLsAtDepth(body []byte, prefix, capability, nonce string, depth int) []byte {
	return rewriteHTMLURLsAtDepthWithBase(body, prefix, capability, nonce, depth, prefix+"/")
}

func rewriteHTMLURLsAtDepthWithBase(body []byte, prefix, capability, nonce string, depth int, basePath string) []byte {
	tok := html.NewTokenizer(bytes.NewReader(body))
	var out bytes.Buffer
	out.Grow(len(body) + 256 + len(runtimeShimTemplate))
	s := &htmlRewriteState{out: &out, prefix: prefix, capability: capability, shim: runtimeShimTag(prefix, capability, nonce), nonce: nonce, depth: depth, basePath: basePath}
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
			s.applyBase(&token)
			rewriteTokenURLs(&token, prefix, capability, nonce, depth, s.basePath)
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
// shaped attribute value in place. Navigation attributes — anchor/area/base
// href, form action, button/input formaction — get the proxy prefix but NOT
// the capability: clicking them navigates the top-level frame, so a bearer in
// the URL would land in the address bar, history, bookmarks, and cross-origin
// Referers (the subtree cookie authorizes the navigation instead). Subresource
// attributes (script/img/stylesheet/manifest/iframe src-href, srcset,
// imagesrcset, …) get the capability so cookie-less fetches stay authorized.
func rewriteTokenURLs(token *html.Token, prefix, capability, nonce string, depth int, basePath string) {
	if token.Type != html.StartTagToken && token.Type != html.SelfClosingTagToken {
		return
	}
	// Metadata links (rel=canonical, alternate, license, author, …) are not
	// fetched; a capability in their href would leak through the emitted
	// canonical URL. rel values that DO fetch (stylesheet, icon, manifest,
	// preload, modulepreload, …) keep the capability.
	metaLink := token.Data == "link" && !isFetchingLinkRel(relValue(token))
	for i, attr := range token.Attr {
		key := strings.ToLower(attr.Key)
		token.Attr[i].Val = rewriteTokenAttr(token, key, attr.Val, prefix, capability, nonce, depth, metaLink, basePath)
	}
}

// rewriteTokenAttr rewrites one URL-shaped attribute value of a token,
// applying the navigation/subresource capability distinction.
func rewriteTokenAttr(token *html.Token, key, val, prefix, capability, nonce string, depth int, metaLink bool, basePath string) string {
	// Inline srcdoc documents are dispatched FIRST: the child document can
	// declare its own <base>, which resets the inherited external-base
	// policy, and a srcdoc value (starting with '<') would otherwise be
	// classified as a relative reference and suppressed unchanged.
	if key == "srcdoc" {
		return rewriteSrcdocValue(val, prefix, capability, nonce, depth, basePath)
	}
	// Under an external <base href>, every path-absolute and relative
	// reference in the document resolves OUTSIDE the proxy subtree: adding
	// the proxy prefix or the capability would either change the app's
	// intended resolution or leak the bearer to the external origin, so such
	// values are left exactly as authored. Scheme-bearing and network-relative
	// references were never rewritten anyway; only the inline style content
	// needs an explicit guard (its leading text is not a URL).
	if basePath == "" && suppressedByExternalBase(val) {
		// External base: leave every path/relative reference as authored.
		return val
	}
	return rewriteTokenAttrKey(token, key, val, prefix, capability, nonce, depth, metaLink, basePath)
}

// rewriteTokenAttrKey dispatches one attribute value to its rewriter.
func rewriteTokenAttrKey(token *html.Token, key, val, prefix, capability, nonce string, depth int, metaLink bool, basePath string) string {
	switch {
	case isNavigationAttr(token, key, metaLink):
		// Navigation references — anchor/area/base href, form action,
		// button/input formaction — and cite (copyable metadata, never
		// fetched): prefix-only, no capability.
		return rewriteURLReference(val, prefix, "")
	case key == attrContent && token.Data == "meta" && isMetaRefresh(token):
		return rewriteMetaRefresh(val, prefix)
	case rewritableURLAttrs[key]:
		return rewriteURLReferenceBase(val, prefix, capability, basePath)
	case key == "srcset" || key == "imagesrcset":
		return rewriteSrcSet(val, prefix, capability, basePath)
	case key == "ping":
		// a[ping] is a whitespace-separated URL list the browser POSTs to on
		// activation (hyperlink auditing): each candidate is a capability-
		// bearing subresource request.
		return rewritePing(val, prefix, capability, basePath)
	case key == "style":
		return rewriteStyleValue(val, prefix, capability, basePath)
	}
	return val
}

// rewritePing rewrites every whitespace-separated URL in an a[ping] list.
func rewritePing(val, prefix, capability, basePath string) string {
	fields := strings.Fields(val)
	for i, f := range fields {
		fields[i] = rewriteURLReferenceBase(f, prefix, capability, basePath)
	}
	return strings.Join(fields, " ")
}

// rewriteStyleValue rewrites inline style url() references, unless the
// document has an external base (the url() references would resolve against
// it and leak the capability).
func rewriteStyleValue(val, prefix, capability, basePath string) string {
	if basePath == "" {
		return val
	}
	return rewriteCSSFragment(val, prefix, capability, basePath)
}

// suppressedByExternalBase reports whether a reference resolves against an
// external <base> (i.e. it is path-absolute or relative, not scheme-bearing,
// network-relative, or fragment-only) and must be left exactly as authored.
func suppressedByExternalBase(val string) bool {
	norm := normalizeURLForClassification(val)
	return norm != "" && norm[0] != '#' && !hasURLScheme(norm) && !strings.HasPrefix(norm, "//")
}

// rewriteSrcdocValue rewrites an inline child document under the same base
// policy as its parent, bounded by maxSRCDocDepth.
func rewriteSrcdocValue(val, prefix, capability, nonce string, depth int, basePath string) string {
	if depth < maxSRCDocDepth {
		return string(rewriteHTMLURLsAtDepthWithBase([]byte(val), prefix, capability, nonce, depth+1, basePath))
	}
	return val
}

// isNavigationAttr reports whether an attribute is a navigation reference
// (anchor/area/base href, form action, button/input formaction) or cite
// (copyable metadata the browser never fetches): prefix-only, no capability.
func isNavigationAttr(token *html.Token, key string, metaLink bool) bool {
	return key == "cite" || key == "href" && (isNavHref(token.Data) || metaLink) ||
		key == "action" && token.Data == "form" ||
		key == "formaction" && (token.Data == "button" || token.Data == "input")
}

// isNavHref reports whether an element's href is a navigation reference.
func isNavHref(tag string) bool {
	return tag == "a" || tag == "area" || tag == "base"
}

// relValue extracts the normalized rel attribute value of a token.
func relValue(token *html.Token) string {
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "rel") {
			return strings.ToLower(strings.TrimSpace(attr.Val))
		}
	}
	return ""
}

// attrContent is the HTML "content" attribute key (meta refresh targets).
const attrContent = "content"

// fetchingLinkRels are link rel tokens whose href the browser fetches; a rel
// value with ANY fetching token is a subresource. Metadata-only rels (no
// fetching token) must not carry the capability.
var fetchingLinkRels = map[string]bool{
	"stylesheet": true, "icon": true, "shortcut": true, "apple-touch-icon": true,
	"manifest": true, "preload": true, "modulepreload": true, "prefetch": true,
	"dns-prefetch": true, "preconnect": true,
}

// isFetchingLinkRel reports whether a link rel value (a whitespace-separated
// token list, per the HTML spec) contains any fetching token.
func isFetchingLinkRel(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if fetchingLinkRels[strings.ToLower(token)] {
			return true
		}
	}
	return false
}

// isMetaRefresh reports whether a meta token is an http-equiv=refresh.
func isMetaRefresh(token *html.Token) bool {
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "http-equiv") && strings.EqualFold(strings.TrimSpace(attr.Val), "refresh") {
			return true
		}
	}
	return false
}

// rewriteMetaRefresh prefixes the root-absolute navigation target inside a
// meta refresh content value (no capability — it is a navigation). The "url"
// token is recognized only at a field boundary — the start of the value or
// right after any ";" separator, ASCII case-insensitive, ignoring surrounding
// whitespace — so unrelated text like `noturl=/evil` is never rewritten and a
// multi-field value like `5;foo=bar; url=/next` is handled. Matching scans
// the ORIGINAL bytes with a length-preserving ASCII fold: strings.ToLower can
// change UTF-8 byte length for some Unicode characters (U+212A KELVIN folds
// 3 bytes to 1), which would corrupt byte offsets into the value. The target
// is parsed exactly once, respecting an optional quote; any text after it is
// preserved untouched.
func rewriteMetaRefresh(content, prefix string) string {
	idx := -1
	from := 0
	for from <= len(content) {
		semi := strings.IndexByte(content[from:], ';')
		end := len(content)
		if semi >= 0 {
			end = from + semi
		}
		field := content[from:end]
		k := 0
		for k < len(field) && isCSSSpace(field[k]) {
			k++
		}
		if asciiFoldHasPrefix(field[k:], "url=") {
			idx = from + k
			break
		}
		if semi < 0 {
			break
		}
		from = end + 1
	}
	if idx < 0 {
		return content
	}
	rest := content[idx+4:]
	trimmed := strings.TrimLeft(rest, " \t\n\r\f")
	offset := len(rest) - len(trimmed)
	target, tail, quote := parseRefreshTarget(trimmed)
	return content[:idx+4] + rest[:offset] + quote + rewriteURLReference(target, prefix, "") + tail
}

// asciiFoldHasPrefixAt reports whether value[i:] starts with prefix under
// ASCII case folding, comparing at most len(prefix) bytes (bounded: it never
// scans or allocates the rest of the string).
func asciiFoldHasPrefixAt(value string, i int, prefix string) bool {
	if i+len(prefix) > len(value) {
		return false
	}
	return asciiFoldHasPrefix(value[i:i+len(prefix)], prefix)
}

// asciiFoldHasPrefix reports whether s starts with prefix under ASCII
// case folding (A-Z only). Length-preserving: unlike strings.ToLower it never
// changes byte lengths, so offsets into s remain valid.
func asciiFoldHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a, b := s[i], prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

// parseRefreshTarget splits a meta refresh target off the text after `url=`,
// respecting an optional quote. The tail keeps the closing quote and any
// remaining text (including nested url= tokens) untouched.
func parseRefreshTarget(trimmed string) (target, tail, quote string) {
	if trimmed == "" {
		return "", "", ""
	}
	if trimmed[0] != '\'' && trimmed[0] != '"' {
		// All five ASCII whitespace bytes terminate an unquoted target,
		// matching the runtime mref() and CSS whitespace semantics.
		if end := strings.IndexAny(trimmed, "; \t\n\r\f"); end >= 0 {
			return trimmed[:end], trimmed[end:], ""
		}
		return trimmed, "", ""
	}
	quote = trimmed[:1]
	if end := strings.IndexByte(trimmed[1:], quote[0]); end >= 0 {
		return trimmed[1 : 1+end], trimmed[1+end:], quote
	}
	return trimmed[1:], "", quote
}

// rewriteURLReference rewrites a URL-shaped attribute value. Path-absolute
// URLs get the proxy prefix plus the capability. Same-origin relative URLs
// keep their own resolution — the document already lives in the proxy subtree,
// so the browser lands inside it — but still get the capability appended so
// cookie-less fetches (a relative <link rel="manifest">) stay authorized.
// Classification runs against a WHATWG-normalized copy (ASCII tabs/newlines
// removed anywhere, leading C0/space trimmed — the browser does the same when
// parsing): a scheme-bearing or network-relative reference hidden behind
// whitespace or embedded controls is left untouched rather than leaking the
// capability to an external origin. Empty, fragment-only, and scheme-bearing
// values are never modified.
func rewriteURLReference(rawURL, prefix, capability string) string {
	// The default base is the proxied document directory; callers that know
	// the effective <base> pass it via rewriteURLReferenceBase.
	return rewriteURLReferenceBase(rawURL, prefix, capability, prefix+"/")
}

// rewriteURLReferenceBase rewrites a URL-shaped reference against an
// explicit document base directory. Relative capability-bearing references
// are resolved against basePath (RFC 3986 merge + dot-segment removal): a
// reference whose resolved path escapes the proxy subtree (e.g. ../x) is
// returned unchanged rather than carrying the capability to a non-proxy
// endpoint.
func rewriteURLReferenceBase(rawURL, prefix, capability, basePath string) string {
	normalized := normalizeURLForClassification(rawURL)
	if capability == "" {
		// Navigation/metadata rewriting never carries the capability: strip
		// any previously issued one (a link reclassified from fetching to
		// metadata, or an app URL carrying a stale cap) so the bearer cannot
		// land in the address bar, history, or a cross-origin Referer. The
		// gateway also strips reserved pairs on forward; this keeps them out
		// of the DOM in the first place. Scheme-bearing references are
		// preserved verbatim EXCEPT http:/https: absolute references, which
		// ARE gateway HTTP queries and must have a stale capability removed
		// (the runtime rn() does the same for same-origin absolutes);
		// data:, mailto:, and other scheme payloads keep their '?'/'#' as
		// payload, and network-relative references are left untouched.
		strip := normalized != "" && normalized[0] != '#' && !strings.HasPrefix(normalized, "//")
		if strip && hasURLScheme(normalized) {
			strip = httpScheme(normalized)
		}
		if strip {
			rawURL = stripCapability(rawURL)
			normalized = normalizeURLForClassification(rawURL)
		}
	}
	if normalized == "" || normalized[0] == '#' || hasURLScheme(normalized) || strings.HasPrefix(normalized, "//") {
		return rawURL
	}
	if normalized[0] == '/' {
		return rewriteAbsolutePath(normalized, prefix, capability)
	}
	if capability == "" {
		return rawURL
	}
	// Relative capability-bearing emission uses the WHATWG-normalized form
	// (controls removed, backslashes slashed, leading/trailing C0/space
	// trimmed) exactly like the runtime nz(): a trailing space in the raw
	// value would otherwise survive into `x ?kandev_cap=…` and be
	// percent-encoded into the pathname by the browser. Only append when the
	// reference resolves inside the proxy subtree (.. segments can escape).
	if !resolveInSubtree(normalized, basePath, prefix) {
		return rawURL
	}
	return withCapability(normalized, capability)
}

// resolveInSubtree reports whether a relative reference resolves to a path
// boundary-inside the proxy prefix when merged with the document base. Only
// the PATH portion is resolved: a trailing ?query/#fragment attached to a ..
// segment (../?q) must not hide the escape.
func resolveInSubtree(ref, basePath, prefix string) bool {
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	return inSubtreePath(resolvePathReference(ref, basePath), prefix)
}

// inSubtreePath reports whether a root-absolute path is boundary-inside the
// proxy prefix (path == prefix or path starts with prefix + "/").
func inSubtreePath(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// resolvePathReference merges a relative reference with a base path and
// removes dot segments per RFC 3986 section 5.3/5.2.4.
func resolvePathReference(ref, basePath string) string {
	if strings.HasPrefix(ref, "/") {
		return removeDotSegments(ref)
	}
	base := basePath
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return removeDotSegments(base + ref)
}

// removeDotSegments removes "." and ".." path segments without escaping
// above the root (RFC 3986 5.2.4). A trailing slash is preserved when the
// input is directory-shaped (ends in "/", "/.", or "/.."), so the result
// stays usable as a base directory.
func removeDotSegments(p string) string {
	trailing := strings.HasSuffix(p, "/") || strings.HasSuffix(p, "/.") || strings.HasSuffix(p, "/..")
	segments := strings.Split(p, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		switch seg {
		case ".":
			// skip
		case "..":
			if len(out) > 1 || len(out) == 1 && out[0] != "" {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, seg)
		}
	}
	joined := strings.Join(out, "/")
	if trailing && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

// stripCapability removes every query parameter whose DECODED key equals the
// reserved proxy capability key (so percent-encoded key spellings like
// %6bandev_cap are stripped too), preserving all other parameters. The
// fragment is split off FIRST: a '?' inside a fragment (#state?x=1) is
// fragment payload, not a query, and a '#' inside a data URL is payload too.
// A URL with no query string is returned unchanged.
func stripCapability(raw string) string {
	path, fragment, hasFragment := strings.Cut(raw, "#")
	if hasFragment {
		path = stripCapabilityQuery(path)
		return path + "#" + fragment
	}
	return stripCapabilityQuery(raw)
}

func stripCapabilityQuery(raw string) string {
	path, query, hasQuery := strings.Cut(raw, "?")
	if !hasQuery {
		return raw
	}
	parts := strings.Split(query, "&")
	kept := parts[:0]
	for _, p := range parts {
		key := p
		if e := strings.IndexByte(p, '='); e >= 0 {
			key = p[:e]
		}
		decoded, err := url.QueryUnescape(key)
		if err != nil {
			decoded = key
		}
		if decoded == proxyCapabilityQueryParam {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		// The original explicitly supplied a query; keep the marker so the
		// reference still resolves as an empty query rather than inheriting
		// the current document's query.
		return path + "?"
	}
	return path + "?" + strings.Join(kept, "&")
}

// normalizeURLForClassification returns a copy of a URL reference with the
// WHATWG parser's forgiving adjustments applied: ASCII tabs and newlines are
// removed anywhere in the string, BACKSLASHES are converted to slashes (the
// browser treats '\' as '/' in special-URL paths, so '\foo' resolves to
// /foo while '\/evil' becomes the network-relative //evil), and leading AND
// trailing C0/space characters are trimmed (the parser strips both for
// special URLs). Only the result is used for classification/emission; the
// original bytes are preserved when the reference is returned unchanged.
func normalizeURLForClassification(raw string) string {
	noControls := strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\r':
			return -1
		case '\\':
			return '/'
		}
		return r
	}, raw)
	return strings.TrimFunc(noControls, func(r rune) bool { return r <= ' ' })
}

// hasURLScheme reports whether a URL reference starts with a scheme per
// RFC 3986 — ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) followed by ":"
// (http:, data:, javascript:, mailto:, …). A colon in a position where a
// scheme could not start (a digit or a non-scheme character first, or after a
// path/query/fragment delimiter) makes the reference relative instead.
func hasURLScheme(raw string) bool {
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			continue
		case c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.':
			if i == 0 {
				return false // schemes start with an ASCII letter
			}
			continue
		case c == ':':
			return i > 0
		default:
			return false
		}
	}
	return false
}

// httpScheme reports whether a scheme-bearing reference uses the http: or
// https: scheme (the only schemes whose '?' is a gateway HTTP query).
func httpScheme(raw string) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] == ':' {
			return strings.EqualFold(raw[:i], "http") || strings.EqualFold(raw[:i], "https")
		}
		if !isSchemeChar(raw[i]) {
			return false
		}
	}
	return false
}

// isSchemeChar reports whether c is valid inside a URL scheme (per RFC 3986).
func isSchemeChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// rewriteSrcSet rewrites each candidate URL in a `srcset` attribute. The
// value format is `url [descriptor], url [descriptor], …` per the HTML spec.
func rewriteSrcSet(value, prefix, capability, basePath string) string {
	parts := splitSrcSetParts(value)
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
		fields[0] = rewriteURLReferenceBase(fields[0], prefix, capability, basePath)
		parts[i] = strings.Join(fields, " ")
	}
	return strings.Join(parts, ", ")
}

// splitSrcSetParts splits a srcset value into its candidate strings. The
// spec separates candidates on commas, but a comma INSIDE a data: URL (e.g.
// data:image/svg+xml,<svg…>) belongs to the URL, not to the candidate list:
// a candidate that started with data: and has not yet hit whitespace is
// still inside its URL, so its commas are preserved. Descriptor text (after
// the first whitespace) is ordinary candidate space where commas separate.
// The scan is single-pass: whitespace-only and data:-prefix state is tracked
// incrementally, never by rescanning the accumulated candidate.
func splitSrcSetParts(value string) []string {
	var parts []string
	var cur strings.Builder
	inData := false
	curEmpty := true // cur contains only whitespace so far
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == ',' {
			if inData {
				cur.WriteByte(c)
				continue
			}
			parts = append(parts, cur.String())
			cur.Reset()
			curEmpty = true
			inData = false
			continue
		}
		// The data: prefix is tested only at the FIRST non-whitespace byte of
		// a candidate (curEmpty is true then), with a bounded five-byte
		// ASCII-fold compare — never a whole-suffix scan or allocation, so
		// N candidates cost O(N), not O(N^2).
		if !inData && curEmpty && !isCSSSpace(c) && asciiFoldHasPrefixAt(value, i, "data:") {
			inData = true
		}
		if inData && isCSSSpace(c) {
			inData = false
		}
		if !isCSSSpace(c) {
			curEmpty = false
		}
		cur.WriteByte(c)
	}
	if !curEmpty {
		parts = append(parts, cur.String())
	}
	return parts
}
