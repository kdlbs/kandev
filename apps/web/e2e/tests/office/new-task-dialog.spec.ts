import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * Regression coverage for the "New Task" dialog (ISSUE-7): it used to drop
 * the chosen assignee entirely (metadata carried it, the create request
 * never did) and wrote the reviewer/approver "Stages" picker into the dead
 * `execution_policy` metadata field, so a created task got no runner and a
 * misleading success toast. The fix sends `assignee_agent_profile_id` as a
 * top-level create field and removes the Stages picker.
 *
 * No spec drove the real dialog before this file — see
 * `new-task-dialog-create-payload.test.tsx` for the unit-level payload pin,
 * which mounts the component directly and never exercises the "New Task"
 * button, the pickers, or the resulting toast/backend round trip.
 */

async function openNewTaskDialog(testPage: import("@playwright/test").Page) {
  await testPage.goto("/office/tasks");
  // The global sidebar also has a "New Task" trigger (icon-only, aria-label
  // only) that opens this same dialog on office routes, so a bare
  // getByRole("button", { name: "New Task" }) is ambiguous. Disambiguate via
  // the toolbar button's icon (IconPlus / tabler-icon-plus) versus the
  // sidebar's IconSquarePlus.
  await testPage.locator('button:has(svg.tabler-icon-plus):has-text("New Task")').click();
  const dialog = testPage.getByTestId("office-new-issue-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

test.describe("Office New Task dialog", () => {
  test("creating a task with an assignee seats the runner and shows no stages picker", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    // Onboarding does not seed a default project (see
    // office-api-client.ts's createProject doc comment), so the picker needs
    // a real project created up front.
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      "New Task Dialog E2E Success Project",
    )) as { id: string; name: string };
    expect(project.id).toBeTruthy();

    const dialog = await openNewTaskDialog(testPage);

    // Regression: the reviewer/approver "Stages" picker (which wrote into
    // the dead `execution_policy` metadata field) is fully removed.
    await expect(dialog.getByText(/review stages/i)).toHaveCount(0);

    await dialog.getByPlaceholder("Task title").fill("New Task Dialog E2E Success");

    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();

    await dialog.getByRole("button", { name: "Assignee" }).click();
    await testPage.getByRole("button", { name: "CEO", exact: true }).click();

    const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
    await dialog.getByTestId("new-task-create-button").click();
    const response = await created;
    const body = (await response.json()) as { id?: string };
    expect(body.id).toBeTruthy();
    const taskId = body.id as string;

    await expect(testPage.locator("[data-sonner-toast]")).toContainText(/task created/i, {
      timeout: 10_000,
    });
    await expect(dialog).toBeHidden();

    // The create request carried the assignee at all (the pre-fix bug: the
    // dialog built `assignee_agent_profile_id` into `metadata` instead of
    // sending it as a top-level create field, so the backend never saw it
    // and seated no runner). GET /api/v1/office/tasks/:id wraps the office
    // dashboard's camelCase TaskDTO under a "task" key, distinct from the
    // snake_case shape the create POST itself returns.
    const stored = (await officeApi.getTask(taskId)) as {
      task: { assigneeAgentProfileId?: string; projectId?: string };
    };
    expect(stored.task.assigneeAgentProfileId).toBe(officeSeed.agentId);
    expect(stored.task.projectId).toBe(project.id);
  });

  test("a rejected create surfaces an error toast, not a success toast", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      "New Task Dialog E2E Failure Project",
    )) as { id: string; name: string };
    expect(project.id).toBeTruthy();

    const dialog = await openNewTaskDialog(testPage);

    await dialog.getByPlaceholder("Task title").fill("New Task Dialog E2E Failure");
    await dialog.getByRole("button", { name: "Project" }).click();
    await testPage.getByRole("button", { name: project.name, exact: true }).click();
    await dialog.getByRole("button", { name: "Assignee" }).click();
    await testPage.getByRole("button", { name: "CEO", exact: true }).click();

    // Force the backend to reject the create so the dialog's failure branch
    // (an error toast, draft preserved, dialog stays open) is exercised
    // without depending on a specific server-side validation rule.
    await testPage.route(
      (url) => url.pathname === "/api/v1/tasks",
      async (route) => {
        if (route.request().method() === "POST") {
          await route.fulfill({
            status: 422,
            contentType: "application/json",
            body: JSON.stringify({ error: "assignee agent profile not found" }),
          });
          return;
        }
        await route.continue();
      },
    );

    await dialog.getByTestId("new-task-create-button").click();

    const toast = testPage.locator("[data-sonner-toast]");
    await expect(toast).toContainText(/assignee agent profile not found/i, { timeout: 10_000 });
    await expect(toast).not.toContainText(/task created/i);
    // Failure never closes the dialog or clears the draft (only success does).
    await expect(dialog).toBeVisible();
    await expect(dialog.getByPlaceholder("Task title")).toHaveValue("New Task Dialog E2E Failure");
  });
});
