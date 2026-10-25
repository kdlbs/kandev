import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { openImageQuestions, previewName } from "./question-image-helpers";

const wide = { id: "wide_preview", path: previewName, alt: "Proposed layout" };
const compact = {
  id: "compact_preview",
  path: "compact-preview.png",
  alt: "Compact layout preview",
};

function choiceScript(mixed: boolean) {
  const questions = [
    {
      id: "visual-choice",
      prompt: "Which layout do you prefer?",
      ...(mixed ? { images: [wide] } : {}),
      options: [
        {
          option_id: "wide",
          label: "Wide layout",
          description: "Cards side by side",
          images: [wide],
        },
        {
          option_id: "compact",
          label: "Compact layout",
          description: "Cards in one column",
          images: [compact],
        },
        {
          option_id: "missing",
          label: "Missing alternative",
          description: "Request another layout",
          images: [{ id: "missing", path: "missing.png", alt: "Missing alternative preview" }],
        },
      ],
    },
  ];
  return `e2e:mcp:kandev:ask_user_question_kandev(${JSON.stringify({ questions })})\ne2e:message("Image questions complete.")`;
}

async function seedChoiceLayouts(page: Page, repositoryPath: string) {
  const previews = await page.evaluate(() =>
    [false, true].map((stacked) => {
      const canvas = document.createElement("canvas");
      canvas.width = 600;
      canvas.height = 320;
      const ctx = canvas.getContext("2d")!;
      ctx.fillStyle = "#f4f4f5";
      ctx.fillRect(0, 0, 600, 320);
      ctx.fillStyle = "#4f46e5";
      ctx.fillRect(0, 0, 600, 64);
      ctx.fillStyle = "#ffffff";
      ctx.font = "24px sans-serif";
      ctx.fillText(stacked ? "Compact layout" : "Wide layout", 24, 42);
      for (let i = 0; i < 2; i++) {
        ctx.fillStyle = "#ffffff";
        const x = stacked ? 24 : 24 + i * 284;
        const y = stacked ? 88 + i * 108 : 88;
        const width = stacked ? 552 : 268;
        ctx.fillRect(x, y, width, stacked ? 88 : 208);
        ctx.fillStyle = "#6366f1";
        ctx.fillRect(x + 16, y + 16, width - 32, 12);
        ctx.fillStyle = "#cbd5e1";
        ctx.fillRect(x + 16, y + 44, width - 64, 8);
      }
      return canvas.toDataURL("image/png").split(",")[1];
    }),
  );
  [previewName, compact.path].forEach((name, index) =>
    fs.writeFileSync(path.join(repositoryPath, name), Buffer.from(previews[index], "base64")),
  );
}

export async function exerciseChoiceImageQuestion(
  page: Page,
  api: ApiClient,
  seed: Parameters<typeof openImageQuestions>[2] & { repositoryPath: string },
  mobile: boolean,
  capture: PrAssetCapture,
) {
  await seedChoiceLayouts(page, seed.repositoryPath);
  const { session, task } = await openImageQuestions(
    page,
    api,
    seed,
    undefined,
    choiceScript(false),
  );
  await page.reload();
  const overlay = session.clarificationOverlay();
  const group = overlay.getByRole("group", { name: "Wide layout", exact: true });
  const inspect = group.getByRole("button", { name: /Proposed layout/ });
  await expect(inspect).toBeVisible({ timeout: 30_000 });
  await capture.screenshot(`${mobile ? "phone" : "desktop"}-choice-images`, {
    caption: "Each choice has separate selection and image inspection controls",
  });
  const draft = overlay.getByTestId("clarification-input");
  await draft.fill("Keep the spacing.");
  if (!mobile) {
    for (const width of [767, 768, 1280]) {
      await page.setViewportSize({ width, height: 850 });
      await expect(draft).toHaveValue("Keep the spacing.");
    }
  }
  await inspect.scrollIntoViewIfNeeded();
  if (mobile) {
    const target = await inspect.boundingBox();
    expect(target!.height).toBeGreaterThanOrEqual(44);
    expect(target!.width).toBeGreaterThanOrEqual(44);
    await inspect.tap();
  } else {
    await inspect.focus();
    await page.keyboard.press("Enter");
  }
  const viewer = page.getByRole("dialog", { name: "Proposed layout" });
  await expect(viewer).toBeVisible();
  await viewer.press("2");
  await viewer.press("ArrowRight");
  if (mobile) await viewer.getByRole("button", { name: "Close", exact: true }).tap();
  else await viewer.press("Escape");
  await expect(inspect).toBeFocused();
  await expect(draft).toHaveValue("Keep the spacing.");
  await expect(
    overlay.locator('[data-testid="clarification-option"][data-selected="true"]'),
  ).toHaveCount(0);
  await expect(overlay).toContainText("Which layout do you prefer?");
  const missing = overlay.getByRole("group", { name: "Missing alternative", exact: true });
  await expect(missing).toContainText("Image file not found.");
  await expect(missing.getByTestId("clarification-option")).toBeEnabled();
  await session.clarificationOption("Compact layout").click();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  const { messages } = await api.listSessionMessages(task.session_id!);
  const question = messages.find((message) => message.type === "clarification_request")!;
  expect(question.metadata?.response).toMatchObject({ selected_options: ["compact"] });
  await page.reload();
  const history = page.getByTestId("clarification-request-message");
  await expect(
    history.getByRole("img", { name: "Compact layout preview", exact: true }),
  ).toHaveCount(0);
  const historyChoice = history.getByRole("group", { name: "Compact layout", exact: true });
  await historyChoice.getByRole("button", { name: "Show images", exact: true }).click();
  await expect(
    historyChoice.getByRole("img", { name: "Compact layout preview", exact: true }),
  ).toBeVisible({ timeout: 30_000 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);

  const mixed = await openImageQuestions(page, api, seed, undefined, choiceScript(true));
  await expect(
    mixed.session.clarificationOverlay().getByRole("img", { name: "Proposed layout", exact: true }),
  ).toHaveCount(2);
  await mixed.session.clarificationOption("Missing alternative").click();
  await expect(mixed.session.idleInput()).toBeVisible({ timeout: 30_000 });
  const mixedMessages = await api.listSessionMessages(mixed.task.session_id!);
  expect(
    mixedMessages.messages.find((message) => message.type === "clarification_request")?.metadata
      ?.response,
  ).toMatchObject({ selected_options: ["missing"] });
}
