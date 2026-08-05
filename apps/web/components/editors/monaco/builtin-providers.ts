// Shared state for controlling Monaco's built-in TS/JS provider suppression.
// Separated into its own module (no heavy imports) so both monaco-loader.ts
// and lsp-client-manager.ts can import it without circular dependencies or
// pulling in the full monaco-editor bundle.

let lspProviderRegistrationDepth = 0;
type ModelSuppressionMatcher = (model: unknown) => boolean;
const modelSuppressions = new Map<string, ModelSuppressionMatcher>();

/** Returns true when an active TS/JS LSP owns this specific Monaco model. */
export function isBuiltinTsSuppressed(model: unknown): boolean {
  for (const matches of modelSuppressions.values()) {
    if (matches(model)) return true;
  }
  return false;
}

/** Register model ownership for one active TS/JS LSP connection. */
export function registerBuiltinTsSuppression(
  ownerId: string,
  matches: ModelSuppressionMatcher,
): { dispose: () => void } {
  modelSuppressions.set(ownerId, matches);
  return {
    dispose: () => {
      if (modelSuppressions.get(ownerId) === matches) modelSuppressions.delete(ownerId);
    },
  };
}

/** Returns true only while the LSP client synchronously registers providers. */
export function isLspProviderRegistrationActive(): boolean {
  return lspProviderRegistrationDepth > 0;
}

/** Distinguish LSP providers from Monaco's lazy built-ins during registration. */
export function withLspProviderRegistration<T>(register: () => T): T {
  lspProviderRegistrationDepth++;
  try {
    return register();
  } finally {
    lspProviderRegistrationDepth--;
  }
}
