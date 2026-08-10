import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

async function gotoForgejoSettings(page: import("@playwright/test").Page) {
  await page.goto("/settings/integrations/forgejo");
  await page.getByTestId("forgejo-origin-input").waitFor();
}

test.describe("Forgejo settings", () => {
  test("shows the workspace-scoped connection form and shared unsaved state", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo`);
    await testPage.getByTestId("forgejo-origin-input").waitFor();
    await expect(testPage.getByTestId("forgejo-origin-input")).toHaveValue("");
    await expect(testPage.getByTestId("forgejo-token-input")).toHaveValue("");
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/workspace/${seedData.workspaceId}/integrations/forgejo$`),
    );

    await testPage.getByTestId("forgejo-origin-input").fill("https://forgejo.example");
    await testPage.getByTestId("forgejo-token-input").fill("forgejo-token");
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  });

  test("keeps Forgejo credential controls usable on a mobile viewport", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 393, height: 851 });
    await gotoForgejoSettings(testPage);

    await expect(testPage.getByTestId("forgejo-origin-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-token-input")).toBeVisible();
    await expect(testPage.getByTestId("forgejo-webhook-secret-input")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile Forgejo settings");
  });
});

test.describe("Forgejo queue", () => {
  test("links a queued issue to an existing Kandev task", async ({ testPage }) => {
    let linkedPayload: Record<string, unknown> | null = null;
    await testPage.route("**/api/v1/forgejo/queue?*", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          issues: [
            {
              repository: {
                owner: "acme",
                name: "app",
                full_name: "acme/app",
                default_branch: "main",
                html_url: "https://forgejo.example/acme/app",
              },
              issue: {
                number: 7,
                title: "Fix queue linking",
                state: "open",
                html_url: "https://forgejo.example/acme/app/issues/7",
                body: "",
              },
            },
          ],
          pull_requests: [],
        }),
      }),
    );
    await testPage.route("**/api/v1/forgejo/task-issues?*", async (route) => {
      linkedPayload = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          id: "link-7",
          task_id: linkedPayload.task_id,
          owner: "acme",
          repo: "app",
          issue_number: 7,
          issue_url: "https://forgejo.example/acme/app/issues/7",
          title: "Fix queue linking",
          state: "open",
        }),
      });
    });

    await testPage.goto("/forgejo");
    await expect(
      testPage.getByRole("link", { name: /acme\/app #7: Fix queue linking/ }),
    ).toBeVisible();
    await testPage.getByLabel("Existing Kandev task ID").fill("task-e2e-7");
    await testPage.getByRole("button", { name: "Link existing task" }).click();

    await expect(
      testPage.getByText("Linked Forgejo issue #7 to Kandev task task-e2e-7"),
    ).toBeVisible();
    expect(linkedPayload).toMatchObject({
      task_id: "task-e2e-7",
      owner: "acme",
      repo: "app",
      number: 7,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "Forgejo queue");
  });
});
