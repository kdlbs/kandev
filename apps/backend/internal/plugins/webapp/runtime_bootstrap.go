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
    window.parent.postMessage(message, "*");
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
		insertion, fallback = updateRuntimeBootstrapPosition(tokenizer, tokenType, raw, position, insertion, fallback)
		position += len(raw)
	}
	if position != len(entry) {
		return -1, ErrRuntimeBootstrapUnavailable
	}
	if insertion < 0 {
		insertion = fallback
	}
	if insertion < 0 || insertion > len(entry) {
		return -1, ErrRuntimeBootstrapUnavailable
	}
	return insertion, nil
}

func updateRuntimeBootstrapPosition(tokenizer *html.Tokenizer, tokenType html.TokenType, raw []byte, position, insertion, fallback int) (int, int) {
	name, hasTagName := runtimeBootstrapTagName(tokenizer, tokenType)
	if !hasTagName {
		return insertion, fallback
	}
	if tokenType == html.StartTagToken {
		switch strings.ToLower(name) {
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
	if fallback < 0 && (strings.EqualFold(name, "head") || strings.EqualFold(name, "body") || strings.EqualFold(name, "html")) {
		fallback = position
	}
	return insertion, fallback
}

func runtimeBootstrapTagName(tokenizer *html.Tokenizer, tokenType html.TokenType) (string, bool) {
	if tokenType != html.StartTagToken && tokenType != html.EndTagToken {
		return "", false
	}
	name, _ := tokenizer.TagName()
	return string(name), true
}
