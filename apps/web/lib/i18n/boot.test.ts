import { afterEach, describe, expect, it } from "vitest";

import type { BootPayload } from "@/src/boot-payload";

import { pseudoLocaleAvailable, resolveInitialLocale } from "./boot";
import { LOCALE_COOKIE } from "./cookie";

function clearLocaleCookie() {
  document.cookie = `${LOCALE_COOKIE}=; path=/; max-age=0`;
}

function payloadWithLocale(locale?: string): BootPayload {
  return { runtime: locale ? { locale } : {}, initialState: {} };
}

describe("resolveInitialLocale", () => {
  afterEach(clearLocaleCookie);

  it("prefers the boot payload locale", () => {
    document.cookie = `${LOCALE_COOKIE}=en; path=/`;
    expect(resolveInitialLocale(payloadWithLocale("pseudo"))).toBe("pseudo");
  });

  it("falls back to the cookie when the payload has no locale", () => {
    document.cookie = `${LOCALE_COOKIE}=pseudo; path=/`;
    expect(resolveInitialLocale(payloadWithLocale())).toBe("pseudo");
  });

  it("defaults to en when neither payload nor cookie is present", () => {
    expect(resolveInitialLocale(undefined)).toBe("en");
  });

  it("coerces an unknown payload locale to en", () => {
    expect(resolveInitialLocale(payloadWithLocale("klingon"))).toBe("en");
  });
});

describe("pseudoLocaleAvailable", () => {
  it("is true when the Go shell reports a dev or e2e profile", () => {
    // The e2e harness serves a PRODUCTION bundle, so import.meta.env.PROD is
    // true there; only this payload flag can distinguish it from a release.
    expect(pseudoLocaleAvailable({ runtime: { nonProduction: true } } as BootPayload)).toBe(true);
  });

  it("falls back to the build mode when the payload says nothing", () => {
    expect(pseudoLocaleAvailable(undefined)).toBe(!import.meta.env.PROD);
    expect(pseudoLocaleAvailable({ runtime: {} } as BootPayload)).toBe(!import.meta.env.PROD);
  });
});
