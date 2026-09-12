package webapp

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

var ErrRuntimeBootstrapUnavailable = errors.New("webapp: runtime startup bootstrap unavailable")

const hostRuntimeBootstrap = `(() => {
  "use strict";
  const VERSION = 1;
  const PROBE_TYPE = "kandev.web_app.startup_probe";
  const RESULT_TYPE = "kandev.web_app.startup_result";
  const MAX_NONCE_BYTES = 128;
  let outcome = null;
  let pendingNonce = null;
  let outcomeSent = false;

  const sendOutcome = () => {
    if (outcomeSent || !pendingNonce || !outcome || window.parent === window) return;
    outcomeSent = true;
    const message = {
      type: RESULT_TYPE,
      version: VERSION,
      nonce: pendingNonce,
      result: outcome.result,
    };
    if (outcome.code) message.code = outcome.code;
    window.parent.postMessage(message, "*"); // Sandboxed iframes have a null origin, so "*" is the only viable target.
  };

  const finish = (result, code) => {
    if (outcome) return;
    outcome = code ? { result, code } : { result };
    sendOutcome();
  };

  const failDocument = () => finish("failed", "document_error");
  window.addEventListener("error", (event) => {
    if (event.target !== window || event instanceof ErrorEvent) failDocument();
  }, true);
  window.addEventListener("unhandledrejection", failDocument, true);

  window.addEventListener("message", (event) => {
    if (event.source !== window.parent || outcomeSent) return;
    const data = event.data;
    if (!data || typeof data !== "object" || Array.isArray(data)) return;
    if (data.type !== PROBE_TYPE || data.version !== VERSION) return;
    if (typeof data.nonce !== "string" || data.nonce.length === 0 || data.nonce.length > MAX_NONCE_BYTES) return;
    if (pendingNonce) return;
    pendingNonce = data.nonce;
    sendOutcome();
  });

  const checkContext = () => {
    if (outcome) return;
    fetch("./_kandev/v1/context", { credentials: "omit", cache: "no-store" })
      .then((response) => {
        if (!response.ok) throw new Error("context unavailable");
        return response.json();
      })
      .then(() => finish("ready"))
      .catch(() => finish("failed", "context_unavailable"));
  };

  if (document.readyState === "complete") checkContext();
  else window.addEventListener("load", checkContext, { once: true });
})();
`

const hostRuntimeBootstrapTag = `<script src="./_kandev/host-runtime.js"></script>`

type runtimeBootstrapNamespace uint8

const (
	runtimeBootstrapHTMLNamespace runtimeBootstrapNamespace = iota
	runtimeBootstrapSVGNamespace
	runtimeBootstrapMathMLNamespace
	runtimeBootstrapTemplateTag = "template"
)

var runtimeBootstrapForeignBreakoutTags = map[string]struct{}{
	"b": {}, "big": {}, "blockquote": {}, "body": {}, "br": {}, "center": {}, "code": {}, "dd": {}, "div": {}, "dl": {}, "dt": {}, "em": {}, "embed": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {}, "head": {}, "hr": {}, "i": {}, "img": {}, "li": {}, "listing": {}, "menu": {}, "meta": {},
	"nobr": {}, "ol": {}, "p": {}, "pre": {}, "ruby": {}, "s": {}, "small": {}, "span": {}, "strong": {}, "strike": {}, "sub": {}, "sup": {}, "table": {}, "tt": {},
	"u": {}, "ul": {}, "var": {},
}

type runtimeBootstrapOpenElement struct {
	name           string
	namespace      runtimeBootstrapNamespace
	childNamespace runtimeBootstrapNamespace
}

type runtimeBootstrapParserState struct {
	elements      []runtimeBootstrapOpenElement
	templateDepth int
}

func injectRuntimeBootstrap(entry []byte) ([]byte, error) {
	if int64(len(entry)) > MaxFileBytes {
		return nil, ErrRuntimeBootstrapUnavailable
	}
	if len(entry) == 0 {
		return nil, ErrRuntimeBootstrapUnavailable
	}
	insertion, err := runtimeBootstrapInsertion(entry)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(entry)+len(hostRuntimeBootstrapTag))
	result = append(result, entry[:insertion]...)
	result = append(result, hostRuntimeBootstrapTag...)
	result = append(result, entry[insertion:]...)
	return result, nil
}

func runtimeBootstrapInsertion(entry []byte) (int, error) {
	tokenizer := html.NewTokenizer(bytes.NewReader(entry))
	tokenizer.SetMaxBuf(len(entry) + 1)
	position := 0
	insertion := -1
	fallback := -1
	state := runtimeBootstrapParserState{}
	for {
		tokenType := tokenizer.Next()
		raw := tokenizer.Raw()
		if tokenType == html.ErrorToken {
			if errors.Is(tokenizer.Err(), io.EOF) {
				break
			}
			return -1, ErrRuntimeBootstrapUnavailable
		}
		if len(raw) == 0 {
			return -1, ErrRuntimeBootstrapUnavailable
		}
		insertion, fallback = updateRuntimeBootstrapPosition(tokenizer, tokenType, raw, position, insertion, fallback, &state)
		position += len(raw)
	}
	if position != len(entry) {
		return -1, ErrRuntimeBootstrapUnavailable
	}
	if insertion < 0 {
		insertion = fallback
	}
	if insertion < 0 {
		if state.templateDepth > 0 {
			return -1, ErrRuntimeBootstrapUnavailable
		}
		// HTML supplies implied head and body elements when an entry omits the
		// corresponding wrapper tags. Appending here keeps the original doctype
		// and encoding declarations intact while placing the script in the
		// implied body, outside any inert template content.
		insertion = len(entry)
	}
	return insertion, nil
}

func updateRuntimeBootstrapPosition(tokenizer *html.Tokenizer, tokenType html.TokenType, raw []byte, position, insertion, fallback int, state *runtimeBootstrapParserState) (int, int) {
	token, hasTag := runtimeBootstrapToken(tokenizer, tokenType)
	if !hasTag {
		return insertion, fallback
	}
	tagName := strings.ToLower(token.Data)
	if tokenType != html.EndTagToken && state.shouldBreakOutOfForeignContent(tagName, token.Attr) {
		state.popForeignContent()
	}
	namespace := state.namespaceForTag(tagName)
	if tokenType == html.EndTagToken {
		return updateRuntimeBootstrapEndTag(tagName, namespace, position, insertion, fallback, state)
	}
	return updateRuntimeBootstrapStartTagPosition(tokenizer, token, tokenType, raw, tagName, namespace, position, insertion, fallback, state)
}

func updateRuntimeBootstrapEndTag(tagName string, namespace runtimeBootstrapNamespace, position, insertion, fallback int, state *runtimeBootstrapParserState) (int, int) {
	state.closeElement(tagName, namespace)
	if namespace == runtimeBootstrapHTMLNamespace && state.templateDepth == 0 && fallback < 0 && isRuntimeBootstrapFallbackTag(tagName) {
		fallback = position
	}
	return insertion, fallback
}

func updateRuntimeBootstrapStartTagPosition(tokenizer *html.Tokenizer, token html.Token, tokenType html.TokenType, raw []byte, tagName string, namespace runtimeBootstrapNamespace, position, insertion, fallback int, state *runtimeBootstrapParserState) (int, int) {
	elementNamespace, childNamespace := state.elementNamespaces(tagName, token.Attr)
	if elementNamespace != runtimeBootstrapHTMLNamespace {
		// The tokenizer otherwise treats some foreign elements, such as SVG
		// title, as HTML raw-text elements.
		tokenizer.NextIsNotRawText()
	}
	if tagName == "script" && tokenType == html.StartTagToken && state.templateDepth == 0 && insertion < 0 {
		insertion = position
	}
	if elementNamespace == runtimeBootstrapHTMLNamespace && tagName == runtimeBootstrapTemplateTag {
		state.templateDepth++
	}
	if tokenType == html.StartTagToken && namespace == runtimeBootstrapHTMLNamespace && state.templateDepth == 0 {
		insertion, fallback = updateRuntimeBootstrapStartTag(tagName, raw, position, insertion, fallback)
	}
	state.openElement(tagName, elementNamespace, childNamespace, tokenType)
	return insertion, fallback
}

func (state *runtimeBootstrapParserState) namespaceForTag(tagName string) runtimeBootstrapNamespace {
	if len(state.elements) == 0 {
		return runtimeBootstrapHTMLNamespace
	}
	top := state.elements[len(state.elements)-1]
	if top.namespace == runtimeBootstrapMathMLNamespace && isRuntimeBootstrapMathMLTextIntegrationPoint(top.name) && tagName != "mglyph" && tagName != "malignmark" {
		return runtimeBootstrapHTMLNamespace
	}
	return top.childNamespace
}

func (state *runtimeBootstrapParserState) shouldBreakOutOfForeignContent(tagName string, attrs []html.Attribute) bool {
	if _, ok := runtimeBootstrapForeignBreakoutTags[tagName]; ok {
		return true
	}
	if tagName != "font" {
		return false
	}
	for _, attr := range attrs {
		switch strings.ToLower(attr.Key) {
		case "color", "face", "size":
			return true
		}
	}
	return false
}

func (state *runtimeBootstrapParserState) popForeignContent() {
	for len(state.elements) > 0 {
		top := state.elements[len(state.elements)-1]
		if top.namespace == runtimeBootstrapHTMLNamespace || top.childNamespace == runtimeBootstrapHTMLNamespace {
			return
		}
		state.elements = state.elements[:len(state.elements)-1]
	}
}

func (state *runtimeBootstrapParserState) elementNamespaces(tagName string, attrs []html.Attribute) (runtimeBootstrapNamespace, runtimeBootstrapNamespace) {
	parentNamespace := state.namespaceForTag(tagName)
	switch parentNamespace {
	case runtimeBootstrapHTMLNamespace:
		switch tagName {
		case "svg":
			return runtimeBootstrapSVGNamespace, runtimeBootstrapSVGNamespace
		case "math":
			return runtimeBootstrapMathMLNamespace, runtimeBootstrapMathMLNamespace
		default:
			return runtimeBootstrapHTMLNamespace, runtimeBootstrapHTMLNamespace
		}
	case runtimeBootstrapSVGNamespace:
		if isRuntimeBootstrapSVGHTMLIntegrationPoint(tagName) {
			return runtimeBootstrapSVGNamespace, runtimeBootstrapHTMLNamespace
		}
		return runtimeBootstrapSVGNamespace, runtimeBootstrapSVGNamespace
	case runtimeBootstrapMathMLNamespace:
		if tagName == "annotation-xml" && hasRuntimeBootstrapHTMLAnnotationEncoding(attrs) {
			return runtimeBootstrapMathMLNamespace, runtimeBootstrapHTMLNamespace
		}
		if isRuntimeBootstrapMathMLTextIntegrationPoint(tagName) {
			return runtimeBootstrapMathMLNamespace, runtimeBootstrapHTMLNamespace
		}
		return runtimeBootstrapMathMLNamespace, runtimeBootstrapMathMLNamespace
	default:
		return runtimeBootstrapHTMLNamespace, runtimeBootstrapHTMLNamespace
	}
}

func (state *runtimeBootstrapParserState) openElement(name string, namespace, childNamespace runtimeBootstrapNamespace, tokenType html.TokenType) {
	if namespace == runtimeBootstrapHTMLNamespace && isRuntimeBootstrapHTMLVoidElement(name) {
		return
	}
	if namespace != runtimeBootstrapHTMLNamespace && tokenType == html.SelfClosingTagToken {
		return
	}
	state.elements = append(state.elements, runtimeBootstrapOpenElement{
		name:           name,
		namespace:      namespace,
		childNamespace: childNamespace,
	})
}

func (state *runtimeBootstrapParserState) closeElement(tagName string, namespace runtimeBootstrapNamespace) {
	for index := len(state.elements) - 1; index >= 0; index-- {
		element := state.elements[index]
		if element.name == tagName && runtimeBootstrapEndTagMatches(element, namespace) {
			for _, popped := range state.elements[index:] {
				if popped.namespace == runtimeBootstrapHTMLNamespace && popped.name == runtimeBootstrapTemplateTag && state.templateDepth > 0 {
					state.templateDepth--
				}
			}
			state.elements = state.elements[:index]
			return
		}
		if runtimeBootstrapEndTagStopsAt(element, tagName, namespace) {
			return
		}
	}
}

func runtimeBootstrapEndTagStopsAt(element runtimeBootstrapOpenElement, tagName string, namespace runtimeBootstrapNamespace) bool {
	if element.namespace == runtimeBootstrapHTMLNamespace && element.name == runtimeBootstrapTemplateTag && tagName != runtimeBootstrapTemplateTag {
		return true
	}
	if namespace == runtimeBootstrapHTMLNamespace && element.namespace != runtimeBootstrapHTMLNamespace && element.childNamespace == runtimeBootstrapHTMLNamespace && element.name != tagName {
		return true
	}
	return namespace != runtimeBootstrapHTMLNamespace && element.namespace == runtimeBootstrapHTMLNamespace
}

func runtimeBootstrapEndTagMatches(element runtimeBootstrapOpenElement, namespace runtimeBootstrapNamespace) bool {
	if element.namespace == namespace {
		return true
	}
	return namespace == runtimeBootstrapHTMLNamespace && element.namespace != runtimeBootstrapHTMLNamespace && element.childNamespace == runtimeBootstrapHTMLNamespace
}

func isRuntimeBootstrapSVGHTMLIntegrationPoint(tagName string) bool {
	switch tagName {
	case "desc", "foreignobject", "title":
		return true
	default:
		return false
	}
}

func isRuntimeBootstrapMathMLTextIntegrationPoint(tagName string) bool {
	switch tagName {
	case "mi", "mo", "mn", "ms", "mtext":
		return true
	default:
		return false
	}
}

func hasRuntimeBootstrapHTMLAnnotationEncoding(attrs []html.Attribute) bool {
	for _, attr := range attrs {
		if strings.EqualFold(attr.Key, "encoding") && (strings.EqualFold(attr.Val, "text/html") || strings.EqualFold(attr.Val, "application/xhtml+xml")) {
			return true
		}
	}
	return false
}

func isRuntimeBootstrapHTMLVoidElement(tagName string) bool {
	switch tagName {
	case "area", "base", "basefont", "bgsound", "br", "col", "embed", "frame", "hr", "img", "input", "keygen", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func updateRuntimeBootstrapStartTag(tagName string, raw []byte, position, insertion, fallback int) (int, int) {
	switch tagName {
	case "script":
		if insertion < 0 {
			insertion = position
		}
	case "head":
		if fallback < 0 {
			fallback = position + len(raw)
		}
	case "body":
		if fallback < 0 {
			fallback = position
		}
	}
	return insertion, fallback
}

func isRuntimeBootstrapFallbackTag(tagName string) bool {
	switch tagName {
	case "head", "body", "html":
		return true
	default:
		return false
	}
}

func runtimeBootstrapToken(tokenizer *html.Tokenizer, tokenType html.TokenType) (html.Token, bool) {
	if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken && tokenType != html.EndTagToken {
		return html.Token{}, false
	}
	return tokenizer.Token(), true
}
