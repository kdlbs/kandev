import { describe, expect, it } from "vitest";
import {
  isBuiltinTsSuppressed,
  isLspProviderRegistrationActive,
  registerBuiltinTsSuppression,
  withLspProviderRegistration,
} from "./builtin-providers";

describe("built-in TypeScript provider state", () => {
  it("keeps model-scoped suppression separate from LSP registration", () => {
    const ownedModel = {};
    const secondOwnedModel = {};
    const suppression = registerBuiltinTsSuppression(
      "session:typescript:1",
      (model) => model === ownedModel,
      new Set(["provideCompletionItems"]),
    );
    const otherSuppression = registerBuiltinTsSuppression(
      "other-session:typescript:1",
      (model) => model === secondOwnedModel,
      new Set(["provideHover"]),
    );

    try {
      expect(isBuiltinTsSuppressed(ownedModel, "provideCompletionItems")).toBe(true);
      expect(isBuiltinTsSuppressed(ownedModel, "provideHover")).toBe(false);
      expect(isBuiltinTsSuppressed(secondOwnedModel, "provideHover")).toBe(true);
      expect(isBuiltinTsSuppressed(secondOwnedModel, "provideCompletionItems")).toBe(false);
      expect(isLspProviderRegistrationActive()).toBe(false);
      withLspProviderRegistration(() => {
        expect(isBuiltinTsSuppressed(ownedModel, "provideCompletionItems")).toBe(true);
        expect(isLspProviderRegistrationActive()).toBe(true);
      });
      expect(isLspProviderRegistrationActive()).toBe(false);
      suppression.dispose();
      expect(isBuiltinTsSuppressed(ownedModel, "provideCompletionItems")).toBe(false);
      expect(isBuiltinTsSuppressed(secondOwnedModel, "provideHover")).toBe(true);
    } finally {
      suppression.dispose();
      otherSuppression.dispose();
    }

    expect(isBuiltinTsSuppressed(ownedModel, "provideCompletionItems")).toBe(false);
    expect(isBuiltinTsSuppressed(secondOwnedModel, "provideHover")).toBe(false);
  });

  it("restores the registration guard when provider setup throws", () => {
    expect(() =>
      withLspProviderRegistration(() => {
        throw new Error("provider setup failed");
      }),
    ).toThrow("provider setup failed");
    expect(isLspProviderRegistrationActive()).toBe(false);
  });
});
