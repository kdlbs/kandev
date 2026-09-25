import { describe, expect, it } from "vitest";
import {
  applyLspRegistrations,
  effectiveLspCapabilities,
  normalizeLspRegistrations,
} from "./lsp-dynamic-capabilities";
import { LSP_CLIENT_CAPABILITIES } from "./lsp-json-rpc";

describe("LSP dynamic capabilities", () => {
  it("rebuilds effective providers from retained registration options", () => {
    const registrations = normalizeLspRegistrations([
      {
        id: "definition",
        method: "textDocument/definition",
        registerOptions: { documentSelector: [{ language: "go" }] },
      },
      { id: "invalid", method: 3 },
    ]);
    const effective = effectiveLspCapabilities(
      { hoverProvider: true },
      new Map(registrations.map((r) => [r.id, r])),
    );

    expect(effective).toEqual({
      hoverProvider: true,
      definitionProvider: { documentSelector: [{ language: "go" }] },
    });
  });

  it("applies register and unregister requests without mutating prior state", () => {
    const original = new Map();
    const registered = applyLspRegistrations(original, "client/registerCapability", {
      registrations: [{ id: "hover", method: "textDocument/hover" }],
    });
    const unregistered = applyLspRegistrations(registered, "client/unregisterCapability", {
      unregisterations: [{ id: "hover", method: "textDocument/hover" }],
    });

    expect(original.size).toBe(0);
    expect(registered.size).toBe(1);
    expect(unregistered.size).toBe(0);
    expect(effectiveLspCapabilities(null, registered)).toEqual({ hoverProvider: true });
    expect(effectiveLspCapabilities(null, unregistered)).toBeNull();
  });

  it("advertises supported dynamic providers and maps semantic tokens", () => {
    const textDocument = LSP_CLIENT_CAPABILITIES.textDocument;
    expect(textDocument.completion.dynamicRegistration).toBe(true);
    expect(textDocument.hover.dynamicRegistration).toBe(true);
    expect(textDocument.definition.dynamicRegistration).toBe(true);
    expect(textDocument.references.dynamicRegistration).toBe(true);
    expect(textDocument.signatureHelp.dynamicRegistration).toBe(true);
    expect(textDocument.semanticTokens.dynamicRegistration).toBe(true);
    expect(textDocument.synchronization.dynamicRegistration).toBe(false);
    const dynamicMethods = Object.entries(textDocument)
      .filter(([, capability]) => {
        if (typeof capability !== "object" || capability === null) return false;
        return (
          "dynamicRegistration" in capability &&
          (capability as { dynamicRegistration?: unknown }).dynamicRegistration === true
        );
      })
      .map(([method]) => method)
      .sort();
    expect(dynamicMethods).toEqual(
      ["completion", "definition", "hover", "references", "semanticTokens", "signatureHelp"].sort(),
    );

    const registered = applyLspRegistrations(new Map(), "client/registerCapability", {
      registrations: [
        {
          id: "semantic",
          method: "textDocument/semanticTokens",
          registerOptions: { legend: { tokenTypes: ["class"], tokenModifiers: [] }, full: true },
        },
      ],
    });
    expect(effectiveLspCapabilities(null, registered)).toEqual({
      semanticTokensProvider: {
        legend: { tokenTypes: ["class"], tokenModifiers: [] },
        full: true,
      },
    });
    const resumed = new Map(
      normalizeLspRegistrations([...registered.values()]).map((r) => [r.id, r]),
    );
    const unregistered = applyLspRegistrations(resumed, "client/unregisterCapability", {
      unregisterations: [{ id: "semantic", method: "textDocument/semanticTokens" }],
    });
    expect(effectiveLspCapabilities(null, unregistered)).toBeNull();
  });
});
