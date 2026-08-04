import { afterEach, describe, expect, it } from "vitest";
import {
  isBuiltinTsSuppressed,
  isLspProviderRegistrationActive,
  setBuiltinTsSuppressed,
  withLspProviderRegistration,
} from "./builtin-providers";

afterEach(() => setBuiltinTsSuppressed(false));

describe("built-in TypeScript provider state", () => {
  it("keeps runtime suppression separate from LSP registration", () => {
    setBuiltinTsSuppressed(true);

    expect(isBuiltinTsSuppressed()).toBe(true);
    expect(isLspProviderRegistrationActive()).toBe(false);
    withLspProviderRegistration(() => {
      expect(isBuiltinTsSuppressed()).toBe(true);
      expect(isLspProviderRegistrationActive()).toBe(true);
    });
    expect(isLspProviderRegistrationActive()).toBe(false);
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
