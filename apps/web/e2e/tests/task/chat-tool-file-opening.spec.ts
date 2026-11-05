import { test, expect } from "../../fixtures/test-base";
import { withToolFileTask } from "../../helpers/tool-file-opening";

test.describe("Desktop tool file references", () => {
  test.setTimeout(90_000);

  // @covers AC-UI-TOOL-FILE-OPEN-001.1 AC-UI-TOOL-FILE-OPEN-001.2
  test("opens an extensionless read in the editor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await withToolFileTask(
      { testPage, apiClient, seedData, backend },
      "read",
      async (session, title) => {
        const link = session
          .activeChat()
          .locator("button[title]")
          .filter({ hasText: "Dockerfile" });
        await expect(link).toHaveAttribute("title", title);
        await expect(session.activeChat().getByText(":300-300")).toBeVisible();
        await link.click();
        const editor = testPage.locator(".monaco-editor:visible").first();
        await expect(editor.locator(".view-lines")).toContainText(/read\s+fixture\s+line\s+\d+/);
        await expect(testPage.locator(".dv-tab", { hasText: "Dockerfile" })).toBeVisible();
      },
    );
  });

  // @covers AC-UI-TOOL-FILE-OPEN-001.1 AC-UI-TOOL-FILE-OPEN-001.4
  test("opens an extensionless expanded search result", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await withToolFileTask(
      { testPage, apiClient, seedData, backend },
      "search",
      async (session, title) => {
        const chat = session.activeChat();
        await chat.getByText("Found 1 file").click();
        const link = chat.locator("button[title]").filter({ hasText: "LICENSE" });
        await expect(link).toHaveAttribute("title", title);
        await link.click();
        const editor = testPage.locator(".monaco-editor:visible").first();
        await expect(editor.locator(".view-lines")).toContainText("search fixture content");
        await expect(testPage.locator(".dv-tab", { hasText: "LICENSE" })).toBeVisible();
      },
    );
  });
});
