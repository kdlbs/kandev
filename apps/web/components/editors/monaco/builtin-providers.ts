// Shared state for controlling Monaco's built-in TS/JS provider suppression.
// Separated into its own module (no heavy imports) so both monaco-loader.ts
// and lsp-client-manager.ts can import it without circular dependencies or
// pulling in the full monaco-editor bundle.

import type { LspMonacoProviderMethod } from "@/lib/lsp/lsp-provider-capabilities";

let lspProviderRegistrationDepth = 0;
type ModelSuppressionMatcher = (model: unknown) => boolean;
type ModelSuppression = {
  matches: ModelSuppressionMatcher;
  providerMethods: ReadonlySet<LspMonacoProviderMethod>;
};
const modelSuppressions = new Map<string, ModelSuppression>();

/** Returns true when an active TS/JS LSP replaces this provider for this model. */
export function isBuiltinTsSuppressed(
  model: unknown,
  providerMethod: LspMonacoProviderMethod,
): boolean {
  for (const suppression of modelSuppressions.values()) {
    if (suppression.providerMethods.has(providerMethod) && suppression.matches(model)) return true;
  }
  return false;
}

/** Register model and advertised-provider ownership for one active TS/JS LSP connection. */
export function registerBuiltinTsSuppression(
  ownerId: string,
  matches: ModelSuppressionMatcher,
  providerMethods: ReadonlySet<LspMonacoProviderMethod>,
): { dispose: () => void } {
  const suppression = { matches, providerMethods: new Set(providerMethods) };
  modelSuppressions.set(ownerId, suppression);
  return {
    dispose: () => {
      if (modelSuppressions.get(ownerId) === suppression) modelSuppressions.delete(ownerId);
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
