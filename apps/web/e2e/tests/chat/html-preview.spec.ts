import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const SEEDED_HTML = "<!doctype html><html><body><p>Saved source</p></body></html>";
const BLOCKED_NAVIGATION_URL = "https://html-preview-navigation.invalid/leak";
const UNSAVED_HTML = `<!doctype html>
<html>
  <head>
    <meta http-equiv="refresh" content="0;url=${BLOCKED_NAVIGATION_URL}">
    <style>
      @import/**/ "${BLOCKED_NAVIGATION_URL}/theme.css";
      @im\\70 ort "${BLOCKED_NAVIGATION_URL}/escaped-theme.css";
      body {
        background: url("${BLOCKED_NAVIGATION_URL}/background.png");
        background-image: image-set("${BLOCKED_NAVIGATION_URL}/image-set.png" 1x);
      }
    </style>
  </head>
  <body>
    <h1>Unsaved HTML preview</h1>
    <button id="increment">Increment</button>
    <output id="value">0</output>
    <p><a id="relative-link" href="relative-target.html">Relative link</a></p>
    <p><a id="remote-link" href="${BLOCKED_NAVIGATION_URL}/link">Remote link</a></p>
    <form id="form" action="${BLOCKED_NAVIGATION_URL}/submit"><button type="submit">Submit</button></form>
    <img id="remote-image">
    <script>
      const output = document.getElementById("value");
      let count = 0;
      document.body.dataset.inlineScript = "ran";
      document.getElementById("increment").addEventListener("click", () => {
        count += 1;
        output.textContent = String(count);
      });
      document.getElementById("remote-image").setAttribute("src", "${BLOCKED_NAVIGATION_URL}/image.png");
      document.getElementById("form").setAttribute("action", "${BLOCKED_NAVIGATION_URL}/submit");
      location.replace("${BLOCKED_NAVIGATION_URL}/location");
      history.pushState({}, "", "${BLOCKED_NAVIGATION_URL}/history");
      try { window.open("${BLOCKED_NAVIGATION_URL}/popup"); } catch { document.body.dataset.popup = "blocked"; }
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

  test("renders the current buffer in an isolated runtime surface", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const blockedRequests: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().startsWith(BLOCKED_NAVIGATION_URL)) blockedRequests.push(request.url());
    });

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

    await testPage.getByTestId("html-preview-toggle").first().click();
    const preview = testPage.getByTestId("html-preview");
    await expect(preview).toBeVisible({ timeout: 10_000 });
    await expect(preview.getByTestId("html-preview-surface")).toBeVisible();
    await expect(preview.locator("h1")).toHaveText("Unsaved HTML preview");
    await expect(preview.locator('[data-inline-script="ran"]')).toBeVisible();
    await expect(preview.locator('[data-popup="blocked"]')).toBeVisible();
    expect(await testPage.evaluate(() => "inlineScript" in document.body.dataset)).toBe(false);

    const increment = preview.locator("#increment");
    await expect(preview.locator("#value")).toHaveText("0");
    await increment.click();
    await expect(preview.locator("#value")).toHaveText("1");

    await expect(preview.locator("#relative-link")).not.toHaveAttribute("href");
    await expect(preview.locator("#remote-link")).not.toHaveAttribute("href");
    await expect(preview.locator("#form")).not.toHaveAttribute("action");
    await expect(preview.locator("#remote-image")).not.toHaveAttribute("src");
    await preview.locator("#relative-link").click();
    await preview.locator("#remote-link").click();
    await expect(preview.locator("h1")).toHaveText("Unsaved HTML preview");
    expect(blockedRequests).toEqual([]);

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
    await expect(previewAfter.getByText("Saved source")).toBeVisible();
    await previewAfter.getByRole("button", { name: "Show code" }).click();
    await expect(testPage.locator(".monaco-editor:visible").first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
