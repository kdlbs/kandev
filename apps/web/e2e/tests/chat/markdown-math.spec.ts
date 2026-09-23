import fs from "node:fs";
import path from "node:path";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForAgentMessage } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

function messageScript(content: string): string {
  const escaped = content.replaceAll("\\", "\\\\").replaceAll('"', '\\"').replaceAll("\n", "\\n");
  return `e2e:message("${escaped}")`;
}

async function openScriptedTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  content: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: messageScript(content),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForAgentMessage(apiClient, task.session_id, content.split("\n", 1)[0]);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

async function openMarkdownPreview(
  testPage: Page,
  session: SessionPage,
  fileName: string,
): Promise<void> {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 5_000 });
  await expect(session.files.getByText(fileName)).toBeVisible({ timeout: 10_000 });
  await session.files.getByText(fileName).click();

  await expect(testPage.locator(`.dv-default-tab:has-text('${fileName}')`)).toBeVisible({
    timeout: 10_000,
  });
  const editor = testPage.getByTestId("markdown-file-editor");
  await expect(editor).toBeVisible({ timeout: 10_000 });
  await expect(editor.getByTestId("markdown-mode-preview")).toHaveAttribute("aria-pressed", "true");
  await expect(editor.getByTestId("markdown-preview")).toBeVisible({ timeout: 10_000 });
}

test.describe("Markdown math", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  for (const theme of ["light", "dark"] as const) {
    test(`renders chat formulas, currency, and invalid TeX safely in ${theme} theme`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      await testPage.addInitScript((value) => localStorage.setItem("theme", value), theme);
      const session = await openScriptedTask(
        testPage,
        apiClient,
        seedData,
        `Markdown Math Chat ${theme}`,
        [
          "Energy: $E = mc^2$",
          "",
          "$$\\frac{a}{b}$$",
          "",
          "$$",
          "\\frac{a + b}{c + d}",
          "$$",
          "",
          "Cost: $100 and $200",
          "",
          "Before $\\notARealCommand$ after",
        ].join("\n"),
      );

      await expect(testPage.locator("html")).toHaveClass(new RegExp(`(^|\\s)${theme}(\\s|$)`));
      const chat = session.activeChat();
      await expect(chat.locator(".katex").first()).toBeVisible();
      await expect(chat.locator(".katex-display")).toHaveCount(2);
      await expect(chat.locator("math").first()).toBeVisible();
      await expect(chat.getByText("Cost: $100 and $200", { exact: true })).toBeVisible();
      const invalidFormula = chat.locator(".markdown-body").filter({ hasText: "Before" }).last();
      await expect(invalidFormula).toContainText("Before");
      await expect(invalidFormula).toContainText("after");
      await expect(chat.locator("script")).toHaveCount(0);

      const mathLayout = await chat
        .locator(".katex-display")
        .first()
        .evaluate((element) => {
          const katex = element.querySelector<HTMLElement>(".katex");
          const base = element.querySelector<HTMLElement>(".base");
          return {
            baseDisplay: base ? getComputedStyle(base).display : null,
            basePosition: base ? getComputedStyle(base).position : null,
            baseWhiteSpace: base ? getComputedStyle(base).whiteSpace : null,
            displayOverflow: getComputedStyle(element).overflowX,
            katexFont: katex ? getComputedStyle(katex).fontFamily : null,
          };
        });
      expect(mathLayout).toMatchObject({
        baseDisplay: "inline-block",
        basePosition: "relative",
        baseWhiteSpace: "nowrap",
        displayOverflow: "auto",
      });
      expect(mathLayout.katexFont).toContain("KaTeX_Main");

      const katexFontLoaded = await testPage.evaluate(async () => {
        await document.fonts.ready;
        return Array.from(document.fonts).some(
          (font) => font.family === "KaTeX_Main" && font.status === "loaded",
        );
      });
      expect(katexFontLoaded).toBe(true);
    });
  }

  test("renders formulas in a sanitized Markdown file preview", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = "math-preview.md";
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    fs.writeFileSync(
      path.join(repoDir, fileName),
      [
        "# Formula preview",
        "",
        "Energy: $E = mc^2$",
        "",
        "$$",
        "\\frac{a}{b}",
        "$$",
        "",
        '[unsafe](javascript:alert("xss"))',
        "",
        "<script>window.markdownMathXss = true</script>",
      ].join("\n"),
    );

    const session = await openScriptedTask(
      testPage,
      apiClient,
      seedData,
      "Markdown Math Preview",
      "Preview formula file",
    );
    await openMarkdownPreview(testPage, session, fileName);

    const preview = testPage.getByTestId("markdown-preview");
    await expect(preview.locator(".katex").first()).toBeVisible();
    await expect(preview.locator(".katex-display")).toBeVisible();
    await expect(preview.locator('a[href^="javascript:"]')).toHaveCount(0);
    await expect(preview.locator("script")).toHaveCount(0);
    expect(await testPage.evaluate(() => "markdownMathXss" in window)).toBe(false);
  });
});
