import { chromium, expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { capturePresentation, threadBoxes } from "./threads-presentation-helpers";

export async function attachQuestionRecord(info: TestInfo, name: string, record: unknown) {
  const path = info.outputPath(`${name}.json`);
  await writeFile(path, JSON.stringify(record, null, 2));
  await info.attach(name, { path, contentType: "application/json" });
}

export async function seedLongThreadQuestion(api: ApiClient, seed: SeedData) {
  const tasks = [];
  for (const [title, scenario] of [
    ["A long required question", "clarification-multi"],
    ["B neighboring question", "clarification"],
  ]) {
    const task = await api.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
      description: `/e2e:${scenario}`,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
    });
    if (!task.session_id) throw new Error("Required question has no session");
    await waitForSessionState(api, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: `${title} must block on its real clarification request`,
      timeout: 60_000,
    });
    tasks.push(task);
  }
  return { task: tasks[0], sibling: tasks[1] };
}

/** Native tab zoom, with neither viewport/device-scale emulation nor CSS zoom. */
export async function withNativeThreadZoom(
  baseURL: string,
  zoom: number,
  info: TestInfo,
  run: (page: Page) => Promise<void>,
) {
  const root = await mkdtemp(join(tmpdir(), "threads-question-zoom-"));
  const extension = join(root, "extension");
  await mkdir(extension);
  await writeFile(
    join(extension, "manifest.json"),
    JSON.stringify({
      manifest_version: 3,
      name: "Threads question native zoom test",
      version: "1.0",
      permissions: ["tabs"],
      background: { service_worker: "background.js" },
    }),
  );
  await writeFile(
    join(extension, "background.js"),
    "chrome.runtime.onInstalled.addListener(() => {});",
  );
  let context;
  try {
    context = await chromium.launchPersistentContext(join(root, "profile"), {
      channel: "chromium",
      headless: true,
      baseURL,
      viewport: null,
      deviceScaleFactor: undefined,
      args: [
        `--disable-extensions-except=${extension}`,
        `--load-extension=${extension}`,
        "--window-size=1366,855",
      ],
    });
    await context.addInitScript(() => {
      localStorage.setItem("kandev.onboarding.completed", "true");
    });
    const page = context.pages()[0];
    const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
    await page.goto(baseURL);
    const initialScale = await page.evaluate(() => devicePixelRatio);
    const actualZoom = await worker.evaluate(
      async ({ baseURL, zoom }) => {
        const { tabs } = (
          globalThis as unknown as {
            chrome: {
              tabs: {
                query: (query: { url: string }) => Promise<{ id: number }[]>;
                setZoom: (id: number, factor: number) => Promise<void>;
                getZoom: (id: number) => Promise<number>;
              };
            };
          }
        ).chrome;
        const [tab] = await tabs.query({ url: `${new URL(baseURL).origin}/*` });
        await tabs.setZoom(tab.id, zoom);
        return tabs.getZoom(tab.id);
      },
      { baseURL, zoom },
    );
    expect(actualZoom).toBe(zoom);
    await expect
      .poll(() => page.evaluate(() => devicePixelRatio))
      .toBeCloseTo(initialScale * zoom, 5);
    await attachQuestionRecord(info, "native-browser-zoom", {
      browser: context.browser()!.version(),
      zoom: actualZoom,
      ...(await page.evaluate(() => ({
        width: innerWidth,
        height: innerHeight,
        devicePixelRatio,
        visualScale: visualViewport?.scale,
      }))),
    });
    try {
      await run(page);
    } finally {
      await capturePresentation(page, info, "native-question-final-state");
    }
  } finally {
    try {
      await context?.close();
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  }
}

/** Intersect every clipping ancestor, including the inner question and outer footer. */
export function questionTargetGeometry(target: Locator) {
  return target.evaluate((element) => {
    const box = element.getBoundingClientRect();
    const clip = { top: 0, bottom: innerHeight, left: 0, right: innerWidth };
    for (let parent = element.parentElement; parent; parent = parent.parentElement) {
      const style = getComputedStyle(parent);
      const bounds = parent.getBoundingClientRect();
      if (/(auto|scroll|hidden|clip)/.test(style.overflowY)) {
        clip.top = Math.max(clip.top, bounds.top);
        clip.bottom = Math.min(clip.bottom, bounds.bottom);
      }
      if (/(auto|scroll|hidden|clip)/.test(style.overflowX)) {
        clip.left = Math.max(clip.left, bounds.left);
        clip.right = Math.min(clip.right, bounds.right);
      }
    }
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    const hit = document.elementFromPoint(x, y);
    const footer = element.closest('[data-testid="thread-footer-allocation"]')!;
    const inner = element.closest('[data-testid="clarification-scroll-region"]')!;
    return {
      x,
      y,
      width: box.width,
      height: box.height,
      top: box.top,
      bottom: box.bottom,
      clip,
      contained:
        box.top >= clip.top - 1 &&
        box.bottom <= clip.bottom + 1 &&
        box.left >= clip.left - 1 &&
        box.right <= clip.right + 1,
      hit: !!hit && element.contains(hit),
      footerScrollTop: footer.scrollTop,
      innerScrollTop: inner.scrollTop,
    };
  });
}

type QuestionInput = "wheel" | "touch" | "keyboard";

async function scrollQuestion(page: Page, tile: Locator, direction: number, touch: boolean) {
  const area = await tile.getByTestId("clarification-scroll-region").evaluate((inner) => {
    const box = inner.getBoundingClientRect();
    const footer = inner
      .closest('[data-testid="thread-footer-allocation"]')!
      .getBoundingClientRect();
    const top = Math.max(box.top, footer.top, 0);
    const bottom = Math.min(box.bottom, footer.bottom, innerHeight);
    return { x: box.x + box.width / 2, y: (top + bottom) / 2, height: bottom - top };
  });
  expect(area.height).toBeGreaterThan(40);
  if (!touch) {
    await page.mouse.move(area.x, area.y);
    await page.mouse.wheel(0, direction * 40);
    return;
  }
  const client = await page.context().newCDPSession(page);
  try {
    await client.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: area.x, y: area.y + direction * 20 }],
    });
    for (let step = 1; step <= 8; step++) {
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x: area.x, y: area.y + direction * (20 - step * 5) }],
      });
    }
  } finally {
    try {
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    } finally {
      await client.detach();
    }
  }
}

export async function reachQuestionTarget(
  page: Page,
  tile: Locator,
  target: Locator,
  input: QuestionInput,
  reverseTab = false,
) {
  await waitForFiniteAnimations(tile);
  if (input === "keyboard") {
    await expect
      .poll(
        async () => {
          if (await target.evaluate((el) => el === document.activeElement)) return true;
          await page.keyboard.press(reverseTab ? "Shift+Tab" : "Tab");
          return false;
        },
        { intervals: [50], timeout: 10_000, message: "Native tab navigation reaches the action" },
      )
      .toBe(true);
  } else {
    await expect
      .poll(
        async () => {
          const geometry = await questionTargetGeometry(target);
          if (!geometry.contained || !geometry.hit) {
            await scrollQuestion(
              page,
              tile,
              geometry.bottom > geometry.clip.bottom + 1 ? 1 : -1,
              input === "touch",
            );
          }
          return geometry;
        },
        {
          intervals: [100],
          timeout: 10_000,
          message: `${input} reaches the complete question action through the footer`,
        },
      )
      .toMatchObject({ contained: true, hit: true });
  }
  const geometry = await questionTargetGeometry(target);
  expect(geometry).toMatchObject({ contained: true, hit: true });
  return geometry;
}

export async function answerLongThreadQuestion(
  page: Page,
  tile: Locator,
  input: QuestionInput,
  info: TestInfo,
  beforeSubmit?: () => Promise<void>,
) {
  const records = [];
  const capture = new PrAssetCapture(page, info.file, { captureKey: info.title });
  const option = (label: string) =>
    tile.getByTestId("clarification-option").filter({
      has: page.getByTestId("clarification-option-label").getByText(label, { exact: true }),
    });
  const activate = async (target: Locator, key = "Enter", reverse = false) => {
    const geometry = await reachQuestionTarget(page, tile, target, input, reverse);
    records.push({ target: await target.innerText(), ...geometry });
    if ((await target.getAttribute("data-testid")) === "clarification-submit") {
      await beforeSubmit?.();
      await capture.screenshot("submit-reachable");
    }
    if (input === "keyboard") await page.keyboard.press(key);
    else if (input === "touch") await page.touchscreen.tap(geometry.x, geometry.y);
    else await page.mouse.click(geometry.x, geometry.y);
  };
  try {
    // Prove the last option is reachable before any activation can focus/scroll it.
    await reachQuestionTarget(page, tile, option("SQLite"), input);
    await capture.screenshot("last-option-reachable");
    await activate(tile.getByTestId("clarification-next"));
    await expect(tile.getByTestId("clarification-step").nth(1)).toHaveAttribute(
      "data-active",
      "true",
    );
    await expect(tile.getByTestId("clarification-step").nth(0)).toHaveAttribute(
      "data-answered",
      "false",
    );
    await activate(tile.getByTestId("clarification-prev"), "Enter", true);
    await expect(tile.getByTestId("clarification-step").nth(0)).toHaveAttribute(
      "data-active",
      "true",
    );
    for (const [index, label] of ["SQLite", "Rust", "Bare metal"].entries()) {
      await activate(option(label), index === 1 ? "Enter" : "Space");
      await expect(tile.getByTestId("clarification-step").nth(index)).toHaveAttribute(
        "data-answered",
        "true",
      );
    }
    await activate(tile.getByTestId("clarification-submit"), "Enter", true);
  } finally {
    await attachQuestionRecord(info, `${input}-question-hit-targets`, records);
    capture.flush();
  }
  if (input === "touch") {
    for (const record of records.filter((record) =>
      /SQLite|Rust|Bare metal|Submit/.test(record.target),
    )) {
      expect(record.height).toBeGreaterThanOrEqual(44);
      expect(record.width).toBeGreaterThanOrEqual(44);
    }
  }
}

export async function expectThreadQuestionSubmitted(
  api: ApiClient,
  sessionId: string,
  tile: Locator,
) {
  await waitForAgentMessage(api, sessionId, "You answered:");
  const { messages } = await api.listSessionMessages(sessionId);
  const receipt = messages
    .filter((message) => message.author_type === "agent")
    .map((message) => message.content)
    .join("\n");
  expect(receipt).toMatch(/"db":\s*{\s*"selected_option":\s*"q1_opt3"/);
  expect(receipt).toMatch(/"language":\s*{\s*"selected_option":\s*"q2_opt3"/);
  expect(receipt).toMatch(/"deploy":\s*{\s*"selected_option":\s*"q3_opt2"/);
  await expect(tile.getByTestId("clarification-overlay-container")).toHaveCount(0);
  await expect(tile.locator(".chat-message-list")).toContainText("You answered:");
}

export async function threadDeckGeometry(page: Page) {
  const board = page.getByTestId("threads-board");
  return {
    boxes: await threadBoxes(board),
    scroll: await board.evaluate((el) => ({ left: el.scrollLeft, top: el.scrollTop })),
  };
}
