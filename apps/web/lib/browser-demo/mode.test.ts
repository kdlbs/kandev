import { describe, expect, it, vi } from "vitest";
import { isBrowserDemoDevRouteAvailable, shouldInstallBrowserDemo } from "./mode";

describe("browser demo mode", () => {
  it("always enables the dedicated browser-demo build", () => {
    expect(
      shouldInstallBrowserDemo({
        env: { VITE_KANDEV_BROWSER_DEMO: "true" },
        pathname: "/",
      }),
    ).toBe(true);
  });

  it("enables /demo during Vite development and remembers the tab", () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: vi.fn((key: string) => values.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => values.set(key, value)),
    };

    expect(shouldInstallBrowserDemo({ env: { DEV: true }, pathname: "/demo", storage })).toBe(true);
    expect(shouldInstallBrowserDemo({ env: { DEV: true }, pathname: "/", storage })).toBe(true);
  });

  it("leaves normal production builds connected to the backend", () => {
    expect(shouldInstallBrowserDemo({ env: {}, pathname: "/demo" })).toBe(false);
    expect(isBrowserDemoDevRouteAvailable({ DEV: false })).toBe(false);
  });
});
