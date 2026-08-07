import { afterEach, describe, expect, it } from "vitest";

import { LOCALE_COOKIE, readLocaleCookie, writeLocaleCookie } from "./cookie";

function clearCookies() {
  for (const part of document.cookie.split(";")) {
    const name = part.split("=")[0]?.trim();
    if (name) document.cookie = `${name}=; path=/; max-age=0`;
  }
}

describe("locale cookie", () => {
  afterEach(clearCookies);

  it("round-trips a written locale", () => {
    writeLocaleCookie("pseudo");
    expect(readLocaleCookie()).toBe("pseudo");
  });

  it("returns undefined when the cookie is absent", () => {
    expect(readLocaleCookie()).toBeUndefined();
  });

  it("treats a malformed percent-escape as unset instead of throwing", () => {
    // `decodeURIComponent("%E0%A4%A")` throws URIError. This runs at module scope
    // via resolveInitialLocale, before React mounts, so a throw here is
    // unrecoverable and blanks the app with no error of any kind.
    document.cookie = `${LOCALE_COOKIE}=%E0%A4%A; path=/`;
    expect(() => readLocaleCookie()).not.toThrow();
    expect(readLocaleCookie()).toBeUndefined();
  });

  it("uses the shared cookie name", () => {
    writeLocaleCookie("en");
    expect(document.cookie).toContain(`${LOCALE_COOKIE}=en`);
  });

  it("does not confuse a similarly-prefixed cookie", () => {
    document.cookie = `${LOCALE_COOKIE}_other=xx; path=/`;
    expect(readLocaleCookie()).toBeUndefined();
  });
});
