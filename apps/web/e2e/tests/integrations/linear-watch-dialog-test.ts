import { devices, expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import path from "node:path";
import { backendFixture, reportUnexpectedBackendFailure } from "../../fixtures/backend";
import { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";

export type WatchSeed = { repositoryPath: string; agentProfileId: string };

// This bounded fixture uses the managed backend without resetting shared seed state.
export const test = backendFixture.extend<
  { testPage: Page },
  { apiClient: ApiClient; seedData: WatchSeed }
>({
  apiClient: [
    async ({ backend }, use) => {
      await use(new ApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],
  seedData: [
    async ({ backend, apiClient }, use) => {
      const root = mkdtempSync(path.join(backend.tmpDir, "linear-watch-seed-"));
      const env = makeGitEnv(root);
      const git = (args: string[]) => execFileSync("git", ["-C", root, ...args], { env });
      try {
        git(["init", "-b", "main"]);
        writeFileSync(path.join(root, "README.md"), "Owned immutable worker seed\n");
        git(["add", "README.md"]);
        git(["commit", "-m", "test: seed owned worker baseline"]);
        const { agents } = await apiClient.listAgents();
        const profile = agents.find((agent) => agent.name === "mock-agent")?.profiles[0];
        expect(profile, "managed mock agent profile exists").toBeDefined();
        await use({ repositoryPath: root, agentProfileId: profile!.id });
      } finally {
        rmSync(root, { recursive: true, force: true });
      }
    },
    { scope: "worker" },
  ],
  testPage: async ({ browser, backend }, use, info) => {
    const context = await browser.newContext({
      ...(info.project.name === "mobile-chrome" ? devices["Pixel 5"] : {}),
      baseURL: backend.frontendUrl,
    });
    try {
      const page = await context.newPage();
      await page.addInitScript(
        ({ port }) => {
          localStorage.setItem("kandev.onboarding.completed", "true");
          window.__KANDEV_API_PORT = port;
        },
        { port: String(backend.port) },
      );
      await use(page);
    } finally {
      await context.close();
      reportUnexpectedBackendFailure(backend.tmpDir, info);
    }
  },
});
export { expect } from "@playwright/test";
