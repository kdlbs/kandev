import * as monacoImport from "monaco-editor";
import type { Monaco } from "@monaco-editor/react";
import type { IDisposable } from "monaco-editor";
import { loader } from "@monaco-editor/react";
import { isBuiltinTsSuppressed, isLspProviderRegistrationActive } from "./builtin-providers";
import type { LspMonacoProviderMethod } from "@/lib/lsp/lsp-provider-capabilities";

// Cast to Monaco type (from @monaco-editor/react) which has the full
// languages.typescript typings. The main 'monaco-editor' export marks
// languages.typescript as deprecated in favor of top-level typescript
// namespace, but @monaco-editor/react's Monaco type still uses it.
const monaco = monacoImport as unknown as Monaco;

// ---------------------------------------------------------------------------
// Built-in TS/JS provider suppression
// ---------------------------------------------------------------------------
// Monaco v0.55.1's setModeConfiguration fires onDidChange, but the
// tsMode.js setupMode() function never subscribes to it — providers are
// registered once and never torn down. So we intercept provider registration
// to wrap built-in TS/JS providers with a model-ownership check. When an LSP
// owns the requested model and advertises the overlapping feature, the wrapper
// returns null/empty instead of calling the original. Built-ins remain available
// to unrelated sessions, models, and features the external server did not offer.
//
// Key: the LSP client marks its own synchronous provider registration with a
// dedicated guard. Every other TS/JS provider is treated as a Monaco built-in
// and wrapped, even if it loads lazily while runtime suppression is already on.
// ---------------------------------------------------------------------------

const TS_LANGUAGES = new Set(["typescript", "javascript", "typescriptreact", "javascriptreact"]);

// Configure workers before Monaco initializes.
// The `new URL('...', import.meta.url)` pattern is recognized by Turbopack/webpack
// to bundle each worker as a separate chunk.
if (typeof window !== "undefined") {
  self.MonacoEnvironment = {
    getWorker(_workerId: string, label: string) {
      switch (label) {
        case "json":
          return new Worker(
            new URL("monaco-editor/esm/vs/language/json/json.worker.js", import.meta.url),
            { type: "module" },
          );
        case "css":
        case "scss":
        case "less":
          return new Worker(
            new URL("monaco-editor/esm/vs/language/css/css.worker.js", import.meta.url),
            { type: "module" },
          );
        case "html":
        case "handlebars":
        case "razor":
          return new Worker(
            new URL("monaco-editor/esm/vs/language/html/html.worker.js", import.meta.url),
            { type: "module" },
          );
        case "typescript":
        case "javascript":
          return new Worker(
            new URL("monaco-editor/esm/vs/language/typescript/ts.worker.js", import.meta.url),
            { type: "module" },
          );
        default:
          return new Worker(
            new URL("monaco-editor/esm/vs/editor/editor.worker.js", import.meta.url),
            { type: "module" },
          );
      }
    },
  };

  // Tell @monaco-editor/react to use local monaco instance (no CDN fetch)
  loader.config({ monaco: monacoImport });

  // Expose on window for code that accesses window.monaco (e.g. keybindings in editor)
  (window as { monaco?: Monaco }).monaco = monaco;

  // Wrap built-in TS/JS provider registration so we can suppress them at call
  // time when an external LSP is active. This MUST happen before the first TS
  // model is created (which triggers the lazy tsMode setup), and since this
  // module runs at import time, it's guaranteed to be in place early enough.
  const langs = monacoImport.languages;

  type ProviderRegistrationFn = (
    selector: string,
    provider: Record<string, unknown>,
    ...rest: unknown[]
  ) => IDisposable;

  function wrapRegistration(
    original: ProviderRegistrationFn,
    methodName: LspMonacoProviderMethod,
    emptyResult: unknown,
  ): ProviderRegistrationFn {
    return function (selector: string, provider: Record<string, unknown>, ...rest: unknown[]) {
      // Only the LSP client's explicitly guarded registrations pass through.
      // Lazy Monaco registrations remain wrapped regardless of suppression.
      if (
        typeof selector === "string" &&
        TS_LANGUAGES.has(selector) &&
        typeof provider[methodName] === "function" &&
        !isLspProviderRegistrationActive()
      ) {
        const origMethod = (provider[methodName] as (...a: unknown[]) => unknown).bind(provider);
        const wrapped = Object.create(provider);
        wrapped[methodName] = function (...args: unknown[]) {
          if (isBuiltinTsSuppressed(args[0], methodName)) return emptyResult;
          return origMethod(...args);
        };
        return original.call(langs, selector, wrapped, ...rest);
      }
      return original.call(langs, selector, provider, ...rest);
    };
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- monkey-patching Monaco internals
  const l = langs as Record<string, any>;
  l.registerHoverProvider = wrapRegistration(
    l.registerHoverProvider.bind(langs),
    "provideHover",
    null,
  );
  l.registerCompletionItemProvider = wrapRegistration(
    l.registerCompletionItemProvider.bind(langs),
    "provideCompletionItems",
    { suggestions: [] },
  );
  l.registerSignatureHelpProvider = wrapRegistration(
    l.registerSignatureHelpProvider.bind(langs),
    "provideSignatureHelp",
    null,
  );
  l.registerDefinitionProvider = wrapRegistration(
    l.registerDefinitionProvider.bind(langs),
    "provideDefinition",
    null,
  );
  l.registerReferenceProvider = wrapRegistration(
    l.registerReferenceProvider.bind(langs),
    "provideReferences",
    null,
  );
}

export { monaco };
