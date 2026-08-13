import { type Page } from "@playwright/test";
import { test as base, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";
import { waitForOfficeTaskSessionLive } from "../../helpers/office-launch";
import { AppSidebarPage } from "../../pages/app-sidebar-page";

/**
 * Regression E2E tests for office issue chat features.
 *
 * These tests cover bugs that were found and fixed:
 * 1. Auto-bridge: agent session response must appear as a task_comment
 * 2. Sidebar agents: CEO agent must appear in sidebar after onboarding
 * 3. Store hydration: office slice must be hydrated during SSR
 */

type IssueChatFixtures = {
  officeApi: OfficeApiClient;
  chatSeed: {
    workspaceId: string;
    agentId: string;
    taskId: string;
  };
};

const test = base.extend<{ testPage: Page }, IssueChatFixtures>({
  officeApi: [
    async ({ backend }, use) => {
      await use(new OfficeApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],

  chatSeed: [
    async ({ officeApi, apiClient, seedData }, use) => {
      const result = (await officeApi.completeOnboarding({
        workspaceName: "Chat Test Workspace",
        taskPrefix: "CT",
        agentName: "CEO",
        agentProfileId: seedData.agentProfileId,
        executorPreference: "local_pc",
        taskTitle: "Introduce yourself",
        taskDescription: "Say hello and describe your role",
      })) as { workspaceId: string; agentId: string; projectId: string; taskId?: string };

      if (!result.taskId) {
        throw new Error("completeOnboarding did not return a taskId");
      }

      // Wait for the agent runtime to come up before any test navigates.
      await waitForOfficeTaskSessionLive(apiClient, result.taskId);

      await use({
        workspaceId: result.workspaceId,
        agentId: result.agentId,
        taskId: result.taskId,
      });
    },
    { scope: "worker" },
  ],

  testPage: async ({ testPage: basePage, apiClient, chatSeed, seedData }, use) => {
    await apiClient.saveUserSettings({
      workspace_id: chatSeed.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
    await use(basePage);
  },
});

test.describe("Office issue chat", () => {
  test("agent response is auto-bridged as a task comment", async ({ chatSeed, officeApi }) => {
    test.setTimeout(20_000);

    // Poll for the auto-bridged comment — the event handler runs async
    // via maybeAsync (goroutine) and needs the DB fallback for streaming
    // agents where Data.Text is empty.
    let agentComment: Record<string, unknown> | undefined;
    // `agentComment` is captured on each poll so the assertions below read the
    // last observed value.
    await expect
      .poll(
        async () => {
          const res = await officeApi.listTaskComments(chatSeed.taskId);
          const comments = (res as { comments?: Record<string, unknown>[] }).comments ?? [];
          agentComment = comments.find(
            (c) => (c.authorType as string) === "agent" || (c.author_type as string) === "agent",
          );
          return Boolean(agentComment);
        },
        { timeout: 15_000, message: "no agent comment appeared" },
      )
      .toBe(true);

    expect(agentComment, "agent response must be auto-bridged as a task comment").toBeDefined();
    expect((agentComment!.body as string).length).toBeGreaterThan(0);
    expect(agentComment!.source).toBe("session");
  });

  test("sidebar shows CEO agent after onboarding", async ({ testPage }) => {
    test.setTimeout(15_000);

    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    // The sidebar must show the CEO agent link — not "No agents yet".
    // Post-overhaul the Agents list lives in the unified AppSidebar's
    // COLLAPSIBLE Agents section, which defaults to collapsed on `/office`.
    // Expand it first, then assert the agent `<Link>` (accessible name = agent
    // name; the avatar is aria-hidden), scoped to the rail.
    const sidebar = new AppSidebarPage(testPage);
    await sidebar.expandSection("Agents");
    await expect(sidebar.root.getByRole("link", { name: /CEO/i }).first()).toBeVisible({
      timeout: 5_000,
    });
    // "No agents yet" must NOT be visible.
    await expect(sidebar.root.getByText("No agents yet")).not.toBeVisible();
  });

  // NOTE (overhaul): the "sidebar shows task count badge" test was removed.
  // Pre-overhaul the office sidebar had a dedicated "Tasks" navigation <Link>
  // whose accessible name carried a count badge. The unified AppSidebar
  // replaced that with a collapsible "Tasks" *section header* (a <button>
  // rendered by app-sidebar-section.tsx, with a TasksViewPicker header action)
  // — there is no longer an in-sidebar Tasks link, and no count badge on it.
  // The feature the test asserted no longer exists, so the test was deleted
  // rather than rewritten to assert a different surface.

  test("issue list shows the onboarding task", async ({ testPage }) => {
    test.setTimeout(15_000);

    await testPage.goto("/office/tasks");
    await expect(testPage.getByText("Introduce yourself", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});
