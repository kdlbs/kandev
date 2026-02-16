// Shared state for controlling Monaco's built-in TS/JS provider suppression.
// Separated into its own module (no heavy imports) so both monaco-loader.ts
// and lsp-client-manager.ts can import it without circular dependencies or
// pulling in the full monaco-editor bundle.

let _suppressed = false;

/** Returns true when built-in TS/JS providers should be suppressed (LSP active). */
export function isBuiltinTsSuppressed(): boolean {
  return _suppressed;
}

/** Set suppression state. Called synchronously from lsp-client-manager. */
export function setBuiltinTsSuppressed(suppressed: boolean): void {
  _suppressed = suppressed;
}
