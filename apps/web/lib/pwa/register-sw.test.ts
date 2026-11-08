import { describe, expect, it } from "vitest";
import { shouldRegisterServiceWorker } from "./register-sw";

describe("shouldRegisterServiceWorker", () => {
  const base = { isProd: true, webdriver: false, isSecureContext: true };

  it("registers in a secure production browser", () => {
    Object.defineProperty(navigator, "serviceWorker", { value: {}, configurable: true });
    expect(shouldRegisterServiceWorker(base)).toBe(true);
  });

  it("skips dev builds, automated browsers and insecure contexts", () => {
    Object.defineProperty(navigator, "serviceWorker", { value: {}, configurable: true });
    expect(shouldRegisterServiceWorker({ ...base, isProd: false })).toBe(false);
    expect(shouldRegisterServiceWorker({ ...base, webdriver: true })).toBe(false);
    expect(shouldRegisterServiceWorker({ ...base, isSecureContext: false })).toBe(false);
  });
});
