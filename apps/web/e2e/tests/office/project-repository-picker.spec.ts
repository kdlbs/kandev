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
});
