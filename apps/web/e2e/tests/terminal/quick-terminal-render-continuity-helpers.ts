import { expect, type Page } from "@playwright/test";
import { dwell } from "../../helpers/causal-waits";
import { readTerminalHostBuffer } from "./terminal-test-helpers";

const HOST_SHELL_START_PATH = "/api/v1/host-shell/start";
const QUICK_TERMINAL_PATH = "/api/v1/quick-terminal-tabs";
const TERMINAL_STREAM_PATH = "/api/v1/agent-login/sessions/";
const UNRELATED_SESSION_ID = "e2e-quick-terminal-render-continuity";

function normalizeTerminalText(text: string): string {
  return text.replace(/\s+/g, "");
}

type E2EStoreState = {
  quickChat: {
    terminalTabs: Array<{ tabId: string; sessionId: string | null }>;
  };
  taskSessions: {
    items: Record<string, Record<string, unknown>>;
  };
};

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => E2EStoreState;
    setState: (partial: Partial<E2EStoreState>) => void;
  };
};

export type QuickTerminalRenderCapture = {
  hostShellStarts: string[];
  descriptorWrites: string[];
  terminalStreams: string[];
  pendingDescriptorWriteCount: () => number;
  snapshot: () => {
    hostShellStarts: number;
    descriptorWrites: number;
    terminalStreams: number;
  };
};

export function captureQuickTerminalRenderRequests(page: Page): QuickTerminalRenderCapture {
  const hostShellStarts: string[] = [];
  const descriptorWrites: string[] = [];
  const terminalStreams: string[] = [];
  const pendingDescriptorWrites = new Set<import("@playwright/test").Request>();

  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname === HOST_SHELL_START_PATH && request.method() === "POST") {
      hostShellStarts.push(request.url());
    }
    if (url.pathname.startsWith(QUICK_TERMINAL_PATH) && request.method() !== "GET") {
      descriptorWrites.push(`${request.method()} ${url.pathname}`);
      pendingDescriptorWrites.add(request);
    }
  });
  page.on("requestfinished", (request) => pendingDescriptorWrites.delete(request));
  page.on("requestfailed", (request) => pendingDescriptorWrites.delete(request));
  page.on("websocket", (socket) => {
    const url = new URL(socket.url());
    if (!url.pathname.startsWith(TERMINAL_STREAM_PATH) || !url.pathname.endsWith("/stream")) return;
    terminalStreams.push(url.pathname);
  });

  return {
    hostShellStarts,
    descriptorWrites,
    terminalStreams,
    pendingDescriptorWriteCount: () => pendingDescriptorWrites.size,
    snapshot: () => ({
      hostShellStarts: hostShellStarts.length,
      descriptorWrites: descriptorWrites.length,
      terminalStreams: terminalStreams.length,
    }),
  };
}

export async function settleQuickTerminalAnimationsForCapture(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const animations = document
      .getAnimations()
      .filter(
        (animation) =>
          animation.playState === "running" &&
          Number.isFinite(animation.effect?.getComputedTiming().endTime),
      );
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });
}

export async function prepareQuickTerminalForCapture(
  page: Page,
  input: { focus: () => Promise<void>; type: (text: string) => Promise<void> },
): Promise<void> {
  await input.focus();
  await input.type("cd /tmp && clear && printf 'KQT_RENDER_CONTINUITY\\n'\n");
  await expect
    .poll(() => readTerminalHostBuffer(page.getByTestId("quick-terminal-terminal")))
    .toContain("KQT_RENDER_CONTINUITY");
}

export async function exerciseQuickTerminalRenderContinuity(
  page: Page,
  capture: QuickTerminalRenderCapture,
  input: { focus: () => Promise<void>; type: (text: string) => Promise<void> },
  openTerminal: () => Promise<void>,
): Promise<{ sessionId: string }> {
  const startResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === HOST_SHELL_START_PATH &&
      response.request().method() === "POST",
  );
  await openTerminal();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible();
  const terminal = dialog.getByTestId("quick-terminal-terminal");
  await expect(terminal).toBeVisible();

  const response = await startResponse;
  const startedSession = (await response.json()) as { session_id?: string };
  if (!startedSession.session_id) throw new Error("Host-shell start did not return a session ID");
  const sessionId = startedSession.session_id;

  await expect
    .poll(() => readTerminalHostBuffer(terminal), {
      timeout: 30_000,
      message: "Quick Terminal should render its host-shell prompt",
    })
    .not.toBe("");
  await expect
    .poll(() => capture.terminalStreams.filter((path) => path.includes(sessionId)).length)
    .toBe(1);
  await expect.poll(() => capture.pendingDescriptorWriteCount()).toBe(0);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );

  const baseline = capture.snapshot();
  expect(baseline.hostShellStarts).toBe(1);
  expect(baseline.terminalStreams).toBe(1);

  const originalXterm = await terminal.locator(".xterm").elementHandle();
  if (!originalXterm) throw new Error("Quick Terminal did not create an xterm node");
  const textarea = dialog.locator(".xterm-helper-textarea");
  await input.focus();
  await input.type("printf 'KQT_%s%s%s%s%s\\n' 'RENDER_'");
  await expect(textarea).toBeFocused();

  const fragments = [" 'CON'", " 'T'", " 'INU'", " 'ITY'", "\n"];
  const marker = "KQT_RENDER_CONTINUITY";
  for (let index = 0; index < fragments.length; index += 1) {
    await publishUnrelatedTaskSessionUpdate(page, sessionId, index + 1);
    await page.evaluate(
      () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
    );
    expect(
      await originalXterm.evaluate(
        (node) => node === document.querySelector('[data-testid="quick-terminal-terminal"] .xterm'),
      ),
    ).toBe(true);
    await expect(textarea).toBeFocused();
    if (index === fragments.length - 2) {
      expect(normalizeTerminalText(await readTerminalHostBuffer(terminal))).not.toContain(marker);
    }
    await input.type(fragments[index]!);
  }

  await expect
    .poll(async () => normalizeTerminalText(await readTerminalHostBuffer(terminal)), {
      timeout: 15_000,
      message: "Quick Terminal should execute the command entered across unrelated renders",
    })
    .toContain(marker);

  const terminalTabId = await dialog
    .getByTestId("quick-terminal-tab-panel")
    .getAttribute("data-terminal-tab-id");
  if (!terminalTabId) throw new Error("Quick Terminal panel is missing its tab identity");
  await expect.poll(() => readQuickTerminalSessionId(page, terminalTabId)).toBe(sessionId);
  await expect(textarea).toBeFocused();
  await dwell(
    400,
    "negative-assertion",
    "confirming unrelated task-session updates do not start another shell, stream, or descriptor write",
  );
  await expect.poll(() => capture.pendingDescriptorWriteCount()).toBe(0);

  const finalCounts = capture.snapshot();
  expect(finalCounts.hostShellStarts).toBe(baseline.hostShellStarts);
  expect(finalCounts.terminalStreams).toBe(baseline.terminalStreams);
  expect(finalCounts.descriptorWrites).toBe(baseline.descriptorWrites);
  return { sessionId };
}

async function publishUnrelatedTaskSessionUpdate(
  page: Page,
  terminalSessionId: string,
  revision: number,
): Promise<void> {
  await page.evaluate(
    ({ terminalSessionId, revision, unrelatedSessionId }) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      const state = store.getState();
      store.setState({
        taskSessions: {
          ...state.taskSessions,
          items: {
            ...state.taskSessions.items,
            [unrelatedSessionId]: {
              ...state.taskSessions.items[unrelatedSessionId],
              session_id: unrelatedSessionId,
              task_id: "unrelated-quick-terminal-render-update",
              status: "WAITING_FOR_INPUT",
              revision,
              related_terminal_session_id: terminalSessionId,
            },
          },
        },
      });
    },
    { terminalSessionId, revision, unrelatedSessionId: UNRELATED_SESSION_ID },
  );
}

async function readQuickTerminalSessionId(page: Page, tabId: string): Promise<string | null> {
  return page.evaluate((currentTabId) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    return (
      store.getState().quickChat.terminalTabs.find((tab) => tab.tabId === currentTabId)
        ?.sessionId ?? null
    );
  }, tabId);
}
