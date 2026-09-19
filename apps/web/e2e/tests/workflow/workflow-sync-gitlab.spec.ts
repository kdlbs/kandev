import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

const GITLAB_PROJECT = "platform/kandev";

const SYNCED_WORKFLOW_DESCRIPTION = "rev 1 (2026-08-08) — synced from GitLab";

const VALID_WORKFLOW_YAML = `version: 1
type: kandev_workflow
workflows:
  - name: GitLab Synced Workflow
    description: "${SYNCED_WORKFLOW_DESCRIPTION}"
    steps:
      - name: Open
        position: 0
        color: bg-blue-500
        is_start_step: true
        allow_manual_move: true
      - name: Closed
        position: 1
        color: bg-green-500
        allow_manual_move: true`;

async function openGitLabSyncTab(testPage: import("@playwright/test").Page) {
  await testPage.getByTestId("workflow-sync-open").click();
  const dialog = testPage.getByTestId("workflow-sync-dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("tab", { name: "GitLab" }).click();
  return dialog;
}

test.describe("GitLab workflow sync", () => {
  test("syncs a workflow from a seeded GitLab repo file", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    await apiClient.mockGitLabAddRepoFiles(seedData.workspaceId, GITLAB_PROJECT, "main", [
      { path: ".kandev/workflows/e2e.yaml", content: VALID_WORKFLOW_YAML },
    ]);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const dialog = await openGitLabSyncTab(testPage);
    await dialog.getByTestId("workflow-sync-url-input").fill(GITLAB_PROJECT);
    await dialog.getByTestId("workflow-sync-branch-input").fill("main");
    await dialog.getByTestId("workflow-sync-directory-input").fill(".kandev/workflows");
    await dialog.getByTestId("workflow-sync-save").click();
    await expect(dialog).not.toBeVisible();

    const status = testPage.getByTestId("workflow-sync-status");
    await expect(status).toBeVisible();

    // A successful sync that changed something calls the router's refresh(),
    // which in this SPA is a literal window.location.reload() — a real
    // browser navigation, not a React re-render. Start waiting for the load
    // event before the click that triggers it (not after), otherwise a fast
    // reload can complete before the wait begins, or a locator can be
    // evaluated mid-navigation and throw "Execution context was destroyed".
    await Promise.all([
      testPage.waitForLoadState("load"),
      status.getByTestId("workflow-sync-now").click(),
    ]);

    // Post-reload hydration: wait for a stable element before touching the
    // DOM further. The "Workflow sync completed" toast itself is transient
    // and can auto-dismiss before an assertion runs, so it isn't checked
    // directly — the durable signal is the synced workflow card below.
    await expect(page.addWorkflowButton).toBeVisible();

    // The workflow list's own data fetch can still be in flight for a
    // moment after the reload finishes loading. findWorkflowCard is a
    // one-shot scan with its own internal 500ms-per-card timeout, which is
    // both too slow to poll with directly and (worse) can independently
    // disagree with a check done a moment earlier under different timing.
    // Poll for the card's own data-testid directly in one evaluateAll, so
    // there is exactly one source of truth for "is it there yet" — no
    // separate re-check afterward that could race against this one.
    let cardTestId: string | null = null;
    await expect
      .poll(
        async () => {
          try {
            cardTestId = await testPage
              .locator('[data-testid^="workflow-card-"]')
              .evaluateAll((elements, targetName) => {
                for (const element of elements) {
                  const input = element.querySelector("input");
                  if (input && (input as HTMLInputElement).value === targetName) {
                    return element.getAttribute("data-testid");
                  }
                }
                return null;
              }, "GitLab Synced Workflow");
          } catch (err) {
            // expect.poll does not retry on a thrown exception, only on a
            // value mismatch — so a transient "Execution context was
            // destroyed" (the reload's own document swap still settling,
            // which addWorkflowButton above can observe a moment before
            // evaluateAll's very next call does) must be swallowed here as
            // "not found yet" rather than aborting the whole poll on its
            // first tick. Any other error is a real failure and propagates.
            if (err instanceof Error && err.message.includes("Execution context was destroyed")) {
              return null;
            }
            throw err;
          }
          return cardTestId;
        },
        { message: "waiting for the synced workflow card to render", timeout: 15_000 },
      )
      .not.toBeNull();

    const card = testPage.getByTestId(cardTestId!);
    await expect(card).toBeVisible();
    await expect(card.getByText("Open")).toBeVisible();
    await expect(card.getByText("Closed")).toBeVisible();

    // Synced workflows are read-only: the description renders the synced
    // value but must not be editable, or a later reconcile would just
    // revert a user's in-UI edit.
    const descriptionInput = card.getByLabel("Description", { exact: true });
    await expect(descriptionInput).toHaveValue(SYNCED_WORKFLOW_DESCRIPTION);
    await expect(descriptionInput).toBeDisabled();
  });

  test("requires a project path before Save is enabled", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const dialog = await openGitLabSyncTab(testPage);
    await expect(dialog.getByTestId("workflow-sync-save")).toBeDisabled();

    await dialog.getByTestId("workflow-sync-url-input").fill(GITLAB_PROJECT);
    await expect(dialog.getByTestId("workflow-sync-save")).toBeEnabled();
  });

  test("removes sync through inline actions without a native dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    const configResponse = await apiClient.rawRequest(
      "POST",
      `/api/v1/workflow-sync/config?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
      {
        provider: "gitlab",
        project_path: GITLAB_PROJECT,
        branch: "main",
        path: ".kandev/workflows",
        interval_seconds: 300,
        poll_enabled: true,
      },
    );
    expect(configResponse.ok).toBe(true);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    let nativeDialogSeen = false;
    testPage.on("dialog", async (nativeDialog) => {
      nativeDialogSeen = true;
      await nativeDialog.dismiss();
    });

    const dialog = testPage.getByTestId("workflow-sync-dialog");
    await testPage.getByTestId("workflow-sync-open").click();
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("workflow-sync-remove").click();
    await expect(dialog.getByTestId("workflow-sync-remove-confirmation")).toBeVisible();
    await expect(testPage.getByRole("dialog")).toHaveCount(1);

    await dialog
      .getByTestId("workflow-sync-remove-confirmation")
      .getByRole("button", { name: "Cancel" })
      .click();
    await expect(dialog.getByTestId("workflow-sync-remove")).toBeVisible();
    expect(
      (
        await apiClient.rawRequest(
          "GET",
          `/api/v1/workflow-sync/config?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        )
      ).status,
    ).toBe(200);

    await dialog.getByTestId("workflow-sync-remove").click();
    await dialog.getByTestId("workflow-sync-remove-confirm").click();
    await expect(dialog).not.toBeVisible();
    expect(nativeDialogSeen).toBe(false);
    expect(
      (
        await apiClient.rawRequest(
          "GET",
          `/api/v1/workflow-sync/config?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        )
      ).status,
    ).toBe(204);
  });

  // A branch name containing a slash (a common convention, e.g.
  // "features/TICKET-123") can't always be told apart from the directory
  // that follows it when parsing a pasted link — this is exactly what
  // broke a real config in production. The Branch/Directory fields exist
  // specifically so a wrong guess is directly correctable.
  test("corrects a mis-parsed slash-containing branch via the Branch field", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const dialog = await openGitLabSyncTab(testPage);
    await dialog
      .getByTestId("workflow-sync-url-input")
      .fill(`${GITLAB_PROJECT}/-/tree/features/kegmil-location-adoption/.kandev/workflows`);

    // The parser can only assume a single-segment branch, so this is the
    // (wrong) guess it makes.
    await expect(dialog.getByTestId("workflow-sync-branch-input")).toHaveValue("features");
    await expect(dialog.getByTestId("workflow-sync-directory-input")).toHaveValue(
      "kegmil-location-adoption/.kandev/workflows",
    );

    await dialog
      .getByTestId("workflow-sync-branch-input")
      .fill("features/kegmil-location-adoption");
    await dialog.getByTestId("workflow-sync-directory-input").fill(".kandev/workflows");
    await dialog.getByTestId("workflow-sync-save").click();
    await expect(dialog).not.toBeVisible();

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/workflow-sync/config?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    const config = (await response.json()) as { branch: string; path: string };
    expect(config.branch).toBe("features/kegmil-location-adoption");
    expect(config.path).toBe(".kandev/workflows");
  });
});
