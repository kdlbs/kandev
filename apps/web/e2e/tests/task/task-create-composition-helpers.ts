import { expect, type Page, type Request } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { expectTaskDescription } from "../../pages/task-description-editor";

export const COMPOSITION_CASES = [
  // Meta and keyCode0 isolate host admission from Linux ProseMirror's Ctrl+Enter
  // hard-break keymap and Enter fallback. Hook tests cover both modifiers/code13.
  { name: "isComposing", isComposing: true, keyCode: 0, ctrlKey: false, metaKey: true },
  { name: "legacy 229", isComposing: false, keyCode: 229, ctrlKey: false, metaKey: true },
] as const;

type FlowArgs = {
  page: Page;
  api: ApiClient;
  workspaceId: string;
  capture: PrAssetCapture;
  mobile?: boolean;
  signal?: (typeof COMPOSITION_CASES)[number];
};

export async function verifyTaskCreateComposition({
  page,
  api,
  workspaceId,
  capture,
  mobile = false,
  signal,
}: FlowArgs) {
  const title = `IME ${mobile ? "phone" : "desktop"} ${signal?.name ?? "touch"}`;
  const prompt = "Disposable composition regression draft";
  const requests: Request[] = [];
  const observeCreate = (request: Request) => {
    if (request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/tasks") {
      requests.push(request);
    }
  };
  page.on("request", observeCreate);
  let taskId: string | undefined;
  try {
    await page.goto("/");
    await expect(page.getByTestId("kanban-board")).toBeVisible();
    const opener = page.getByTestId(mobile ? "mobile-fab" : "create-task-button");
    await expect(opener).toHaveCount(1);
    if (mobile) await opener.tap();
    else await opener.click();
    const dialog = page.getByTestId("create-task-dialog");
    await expect(dialog).toHaveCount(1);
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("source-mode-scratch").click();
    await dialog.getByTestId("task-title-input").fill(title);
    const editor = dialog.getByTestId("task-description-input");
    await editor.fill(prompt);
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled();
    if (signal) {
      const result = await editor.evaluate((element, init) => {
        let propagated = false;
        const observe = () => {
          propagated = true;
        };
        document.addEventListener("keydown", observe);
        const event = new KeyboardEvent("keydown", {
          key: "Enter",
          code: "Enter",
          bubbles: true,
          cancelable: true,
          ...init,
        });
        try {
          element.dispatchEvent(event);
          return { defaultPrevented: event.defaultPrevented, propagated };
        } finally {
          document.removeEventListener("keydown", observe);
        }
      }, signal);
      await dwell(
        page,
        250,
        "negative-assertion",
        "Observe the armed create counter after synthetic composition confirmation",
      );
      expect(requests).toHaveLength(0);
      expect(result).toEqual({ defaultPrevented: false, propagated: true });
      await expect(dialog).toBeVisible();
      await expectTaskDescription(editor, prompt);
      await expect(dialog.getByTestId("task-title-input")).toHaveValue(title);
      await dialog.evaluate(async (element) => {
        await Promise.all(
          element
            .getAnimations({ subtree: true })
            .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      if (signal.isComposing) {
        await capture.screenshot(`composition-${mobile ? "phone" : "desktop"}`, {
          caption: "Composition confirmation leaves Create Task open with the entered text.",
        });
      }
    }
    const created = waitForHttp(page, "POST", /^\/api\/v1\/tasks$/);
    if (!signal) {
      await dialog.getByTestId("mobile-create-without-agent").tap();
    } else if (mobile) {
      // Android ProseMirror absorbs keyCode13 before the host hook. A synthetic
      // ordinary chord with no legacy code reaches the same host submit wiring.
      await editor.dispatchEvent("keydown", {
        key: "Enter",
        code: "Enter",
        ctrlKey: signal.ctrlKey,
        metaKey: signal.metaKey,
        bubbles: true,
        cancelable: true,
      });
    } else {
      await editor.press(signal.isComposing ? "Control+Enter" : "Meta+Enter");
    }
    const response = await created;
    expect(response.ok()).toBe(true);
    taskId = (await response.json()).id;
    expect(requests).toHaveLength(1);
    expect(requests[0].postDataJSON()).toMatchObject({ title, description: prompt });
    await expect(dialog).not.toBeVisible();
    await page.reload();
    const { tasks } = await api.listTasks(workspaceId);
    expect(tasks.filter((task) => task.title === title)).toEqual([
      expect.objectContaining({ id: taskId, title }),
    ]);
    await page.goto(`/t/${taskId}`);
    await expect(
      page.getByTestId(mobile ? "mobile-task-picker-trigger" : "task-topbar-title"),
    ).toContainText(title);
  } finally {
    page.removeListener("request", observeCreate);
    // Also clean an unexpected RED creation that completed before its assertion.
    const { tasks } = await api.listTasks(workspaceId);
    for (const task of tasks.filter((item) => item.title === title)) {
      await api.deleteTask(task.id);
    }
  }
}
