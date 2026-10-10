import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { openTaskSession, waitForAgentMessage } from "../../helpers/session";
import {
  readIdleSuspensionSnapshot,
  readIdleSuspensionState,
  updateWorkspaceIdlePolicy,
} from "../../helpers/idle-parking";

test.describe("workspace ACP idle suspension", () => {
  test("saves the policy and resumes the same conversation when the task is focused", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const databasePath = path.join(backend.tmpDir, "kandev.db");
    let targetSessionId = "";
    const focusLaunchRequestIds = new Set<string>();
    const focusLaunchResponses: Array<{ id: string; payload: unknown }> = [];
    testPage.on("websocket", (socket) => {
      socket.on("framesent", (frame) => {
        try {
          const message = JSON.parse(String(frame.payload)) as {
            id?: string;
            action?: string;
            payload?: { session_id?: string; activation_source?: string };
          };
          if (
            message.action === "session.launch" &&
            message.payload?.session_id === targetSessionId &&
            message.payload.activation_source === "session_focus" &&
            message.id
          ) {
            focusLaunchRequestIds.add(message.id);
          }
        } catch {
          // Ignore unrelated websocket frames.
        }
      });
      socket.on("framereceived", (frame) => {
        try {
          const message = JSON.parse(String(frame.payload)) as {
            id?: string;
            type?: string;
            payload?: unknown;
          };
          if (message.type === "response" && message.id && focusLaunchRequestIds.has(message.id)) {
            focusLaunchResponses.push({ id: message.id, payload: message.payload });
          }
        } catch {
          // Ignore unrelated websocket frames.
        }
      });
    });
    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
      const policySwitch = testPage.getByTestId("workspace-idle-suspension-switch");
      const timeout = testPage.getByTestId("workspace-idle-timeout-input");
      await expect(policySwitch).toHaveAttribute("aria-checked", "false");
      await expect(timeout).toHaveValue("120");
      await expect(timeout).toBeDisabled();

      await policySwitch.click();
      await timeout.fill("1");
      const floatingSave = testPage.getByTestId("settings-floating-save");
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible();
      const saved = await apiClient.rawRequest("GET", `/api/v1/workspaces/${seedData.workspaceId}`);
      expect(saved.ok).toBe(true);
      const savedWorkspace = (await saved.json()) as {
        acp_idle_suspension_enabled: boolean;
        acp_idle_timeout_minutes: number;
      };
      expect(savedWorkspace.acp_idle_suspension_enabled).toBe(true);
      expect(savedWorkspace.acp_idle_timeout_minutes).toBe(1);

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `Idle parking focus ${Date.now()}`,
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("task creation did not return a session_id");
      const sessionId = task.session_id;
      targetSessionId = sessionId;
      await waitForAgentMessage(
        apiClient,
        sessionId,
        "This is a simple mock response for e2e testing.",
      );
      await expect
        .poll(() => JSON.stringify(readIdleSuspensionSnapshot(databasePath, sessionId)), {
          timeout: 180_000,
          intervals: [500, 1_000],
          message: "workspace policy suspends a settled mock ACP session",
        })
        .toContain('"idleSuspensionState":"suspended"');

      await testPage.goto("/");
      const focusedSession = await openTaskSession(testPage, task.id);
      await expect.poll(() => focusLaunchResponses.length, { timeout: 10_000 }).toBe(1);
      await expect
        .poll(
          () => ({
            state: readIdleSuspensionState(databasePath, sessionId),
            focusLaunchRequestCount: focusLaunchRequestIds.size,
            focusLaunchResponses,
          }),
          {
            timeout: 30_000,
            message: "explicit task focus resumes the idle-suspended session",
          },
        )
        .toMatchObject({ state: "", focusLaunchRequestCount: 1 });
      await expect(focusedSession.activeChat()).toContainText(
        "This is a simple mock response for e2e testing.",
      );
      await expect
        .poll(() => readIdleSuspensionState(databasePath, sessionId), {
          timeout: 100_000,
          intervals: [500, 1_000],
          message: "the same runtime parks again after a fresh idle interval",
        })
        .toBe("suspended");
    } finally {
      await updateWorkspaceIdlePolicy(backend.baseUrl, seedData.workspaceId, false, 120);
    }
  });
});
