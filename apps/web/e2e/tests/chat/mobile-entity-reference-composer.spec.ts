import { type Locator, type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import type { CreateTaskResponse } from "../../../lib/types/http";
import type { EntityReference } from "../../../lib/types/entity-reference";

function taskReference(workspaceId: string, taskId: string, title: string): EntityReference {
  return {
    version: 1,
    ref: `mention:v1:kandev:task:${workspaceId}:${taskId}`,
    provider: "kandev",
    kind: "task",
    id: taskId,
    title,
    url: `/t/${taskId}`,
    scope: workspaceId,
  };
}

function githubIssueReference(index: number): EntityReference {
  const id = `github-issue-${index}`;
  const scope = "github.example.test/acme/widgets";
  return {
    version: 1,
    ref: `mention:v1:github:issue:${encodeURIComponent(scope)}:${id}`,
    provider: "github",
    kind: "issue",
    id,
    key: String(100 + index),
    title: `Mobile Reference GitHub ${String(index).padStart(2, "0")}`,
    url: `https://github.example.test/acme/widgets/issues/${100 + index}`,
    scope,
  };
}

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Entity Reference Active Task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

async function expectPersistedReference(
  apiClient: ApiClient,
  sessionId: string,
  taskId: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => {
          const references = message.metadata?.entity_references;
          return (
            Array.isArray(references) &&
            references.some(
              (reference) =>
                typeof reference === "object" &&
                reference !== null &&
                (reference as Record<string, unknown>).id === taskId,
            )
          );
        });
      },
      { timeout: 15_000, message: `Wait for mobile reference to task ${taskId}` },
    )
    .toBe(true);
}

function visibleEditor(scope: Locator | Page): Locator {
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

test.describe("Mobile entity reference composer", () => {
  test("touch selection stays viewport-contained, scrollable, and sends durable metadata", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const references: EntityReference[] = [];
    for (let index = 1; index <= 5; index += 1) {
      const title = `Mobile Reference ${String(index).padStart(2, "0")}`;
      const task = await apiClient.seedTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
      references.push(taskReference(seedData.workspaceId, task.task_id, title));
    }
    const githubReferences = [1, 2, 3].map(githubIssueReference);
    const active = await createReadyTask(apiClient, seedData);
    if (!active.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    const session = await openTaskChat(testPage, active.id);

    await testPage.route("**/api/v1/workspaces/*/mentions/search?*", async (route) => {
      const query = new URL(route.request().url()).searchParams.get("q") ?? "";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          query,
          groups: [
            {
              source: "kandev_tasks",
              provider: "kandev",
              kind: "task",
              display_name: "Kandev tasks",
              kind_label: "Task",
              status: "ok",
              results: references,
            },
            {
              source: "github_issues",
              provider: "github",
              kind: "issue",
              display_name: "GitHub issues",
              kind_label: "Issue",
              status: "ok",
              results: githubReferences,
            },
          ],
        }),
      });
    });

    const editor = visibleEditor(session.activeChat());
    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("#Mobile Reference");

    const menu = testPage.getByTestId("entity-reference-menu");
    await expect(menu).toBeVisible({ timeout: 10_000 });
    await expect(menu.getByRole("option")).toHaveCount(
      references.length + githubReferences.length,
      { timeout: 10_000 },
    );
    const geometry = await menu.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const viewport = window.visualViewport;
      const viewportBounds = viewport
        ? {
            left: viewport.offsetLeft,
            top: viewport.offsetTop,
            right: viewport.offsetLeft + viewport.width,
            bottom: viewport.offsetTop + viewport.height,
          }
        : { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight };
      const listbox = element.querySelector<HTMLElement>("[role='listbox']");
      const rowHeights = Array.from(element.querySelectorAll<HTMLElement>("[role='option']")).map(
        (row) => row.getBoundingClientRect().height,
      );
      return {
        menu: { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom },
        viewport: viewportBounds,
        minRowHeight: Math.min(...rowHeights),
        listClientHeight: listbox?.clientHeight ?? 0,
        listScrollHeight: listbox?.scrollHeight ?? 0,
        listOverflowY: listbox ? getComputedStyle(listbox).overflowY : "",
        documentClientWidth: document.documentElement.clientWidth,
        documentScrollWidth: document.documentElement.scrollWidth,
      };
    });

    expect(geometry.menu.left).toBeGreaterThanOrEqual(geometry.viewport.left - 1);
    expect(geometry.menu.top).toBeGreaterThanOrEqual(geometry.viewport.top - 1);
    expect(geometry.menu.right).toBeLessThanOrEqual(geometry.viewport.right + 1);
    expect(geometry.menu.bottom).toBeLessThanOrEqual(geometry.viewport.bottom + 1);
    expect(geometry.minRowHeight).toBeGreaterThanOrEqual(44);
    expect(geometry.listScrollHeight).toBeGreaterThan(geometry.listClientHeight);
    expect(geometry.listOverflowY).toMatch(/auto|scroll/);
    expect(geometry.documentScrollWidth).toBeLessThanOrEqual(geometry.documentClientWidth);

    const selected = references[0];
    const option = testPage.getByRole("option").filter({ hasText: selected.title });
    await option.tap();
    await expect(menu).not.toBeVisible();
    await expect(editor.getByTestId("entity-reference-chip")).toHaveText(selected.title);
    await expect(editor).toBeFocused();

    await session.activeChat().getByTestId("submit-message-button").tap();
    await expectPersistedReference(apiClient, active.session_id, selected.id);
    await expect(
      session
        .activeChat()
        .locator(".chat-message-list:visible")
        .getByTestId("entity-reference-chip")
        .filter({ hasText: selected.title }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
