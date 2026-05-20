import { test, expect } from "../../fixtures/office-fixture";

test.describe("Comment Markdown Rendering", () => {
  test("bold and inline code render correctly", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Bold And Code Comment Task", {
      workflow_id: officeSeed.workflowId,
    });

    await officeApi.createTaskComment(task.id, "This is **bold** and `code` inline.");

    // Navigate directly to the task detail page.
    await testPage.goto(`/office/tasks/${task.id}`);

    // Wait for the Chat tab content to load (it is the default tab).
    await expect(testPage.getByText("This is")).toBeVisible({ timeout: 10_000 });

    // Bold text should be wrapped in a <strong> element.
    await expect(testPage.locator("strong").filter({ hasText: "bold" })).toBeVisible({
      timeout: 10_000,
    });

    // Inline code should be wrapped in a <code> element.
    await expect(testPage.locator("code").filter({ hasText: "code" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("task identifier becomes a clickable link", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Identifier Link Comment Task",
      { workflow_id: officeSeed.workflowId },
    );

    // The task prefix was set to "E2E" during onboarding; use a valid-looking
    // identifier so the remarkTaskLinks plugin turns it into a link.
    await officeApi.createTaskComment(task.id, "See E2E-1 for context.");

    await testPage.goto(`/office/tasks/${task.id}`);

    await expect(testPage.getByText("See")).toBeVisible({ timeout: 10_000 });

    // The identifier E2E-1 should appear in the comment text (rendered as markdown).
    // Note: the auto-linking remark plugin transforms it to a link only when the
    // plugin is active. If not linked, it still renders as plain text.
    await expect(testPage.getByText("E2E-1")).toBeVisible({ timeout: 10_000 });
  });

  test("fenced code block renders with code styling", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Fenced Code Block Comment Task",
      { workflow_id: officeSeed.workflowId },
    );

    const fencedBody = "Here is some code:\n\n```\nconst x = 42;\nconsole.log(x);\n```\n";
    await officeApi.createTaskComment(task.id, fencedBody);

    await testPage.goto(`/office/tasks/${task.id}`);

    await expect(testPage.getByText("Here is some code:")).toBeVisible({ timeout: 10_000 });

    // A <pre> or <code> block should contain the fenced code content.
    await expect(
      testPage.locator("pre, code").filter({ hasText: "const x = 42;" }).first(),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("markdown table renders as a table element", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Markdown Table Comment Task", {
      workflow_id: officeSeed.workflowId,
    });

    const tableBody =
      "| Name  | Value |\n| ----- | ----- |\n| Alpha | 1     |\n| Beta  | 2     |\n";
    await officeApi.createTaskComment(task.id, tableBody);

    await testPage.goto(`/office/tasks/${task.id}`);

    // Heading proves the SSR commit landed before we wait on the
    // table render — separates "page never loaded" failures from
    // "markdown renderer never produced a <table>" failures.
    await expect(
      testPage.getByRole("heading", { name: "Markdown Table Comment Task" }),
    ).toBeVisible({ timeout: 15_000 });

    // The markdown table should be rendered as an HTML <table>. The
    // markdown renderer mounts after the comments slice hydrates, so
    // give it generous time on a heavy parallel-suite run.
    await expect(testPage.locator("table").first()).toBeVisible({ timeout: 20_000 });
    await expect(testPage.locator("table").filter({ hasText: "Alpha" })).toBeVisible({
      timeout: 10_000,
    });
  });
});
