import { expect, test } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import { managedRuntimeExecutionCacheKey } from "../../helpers/managed-runtime-recovery";
import { KanbanPage } from "../../pages/kanban-page";

const AGENT_NAME = "opencode-acp";
const PROFILE_NAME = "Recovered OpenCode profile";

test.describe("host utility native runtime", () => {
  test("uses the native binary for the model catalogue before task creation", async ({
    apiClient,
    backend,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the native fixture uses a POSIX npx sentinel");
    test.setTimeout(120_000);

    const npxPath = path.join(backend.tmpDir, "bin", "npx");
    const discoveryPath = path.join(backend.tmpDir, "bin", "opencode");
    const cacheRoot = path.join(backend.tmpDir, "managed-npm-cache");
    const mockAgentPath = path.resolve(__dirname, "../../../../backend/bin/mock-agent");
    const npxInvocationPath = path.join(cacheRoot, "npx-invocations");
    fs.writeFileSync(
      npxPath,
      `#!/bin/sh\nprintf 'unexpected npx invocation\\n' >> '${npxInvocationPath}'\nexit 1\n`,
      { mode: 0o755 },
    );
    fs.copyFileSync(mockAgentPath, discoveryPath);
    fs.chmodSync(discoveryPath, 0o755);

    const runtimeEnv = {
      KANDEV_MOCK_AGENT: "true",
      KANDEV_E2E_MOCK_AGENT_PATH: mockAgentPath,
      NPM_CONFIG_CACHE: cacheRoot,
    };
    let releaseEnv: (() => Promise<void>) | undefined;
    let profileId = "";
    try {
      await backend.restart(runtimeEnv);
      const { agents: persistedAgents } = await apiClient.listAgents();
      const persistedAgent = persistedAgents.find((candidate) => candidate.name === AGENT_NAME);
      expect(persistedAgent).toBeDefined();
      const profile = await apiClient.createAgentProfile(persistedAgent!.id, PROFILE_NAME, {
        model: "mock-fast",
      });
      profileId = profile.id;

      releaseEnv = await backend.useEnv(runtimeEnv);

      let packageSpec = "";
      await expect
        .poll(
          async () => {
            const { agents } = await apiClient.listAvailableAgents();
            const agent = agents.find((candidate) => candidate.name === AGENT_NAME);
            const runtime = agent?.runtime_update;
            packageSpec =
              runtime?.package && runtime.effective_version
                ? `${runtime.package}@${runtime.effective_version}`
                : "";
            const hasMockModel = agent?.model_config.available_models.some(
              (model) => model.id === "mock-fast",
            );
            const probeError = agent?.model_config.error ?? "";
            return `${agent?.model_config.status ?? "missing"}:${hasMockModel}:${packageSpec !== ""}:${probeError}`;
          },
          {
            timeout: 60_000,
            message: "OpenCode host capabilities should load before task creation",
          },
        )
        .toBe("ok:true:true:");

      const target = path.join(cacheRoot, "_npx", managedRuntimeExecutionCacheKey(packageSpec));
      expect(fs.existsSync(path.join(target, "stale-marker"))).toBe(false);
      expect(fs.existsSync(path.join(target, "fresh-marker"))).toBe(false);
      expect(fs.existsSync(npxInvocationPath)).toBe(false);

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });
      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const selector = dialog.getByTestId("agent-profile-selector");
      await selector.click();
      const option = testPage
        .getByRole("listbox")
        .getByRole("option", { name: PROFILE_NAME, exact: false });
      await expect(option).toBeVisible();
      await expect(option.getByTestId("agent-profile-model-probe-warning")).toHaveCount(0);
      await option.click();

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.model).toBe("mock-fast");
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      fs.rmSync(npxPath, { force: true });
      fs.rmSync(discoveryPath, { force: true });
      if (releaseEnv) await releaseEnv();
    }
  });
});
