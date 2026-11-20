import { expect, test } from "../../fixtures/test-base";

test("searches saved prompt names and content on desktop", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  const ids: string[] = [];
  try {
    const first = await apiClient.createPrompt(
      "search-example-alpha",
      "First line\nHidden needle body",
    );
    ids.push(first.id);
    const second = await apiClient.createPrompt("search-example-beta", "Other content");
    ids.push(second.id);
    await testPage.goto("/settings/prompts");
    const search = testPage.getByRole("searchbox", { name: "Search..." });
    const rows = testPage.getByTestId("prompt-list-item");
    await expect(rows.filter({ hasText: "search-example-alpha" })).toBeVisible();
    await search.click();
    await testPage.keyboard.type("  NEEDLE  ");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute("data-prompt-name", "search-example-alpha");
    await prCapture.screenshot("saved-prompt-search-desktop");
    await search.fill("search-example-beta");
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute("data-prompt-name", "search-example-beta");
    await search.fill("missing-search-example");
    await expect(testPage.getByText("No results.")).toBeVisible();
    await expect(search).toBeEditable();
    await search.fill("");
    await expect(rows.filter({ hasText: "search-example-alpha" })).toBeVisible();
    await expect(rows.filter({ hasText: "search-example-beta" })).toBeVisible();
  } finally {
    for (const id of ids) await apiClient.deletePrompt(id);
  }
});
