import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("isDebugUI", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("is true when window.__KANDEV_DEBUG is set at runtime", async () => {
    vi.stubEnv("VITE_KANDEV_DEBUG", "");
    vi.stubGlobal("window", { __KANDEV_DEBUG: true });
    const { isDebugUI } = await import("./config");
    expect(isDebugUI()).toBe(true);
  });

  it("is false in production with no flags", async () => {
    vi.stubEnv("VITE_KANDEV_DEBUG", "");
    vi.stubGlobal("window", {});
    const { isDebugUI } = await import("./config");
    expect(isDebugUI()).toBe(false);
  });

  it("is true when VITE_KANDEV_DEBUG=true", async () => {
    vi.stubEnv("VITE_KANDEV_DEBUG", "true");
    vi.stubGlobal("window", {});
    const { isDebugUI } = await import("./config");
    expect(isDebugUI()).toBe(true);
  });
});
