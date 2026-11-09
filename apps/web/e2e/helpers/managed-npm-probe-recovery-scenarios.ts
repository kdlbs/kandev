import fs from "node:fs";
import { expect, test } from "../fixtures/test-base";
import { prepareIncompleteManagedRuntimeProfile } from "./managed-runtime-recovery";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function registerManagedNpmProbeRecoveryTests() {
  test("managed npm review fixes: profile flags preserve discovery reuse and model options", async ({
    apiClient,
    backend,
    testPage,
  }, testInfo) => {
    test.skip(process.platform === "win32", "fixture uses POSIX scripts");
    test.setTimeout(150_000);
    const fixture = await prepareIncompleteManagedRuntimeProfile(apiClient, backend, {
      hostSubprocess: true,
      cliFlags: ["--dangerously-skip-permissions"],
    });
    try {
      const probeResponse = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/agent-models/opencode-acp/") &&
          response.url().endsWith("/probe"),
      );
      await testPage.goto(`/settings/agents/opencode-acp/profiles/${fixture.profile.id}`);
      const probe = await probeResponse;
      expect(await probe.json(), "flagged profile probe response").toMatchObject({
        status: "ok",
      });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 60_000 },
      );
      const requestsAfterRecovery = fixture.registry.archiveRequests();
      const model = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(model).toBeEnabled();
      if (testInfo.project.name === "mobile-chrome") await model.tap();
      else await model.click();
      await expect(testPage.getByRole("option").first()).toBeVisible();
      await testPage.keyboard.press("Escape");
      const refreshedProbe = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/agent-models/opencode-acp/") &&
          response.url().endsWith("/probe"),
      );
      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      if (testInfo.project.name === "mobile-chrome") await refresh.tap();
      else await refresh.click();
      expect(await (await refreshedProbe).json(), "reused flagged probe response").toMatchObject({
        status: "ok",
      });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 60_000 },
      );
      expect(fixture.registry.archiveRequests()).toBe(requestsAfterRecovery);
      const saved = await apiClient.getAgentProfile(fixture.profile.id);
      expect(saved.model).toBe(fixture.profile.model);
      expect(saved.cli_flags).toEqual(fixture.profile.cli_flags);
      await assertNoDocumentHorizontalOverflow(testPage);
    } catch (error) {
      if (fixture.npmInvocationLog) {
        for (const log of [fixture.npmInvocationLog, `${fixture.npmInvocationLog}.stderr`]) {
          if (fs.existsSync(log))
            await testInfo.attach("npm-fixture-log", { path: log, contentType: "text/plain" });
        }
      }
      await testInfo.attach("flagged-probe-backend-log", {
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

  test("reusable verified cache repair reuses the published profile discovery probe without a repair action", async ({
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
      const requestsAfterRecovery = fixture.registry.archiveRequests();
      const refreshedProbe = testPage.waitForResponse(
        (response) =>
          response.url().includes("/api/v1/agent-models/opencode-acp/") &&
          response.url().endsWith("/probe"),
      );
      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      if (testInfo.project.name === "mobile-chrome") await refresh.tap();
      else await refresh.click();
      const refreshed = await refreshedProbe;
      expect(await refreshed.json(), "reused profile probe response").toMatchObject({
        status: "ok",
      });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 60_000 },
      );
      expect(fixture.registry.archiveRequests()).toBe(requestsAfterRecovery);
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
