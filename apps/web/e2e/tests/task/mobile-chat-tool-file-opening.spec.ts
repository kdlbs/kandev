import { test, expect } from "../../fixtures/test-base";
import { withToolFileTask } from "../../helpers/tool-file-opening";

test.describe("Mobile tool file references", () => {
  test.setTimeout(90_000);

  // @covers AC-UI-TOOL-FILE-OPEN-001.2 AC-UI-TOOL-FILE-OPEN-001.4
  test("opens an extensionless read at its requested line", async ({
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
        await link.tap();
        const viewer = testPage.getByTestId("mobile-file-viewer-panel");
        await expect(viewer).toBeVisible();
        await expect(viewer.getByText("Dockerfile")).toBeVisible();
        await expect(viewer.locator(".cm-content")).toContainText("read fixture line 300");
        await expect(
          viewer.locator(".cm-lineNumbers .cm-gutterElement", { hasText: "300" }),
        ).toBeInViewport();
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
        await chat.getByText("Found 1 file").tap();
        const link = chat.locator("button[title]").filter({ hasText: "LICENSE" });
        await expect(link).toHaveAttribute("title", title);
        await link.tap();
        const viewer = testPage.getByTestId("mobile-file-viewer-panel");
        await expect(viewer).toBeVisible();
        await expect(viewer.getByText("LICENSE")).toBeVisible();
        await expect(viewer.locator(".cm-content")).toContainText("search fixture content");
      },
    );
  });
});
