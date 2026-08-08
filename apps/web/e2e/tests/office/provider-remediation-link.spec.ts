import { test, expect } from "../../fixtures/office-fixture";

const REMEDIATION_URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

test.describe("Office run-error remediation link", () => {
  test("renders the allowlisted link on the failed-session chat entry", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Office remediation link", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "FAILED",
      agentProfileId: officeSeed.agentId,
      completedAt: "2026-08-07T15:15:44Z",
      metadata: {
        last_agent_error: {
          message: "AI_APICallError: 5-hour usage limit reached.",
          occurred_at: "2026-08-07T15:15:44Z",
          remediation_url: REMEDIATION_URL,
        },
      },
    });

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Office remediation link" })).toBeVisible({
      timeout: 10_000,
    });

    const link = testPage.getByTestId("remediation-link");
    await expect(link).toBeVisible({ timeout: 10_000 });
    await expect(link).toHaveAttribute("href", REMEDIATION_URL);
    await expect(link).toHaveAttribute("target", "_blank");
    await expect(link).toHaveAttribute("rel", "noopener noreferrer");

    // The destination exists only on the link — never as visible text.
    await expect(testPage.getByTestId("task-chat-root")).not.toContainText("opencode.ai");
  });

  test("renders no link for invalid or absent remediation metadata", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    for (const [title, metadata] of [
      [
        "Office invalid link",
        {
          last_agent_error: {
            message: "provider rejected the request.",
            occurred_at: "2026-08-07T15:15:44Z",
            remediation_url: "https://evil.example.com/x",
          },
        },
      ],
      [
        "Office no link",
        {
          last_agent_error: {
            message: "AI_APICallError: provider rejected the request.",
            occurred_at: "2026-08-07T15:15:44Z",
          },
        },
      ],
    ] as const) {
      const task = await apiClient.createTask(officeSeed.workspaceId, title, {
        workflow_id: officeSeed.workflowId,
      });
      await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
        assignee_agent_profile_id: officeSeed.agentId,
      });
      await apiClient.seedTaskSession(task.id, {
        state: "FAILED",
        agentProfileId: officeSeed.agentId,
        completedAt: "2026-08-07T15:15:44Z",
        metadata,
      });

      await testPage.goto(`/office/tasks/${task.id}`);
      await expect(testPage.getByRole("heading", { name: title })).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByTestId("run-error-resume-button")).toBeVisible({
        timeout: 10_000,
      });
      await expect(testPage.getByTestId("remediation-link")).toHaveCount(0);
    }
  });
});
