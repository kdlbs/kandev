import fs from "node:fs";
import { expect, test } from "../fixtures/test-base";
import { prepareIncompleteManagedRuntimeProfile } from "./managed-runtime-recovery";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function registerManagedNpmProbeRecoveryTests() {
  test("profile discovery prepares an incomplete exact runtime without a repair action", async ({
    apiClient,
    backend,
    testPage,
  }, testInfo) => {
    test.skip(process.platform === "win32", "fixture uses POSIX scripts");
    test.setTimeout(150_000);
    const fixture = await prepareIncompleteManagedRuntimeProfile(apiClient, backend, {
      hostSubprocess: true,
    });
    try {
      const probeResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/agent-models/opencode-acp/") &&
          response.url().endsWith("/probe"),
      );
      await testPage.goto(`/settings/agents/opencode-acp/profiles/${fixture.profile.id}`);
      const probe = await probeResponse;
      const result = await probe.json();
      expect(result, "profile probe response").toMatchObject({ status: "ok" });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 60_000 },
      );
      const model = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(model).toBeEnabled();
      if (testInfo.project.name === "mobile-chrome") await model.tap();
      else await model.click();
      await expect(testPage.getByRole("option").first()).toBeVisible();
      await testPage.keyboard.press("Escape");
      expect(fixture.registry.requests()).toBeGreaterThan(0);
      expect(fs.existsSync(`${fixture.sharedRoot}/package.json`)).toBe(false);
      await assertNoDocumentHorizontalOverflow(testPage);
      expect((await apiClient.getAgentProfile(fixture.profile.id)).model).toBe(
        fixture.profile.model,
      );
    } catch (error) {
      if (fixture.npmInvocationLog) {
        for (const log of [fixture.npmInvocationLog, `${fixture.npmInvocationLog}.stderr`]) {
          if (fs.existsSync(log))
            await testInfo.attach("npm-fixture-log", { path: log, contentType: "text/plain" });
        }
      }
      await testInfo.attach("probe-backend-log", {
        path: backend.logPath,
        contentType: "text/plain",
      });
      throw error;
    } finally {
      await testPage.goto("/");
      await fixture.releaseBackendEnv();
      await apiClient.deleteAgentProfile(fixture.profile.id, true);
      await fixture.registry.close();
      fs.rmSync(fixture.cacheRoot, { recursive: true, force: true });
      for (const fixturePath of fixture.hostFixturePaths) fs.rmSync(fixturePath, { force: true });
    }
  });
}
