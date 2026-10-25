import { afterEach, expect, it, vi } from "vitest";

const original = process.argv;
afterEach(() => {
  process.argv = original;
  vi.unstubAllEnvs();
  vi.resetModules();
});

it("ordinary Playwright discovery excludes upgrade workloads", async () => {
  process.argv = ["node", "playwright", "test"];
  vi.stubEnv("KANDEV_UPGRADE_PROJECT", undefined);
  vi.resetModules();
  const { default: config } = await import("./playwright.config");
  expect(config.projects?.map((p) => p.name)).not.toContain("upgrade-desktop");
  expect(config.projects?.map((p) => p.name)).not.toContain("upgrade-mobile");
  expect(config.globalSetup).toBe("./global-setup.ts");
});

it.each(["upgrade-desktop", "upgrade-mobile"])(
  "runner selection %s survives worker reload without CLI project arguments",
  async (project) => {
    process.argv = ["node", "playwright-worker"];
    vi.stubEnv("KANDEV_UPGRADE_PROJECT", project);
    vi.resetModules();
    const { default: config } = await import("./playwright.config");
    expect(config.projects?.map((p) => p.name)).toContain("upgrade-desktop");
    expect(config.projects?.map((p) => p.name)).toContain("upgrade-mobile");
    expect(config.globalSetup).toBe("./upgrade-global-setup.ts");
  },
);
