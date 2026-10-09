import { expect, test } from "../../fixtures/test-base";
import { DatabaseSync } from "../../helpers/node-sqlite";
import fs from "node:fs";
import path from "node:path";
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
    fs.writeFileSync(
      discoveryPath,
      `#!/bin/sh\nif [ "$1" = "--version" ]; then printf 'opencode 1.18.5\\n'; exit 0; fi\nexec "${mockAgentPath}" "$@"\n`,
      { mode: 0o755 },
    );

    const runtimeEnv = {
      KANDEV_MOCK_AGENT: "true",
      KANDEV_E2E_MOCK_AGENT_PATH: mockAgentPath,
      NPM_CONFIG_CACHE: cacheRoot,
    };
    let releaseEnv: (() => Promise<void>) | undefined;
    let profileId = "";
    try {
      await backend.restart(runtimeEnv);
      const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
      try {
        database.exec("PRAGMA busy_timeout = 10000");
        database
          .prepare(
            "INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
          )
          .run(
            "managed_runtime.opencode.selection",
            JSON.stringify({
              schema_version: 1,
              family: "v1",
              source: "native",
              package: "opencode-ai",
              applied_default_version: "1.18.32",
              revision: 1,
            }),
          );
      } finally {
        database.close();
      }
      await backend.restart(runtimeEnv);
      const { agents: persistedAgents } = await apiClient.listAgents();
      const persistedAgent = persistedAgents.find((candidate) => candidate.name === AGENT_NAME);
      expect(persistedAgent).toBeDefined();
      const profile = await apiClient.createAgentProfile(persistedAgent!.id, PROFILE_NAME, {
        model: "mock-fast",
      });
      profileId = profile.id;

      releaseEnv = await backend.useEnv(runtimeEnv);

      await expect
        .poll(
          async () => {
            const { agents } = await apiClient.listAvailableAgents();
            const agent = agents.find((candidate) => candidate.name === AGENT_NAME);
            const hasMockModel = agent?.model_config.available_models.some(
              (model) => model.id === "mock-fast",
            );
            const probeError = agent?.model_config.error ?? "";
            return {
              status: agent?.model_config.status ?? "missing",
              hasMockModel: hasMockModel ?? false,
              probeError,
            };
          },
          {
            timeout: 60_000,
            message: "OpenCode host capabilities should load before task creation",
          },
        )
        .toMatchObject({
          status: "ok",
          hasMockModel: true,
          probeError: "",
        });

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
