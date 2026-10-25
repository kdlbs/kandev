// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { openTaskSession, waitForAgentMessage } from "../../helpers/session";
import {
  readIdleSuspensionSnapshot,
  readIdleSuspensionState,
  readSessionContentMessageCount,
  readSessionMessageCount,
  updateWorkspaceIdlePolicy,
} from "../../helpers/idle-parking";

test.describe("mobile: workspace ACP idle suspension", () => {
  test("a message wakes the parked session and the touch composer delivers once", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const databasePath = path.join(backend.tmpDir, "kandev.db");
    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
      const policySwitch = testPage.getByTestId("workspace-idle-suspension-switch");
      const timeout = testPage.getByTestId("workspace-idle-timeout-input");
      await expect(policySwitch).toHaveAttribute("aria-checked", "false");
      await expect(timeout).toHaveValue("120");
      await expect(timeout).toBeDisabled();

      await policySwitch.tap();
      await timeout.fill("1");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).tap();
      await expect(floatingSave).not.toBeVisible();
      const saved = await apiClient.rawRequest("GET", `/api/v1/workspaces/${seedData.workspaceId}`);
      expect(saved.ok).toBe(true);
      const workspace = (await saved.json()) as {
        acp_idle_suspension_enabled: boolean;
        acp_idle_timeout_minutes: number;
      };
      expect(workspace.acp_idle_suspension_enabled).toBe(true);
      expect(workspace.acp_idle_timeout_minutes).toBe(1);

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `Mobile idle parking message ${Date.now()}`,
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("task creation did not return a session_id");
      await waitForAgentMessage(
        apiClient,
        task.session_id,
        "This is a simple mock response for e2e testing.",
      );

      const priorUserMessages = readSessionMessageCount(databasePath, task.session_id, "user");
      const priorAgentMessages = readSessionContentMessageCount(
        databasePath,
        task.session_id,
        "agent",
      );
      await expect
        .poll(() => JSON.stringify(readIdleSuspensionSnapshot(databasePath, task.session_id!)), {
          timeout: 110_000,
          intervals: [500, 1_000],
          message: "workspace policy suspends a settled mock ACP session on mobile",
        })
        .toContain('"idleSuspensionState":"suspended"');

      await apiClient.addUserMessage(task.id, task.session_id, "/e2e:simple-message");
      await expect
        .poll(() => readIdleSuspensionState(databasePath, task.session_id!), {
          timeout: 30_000,
          message: "an accepted message resumes its idle-suspended session",
        })
        .toBe("");
      await expect
        .poll(() => readSessionMessageCount(databasePath, task.session_id!, "user"), {
          timeout: 15_000,
        })
        .toBe(priorUserMessages + 1);
      await expect
        .poll(() => readSessionContentMessageCount(databasePath, task.session_id!, "agent"), {
          timeout: 30_000,
        })
        .toBe(priorAgentMessages + 1);

      await testPage.goto("/");
      const session = await openTaskSession(testPage, task.id);
      await expect
        .poll(() => readSessionContentMessageCount(databasePath, task.session_id!, "agent"), {
          timeout: 15_000,
        })
        .toBe(priorAgentMessages + 1);
      await expect(
        session
          .activeChat()
          .getByText("This is a simple mock response for e2e testing.", { exact: true }),
      ).toHaveCount(2);

      await session.sendMessageViaButton("/e2e:simple-message");
      await expect
        .poll(() => readSessionMessageCount(databasePath, task.session_id!, "user"), {
          timeout: 15_000,
        })
        .toBe(priorUserMessages + 2);
      await expect
        .poll(() => readSessionContentMessageCount(databasePath, task.session_id!, "agent"), {
          timeout: 30_000,
        })
        .toBe(priorAgentMessages + 2);
      await expect(
        session
          .activeChat()
          .getByText("This is a simple mock response for e2e testing.", { exact: true }),
      ).toHaveCount(3);
    } finally {
      await updateWorkspaceIdlePolicy(backend.baseUrl, seedData.workspaceId, false, 120);
    }
  });
});
