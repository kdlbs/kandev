export type LspDynamicRegistration = {
  id: string;
  method: string;
  registerOptions?: unknown;
};

const providerCapabilities: Record<string, string> = {
  "textDocument/completion": "completionProvider",
  "textDocument/hover": "hoverProvider",
  "textDocument/definition": "definitionProvider",
  "textDocument/references": "referencesProvider",
  "textDocument/signatureHelp": "signatureHelpProvider",
  "textDocument/semanticTokens": "semanticTokensProvider",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function normalizeLspRegistrations(value: unknown): LspDynamicRegistration[] {
  if (!Array.isArray(value)) return [];
  return value.filter(
    (registration): registration is LspDynamicRegistration =>
      isRecord(registration) &&
      typeof registration.id === "string" &&
      typeof registration.method === "string",
  );
}

export function applyLspRegistrations(
  current: ReadonlyMap<string, LspDynamicRegistration>,
  method: "client/registerCapability" | "client/unregisterCapability",
  params: unknown,
): Map<string, LspDynamicRegistration> {
  const next = new Map(current);
  if (!isRecord(params)) return next;
  if (method === "client/registerCapability") {
    for (const registration of normalizeLspRegistrations(params.registrations)) {
      next.set(registration.id, registration);
    }
    return next;
  }

  let unregistrations: unknown[] = [];
  if (Array.isArray(params.unregistrations)) unregistrations = params.unregistrations;
  else if (Array.isArray(params.unregisterations)) unregistrations = params.unregisterations;
  for (const registration of unregistrations) {
    if (isRecord(registration) && typeof registration.id === "string") {
      next.delete(registration.id);
    }
  }
  return next;
}

export function effectiveLspCapabilities(
  staticCapabilities: Record<string, unknown> | null,
  registrations: ReadonlyMap<string, LspDynamicRegistration>,
): Record<string, unknown> | null {
  if (!staticCapabilities && registrations.size === 0) return null;
  const effective = { ...(staticCapabilities ?? {}) };
  for (const registration of registrations.values()) {
    const capability = providerCapabilities[registration.method];
    if (!capability) continue;
    effective[capability] = registration.registerOptions ?? true;
  }
  return effective;
}
