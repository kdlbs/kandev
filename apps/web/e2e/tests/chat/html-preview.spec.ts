import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const SEEDED_HTML = "<!doctype html><html><body><p>Saved source</p></body></html>";
const UNSAVED_HTML = `<!doctype html>
<html>
  <body>
    <h1>Unsaved HTML preview</h1>
    <script>
      document.body.dataset.inlineScript = "ran";
      document.body.insertAdjacentHTML("beforeend", "<p id='script-result'>Inline script ran</p>");
      try {
        parent.document.body.dataset.previewEscaped = "yes";
      } catch {
        // The preview must not be able to reach the parent document.
      }
    </script>
  </body>
</html>`;

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("HTML preview", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("renders the current buffer in an isolated preview frame", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    fs.writeFileSync(path.join(repoDir, "preview.html"), SEEDED_HTML);

    const session = await seedTaskWithSession(testPage, apiClient, seedData, "HTML Preview Test");

    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });
    await expect(session.files.getByText("preview.html")).toBeVisible({ timeout: 10_000 });
    await session.files.getByText("preview.html").click();
    await expect(testPage.locator(".dv-default-tab:has-text('preview.html')")).toBeVisible({
      timeout: 10_000,
    });

    const editor = testPage.locator(".monaco-editor:visible").first();
    await expect(editor).toBeVisible({ timeout: 15_000 });
    const editorContent = editor.locator(".view-lines");
    await editorContent.click();
    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.insertText(UNSAVED_HTML);
    await expect(editorContent).toContainText("Unsaved HTML preview", {
      timeout: 10_000,
    });

    const previewToggle = testPage.getByTestId("html-preview-toggle").first();
    await expect(previewToggle).toBeVisible({ timeout: 10_000 });
    await previewToggle.click();

    const preview = testPage.getByTestId("html-preview");
    await expect(preview).toBeVisible({ timeout: 10_000 });
    const frame = preview.getByTestId("html-preview-frame");
    await expect(frame).toHaveAttribute("sandbox", "allow-scripts");
    await expect(frame).not.toHaveAttribute("allow-same-origin");
    await expect(frame).toHaveAttribute("referrerpolicy", "no-referrer");
    await expect(frame).toHaveAttribute("srcdoc", /default-src 'none'/);

    const frameDocument = frame.contentFrame();
    await expect(
      frameDocument.getByRole("heading", { name: "Unsaved HTML preview" }),
    ).toBeVisible();
    await expect(frameDocument.getByText("Inline script ran")).toBeVisible();
    expect(await testPage.evaluate(() => document.body.dataset.previewEscaped ?? null)).toBeNull();

    await prCapture.screenshot("html-preview-desktop", {
      caption: "Unsaved HTML rendered in the desktop preview surface",
    });

    await preview.getByRole("button", { name: "Show code" }).click();
    await expect(testPage.getByTestId("html-preview")).toHaveCount(0);
    await expect(editor).toBeVisible();

    await testPage.getByTestId("html-preview-toggle").first().click();
    await expect(testPage.getByTestId("html-preview")).toBeVisible({ timeout: 10_000 });
    await testPage.reload();

    const sessionAfter = new SessionPage(testPage);
    await expect(sessionAfter.sidebar).toBeVisible({ timeout: 30_000 });
    const editorTabAfter = testPage.locator(".dv-default-tab:has-text('preview.html')");
    await expect(editorTabAfter).toBeVisible({ timeout: 15_000 });
    await editorTabAfter.click();
    const previewAfter = testPage.getByTestId("html-preview");
    await expect(previewAfter).toBeVisible({ timeout: 10_000 });
    await expect(
      previewAfter.getByTestId("html-preview-frame").contentFrame().getByText("Saved source"),
    ).toBeVisible();
    await previewAfter.getByRole("button", { name: "Show code" }).click();
    await expect(testPage.locator(".monaco-editor:visible").first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
