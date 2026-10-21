import { afterEach, expect, it, vi } from "vitest";

const original = process.argv;
afterEach(() => {
  process.argv = original;
  vi.resetModules();
});

it("ordinary Playwright discovery excludes upgrade workloads", async () => {
  process.argv = ["node", "playwright", "test"];
  vi.resetModules();
  const { default: config } = await import("./playwright.config");
  expect(config.projects?.map((p) => p.name)).not.toContain("upgrade-desktop");
  expect(config.projects?.map((p) => p.name)).not.toContain("upgrade-mobile");
  expect(config.globalSetup).toBe("./global-setup.ts");
});

it.each([["--project=upgrade-desktop"], ["--project", "upgrade-mobile"]])(
  "explicit upgrade selection registers dedicated projects: %j",
  async (...args) => {
    process.argv = ["node", "playwright", "test", ...args];
    vi.resetModules();
    const { default: config } = await import("./playwright.config");
    expect(config.projects?.map((p) => p.name)).toContain("upgrade-desktop");
    expect(config.projects?.map((p) => p.name)).toContain("upgrade-mobile");
    expect(config.globalSetup).toBe("./upgrade-global-setup.ts");
  },
);
