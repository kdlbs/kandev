import { expect, test } from "../../fixtures/test-base";
import fs from "node:fs";
import path from "node:path";
import { managedRuntimeExecutionCacheKey } from "../../helpers/managed-runtime-recovery";
import { KanbanPage } from "../../pages/kanban-page";

const AGENT_NAME = "opencode-acp";
const PROFILE_NAME = "Recovered OpenCode profile";

test.describe("host utility managed runtime recovery", () => {
  test("recovers the model catalogue before task creation", async ({
    apiClient,
    backend,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the managed npm fixture uses a POSIX launcher");
    test.setTimeout(120_000);

    const wrapperSource = path.resolve(__dirname, "../../fixtures/managed-runtime-npx.sh");
    const wrapperPath = path.join(backend.tmpDir, "bin", "npx");
    const cacheRoot = path.join(backend.tmpDir, "managed-npm-cache");
    const mockAgentPath = path.resolve(__dirname, "../../../../backend/bin/mock-agent");
    fs.copyFileSync(wrapperSource, wrapperPath);
    fs.chmodSync(wrapperPath, 0o755);

    let releaseEnv: (() => Promise<void>) | undefined;
    let profileId = "";
    try {
      releaseEnv = await backend.useEnv({
        KANDEV_MOCK_AGENT: "true",
        KANDEV_E2E_MOCK_AGENT_PATH: mockAgentPath,
        KANDEV_E2E_NPX_MOCK_OTHERS: "true",
        NPM_CONFIG_CACHE: cacheRoot,
      });

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
            return `${agent?.model_config.status ?? "missing"}:${hasMockModel}:${packageSpec !== ""}`;
          },
          {
            timeout: 60_000,
            message: "OpenCode host capabilities should recover before task creation",
          },
        )
        .toBe("ok:true:true");

      const { agents: persistedAgents } = await apiClient.listAgents();
      const persistedAgent = persistedAgents.find((candidate) => candidate.name === AGENT_NAME);
      expect(persistedAgent).toBeDefined();
      const profile = await apiClient.createAgentProfile(persistedAgent!.id, PROFILE_NAME, {
        model: "mock-fast",
      });
      profileId = profile.id;

      const target = path.join(cacheRoot, "_npx", managedRuntimeExecutionCacheKey(packageSpec));
      expect(fs.existsSync(path.join(target, "stale-marker"))).toBe(false);
      expect(fs.readFileSync(path.join(target, "fresh-marker"), "utf8")).toBe("fresh\n");
      expect(
        fs.readFileSync(path.join(cacheRoot, "_npx", "0123456789abcdef", "sibling-marker"), "utf8"),
      ).toBe("sibling\n");
      expect(fs.readFileSync(path.join(cacheRoot, "offline-invocations"), "utf8").trim()).toBe(
        packageSpec,
      );
      expect(fs.readFileSync(path.join(cacheRoot, "online-invocations"), "utf8").trim()).toBe(
        packageSpec,
      );

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
      fs.rmSync(wrapperPath, { force: true });
      if (releaseEnv) await releaseEnv();
    }
  });
});
