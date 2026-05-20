import { test, expect } from "../../fixtures/office-fixture";

/**
 * Skills page Monaco read-only contract.
 *
 * `apps/web/app/office/workspace/skills/skill-detail.tsx` flips the
 * `readOnly` prop on the Monaco-based ScriptEditor based on
 * `skill.isSystem` (and the source-meta read-only signal). A regression
 * that drops the prop or inverts the boolean would silently allow
 * editing kandev-shipped SKILL.md content in the UI. These specs pin
 * the contract via a sibling marker rendered next to the editor wrapper.
 */
test.describe("Office skills - Monaco read-only mode", () => {
  test("system skills render Monaco in read-only mode", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    // Prime the workspace so SSR sees the bundled system skills.
    const priming = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/workspaces/${officeSeed.workspaceId}/skills`,
    );
    expect(priming.ok).toBe(true);

    await testPage.goto("/office/workspace/skills");

    // Expand the (collapsed by default) System group.
    const systemToggle = testPage.locator('button:has(span:text-is("System"))').first();
    await expect(systemToggle).toBeVisible({ timeout: 10_000 });
    await systemToggle.click();

    // Pick a known-bundled system skill — kandev-protocol is asserted
    // present in system-skills.spec.ts so it's a stable target here.
    const protocolRow = testPage.locator("button", { hasText: "kandev-protocol" }).first();
    await expect(protocolRow).toBeVisible({ timeout: 5_000 });
    await protocolRow.click();

    const editor = testPage.getByTestId("skill-content-editor");
    await expect(editor).toBeVisible({ timeout: 10_000 });
    await expect(editor).toHaveAttribute("data-readonly", "true");
    await expect(testPage.getByTestId("skill-content-readonly")).toBeAttached();
  });

  test("workspace skills are editable", async ({ officeApi, testPage, officeSeed }) => {
    const unique = `editable-${Date.now()}`;
    const created = (await officeApi.createSkill(officeSeed.workspaceId, {
      name: `Editable Skill ${unique}`,
      slug: unique,
      content: "# Editable\n\nWorkspace-scope skill content.\n",
    })) as { id?: string; slug?: string };
    expect(created.slug).toBe(unique);

    await testPage.goto("/office/workspace/skills");

    // Workspace skills render outside the collapsed System group, so
    // the row is visible without needing to expand anything.
    const row = testPage.locator("button", { hasText: `Editable Skill ${unique}` }).first();
    await expect(row).toBeVisible({ timeout: 10_000 });
    await row.click();

    const editor = testPage.getByTestId("skill-content-editor");
    await expect(editor).toBeVisible({ timeout: 10_000 });
    await expect(editor).toHaveAttribute("data-readonly", "false");
    await expect(testPage.getByTestId("skill-content-readonly")).toHaveCount(0);
  });
});
