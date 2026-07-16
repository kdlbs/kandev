import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/office-fixture";

/**
 * Project page — repository picker (chip-style, popover-driven).
 *
 * Surfaces under test:
 *   - `ProjectRepositoryPicker` (popover with cmdk search + custom URL)
 *   - `ProjectReposSection` chip rendering + remove flow
 *
 * Without this, an endpoint rename (`PATCH /office/projects/:id`)
 * or a payload-shape drift would silently break the user's only
 * way to attach a repo to a project.
 *
 * Onboarding does not seed a project (CompleteResult.ProjectID is
 * empty), so each test creates its own via the office API.
 */
async function createProject(
  apiClient: { rawRequest: (m: string, u: string, b?: unknown) => Promise<Response> },
  workspaceId: string,
  name: string,
): Promise<string> {
  const res = await apiClient.rawRequest(
    "POST",
    `/api/v1/office/workspaces/${workspaceId}/projects`,
    { name },
  );
  const body = (await res.json()) as { project?: { id?: string }; id?: string };
  return (body.project?.id ?? body.id) as string;
}

test.describe("Project repository picker", () => {
  test("paste a remote URL → chip appears → remove → empty state returns", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Project");
    await testPage.goto(`/office/projects/${projectId}`);
    await expect(testPage.getByText("Repositories").first()).toBeVisible({ timeout: 10_000 });

    // Empty state until something gets added.
    await expect(testPage.getByText("No repositories added yet.")).toBeVisible();

    // Open the popover and type a URL the picker treats as remote.
    await testPage.getByTestId("project-add-repository").click();
    const url = "https://github.com/example/repo.git";
    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);

    // The "Use this URL" row appears in the Add custom group; the
    // option value matches `__custom__:<query>` so we click by role.
    const customRow = testPage.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible({ timeout: 5_000 });
    await customRow.click();

    // Chip renders with the raw URL. Tooltip carries the full
    // string; the visible label may be truncated.
    const chip = testPage.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(chip).toBeVisible({ timeout: 10_000 });

    // Empty-state line disappears once a chip exists.
    await expect(testPage.getByText("No repositories added yet.")).toHaveCount(0);

    // Re-open the picker: the exact attached value must not offer a
    // duplicate custom row, but a prefix of it must stay addable —
    // partial overlap with existing entries cannot block literal input.
    await testPage.getByTestId("project-add-repository").click();
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);
    await expect(testPage.getByTestId("project-add-custom")).toHaveCount(0);
    await searchInput.fill(url.replace(/\.git$/, ""));
    await expect(testPage.getByTestId("project-add-custom")).toBeVisible();
    await testPage.keyboard.press("Escape");

    // Remove the chip; empty state comes back.
    await chip.getByTestId("project-repo-chip-remove").click();
    await expect(chip).toHaveCount(0, { timeout: 10_000 });
    await expect(testPage.getByText("No repositories added yet.")).toBeVisible();
  });

  test("typing a local path triggers the 'Add as local path' subtitle", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Local");
    await testPage.goto(`/office/projects/${projectId}`);
    await testPage.getByTestId("project-add-repository").click();

    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();

    // A bare absolute path is treated as a local-path entry, not a URL.
    await searchInput.fill("/Users/example/some-project");
    await expect(testPage.getByTestId("project-add-custom")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("project-add-custom")).toContainText(/Add as local path/i);
  });

  test("a near-miss workspace suggestion does not block adding the literal path", async ({
    apiClient,
    testPage,
    officeSeed,
    backend,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Overlap");
    // Register a workspace repo whose path is a superstring of the one
    // we'll type — the classic /work/app vs /work/app-old overlap.
    const oldPath = path.join(backend.tmpDir, "repos", "app-old");
    fs.mkdirSync(oldPath, { recursive: true });
    await apiClient.createRepository(officeSeed.workspaceId, oldPath, "main", { name: "app-old" });

    await testPage.goto(`/office/projects/${projectId}`);
    await testPage.getByTestId("project-add-repository").click();
    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();

    const newPath = path.join(backend.tmpDir, "repos", "app");
    await searchInput.fill(newPath);

    // Both the near-miss suggestion and the free-form row must be offered.
    await expect(testPage.getByRole("option", { name: /app-old/ })).toBeVisible();
    const customRow = testPage.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible();
    await customRow.click();

    // The literal path is attached — not the near-miss suggestion.
    const chip = testPage.getByTestId("project-repo-chip");
    await expect(chip).toHaveAttribute("data-repository-value", newPath, { timeout: 10_000 });
  });

  // The create-project dialog embeds the same picker as the detail page.
  // Without this, a dialog-side regression (picker not wired to the form
  // state, repositories dropped from the create payload) would silently
  // break attaching a repo at creation time.
  test("create-project dialog: pick a repo → create → repo persists on detail page", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/projects");
    await testPage
      .getByRole("button", { name: /New Project|Create your first project/ })
      .first()
      .click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const projectName = `Picker Create ${Date.now()}`;
    await dialog.locator("#project-name").fill(projectName);

    // Add a repository through the picker's custom-entry row.
    const url = "https://github.com/example/created-with-picker.git";
    await dialog.getByTestId("project-add-repository").click();
    const searchInput = dialog.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);
    const customRow = dialog.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible({ timeout: 5_000 });
    await customRow.click();

    // Chip renders inside the dialog before submitting.
    const dialogChip = dialog.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(dialogChip).toBeVisible();

    await dialog.getByRole("button", { name: "Create Project" }).click();
    await expect(dialog).toHaveCount(0, { timeout: 10_000 });

    // The new project card appears; opening it shows the persisted repo chip.
    await testPage.getByText(projectName).first().click();
    const detailChip = testPage.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(detailChip).toBeVisible({ timeout: 10_000 });
  });
});
