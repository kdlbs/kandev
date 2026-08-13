import type { Page, Response } from "@playwright/test";

/**
 * Causal waits — wait for the backend event that *causes* a UI change instead
 * of polling the UI for its shadow.
 *
 * The suite's dominant flake shape is a test that asserts on a rendered
 * consequence with a hand-picked budget:
 *
 * ```ts
 * await expect(button).toBeEnabled({ timeout: 30_000 }); // why 30s? nobody knows
 * ```
 *
 * The element rendered fine; it just never left its pending state inside the
 * budget, because the backend round trip that would flip it was late. Raising
 * the number buys time, it does not remove the race: under CI load the next
 * budget expires too.
 *
 * The fix is to name the cause. A pending UI state in this app is waiting on
 * exactly one of three observable things, and each has a primitive here:
 *
 * | The UI is waiting on…             | Use                               |
 * | --------------------------------- | --------------------------------- |
 * | an HTTP request/response          | {@link waitForHttp}               |
 * | a server-pushed WS notification   | {@link watchWs}`.waitForEvent`    |
 * | a WS request/response round trip  | {@link watchWs}`.waitForResponse` |
 *
 * A fourth case — "the backend has reached state X" — needs no primitive:
 * `expect.poll(() => apiClient.getX(id))` against `helpers/api-client.ts`
 * already reads the backend directly rather than through the DOM.
 *
 * ## The one rule: arm before you act
 *
 * Every wait here is *armed* before the action that triggers it and awaited
 * after, because a signal that has already gone past cannot be waited for:
 *
 * ```ts
 * const branchesLoaded = waitForHttp(page, "GET", /\/repositories\/[^/]+\/branches/);
 * await createRepositoryButton.click();
 * await branchesLoaded; // the cause has landed…
 * await expect(branchSelector).toBeEnabled(); // …so this needs no budget
 * ```
 *
 * The trailing UI assertion keeps its default timeout on purpose: once the
 * cause has landed, all that is left is a React render, and a render slower
 * than the default is a real bug worth failing on.
 */

/**
 * Generous enough for a loaded CI machine, far below the 60s per-test timeout,
 * so a missed signal fails with this module's message (which names the signal)
 * rather than as an anonymous test timeout.
 */
const DEFAULT_TIMEOUT = 15_000;

/** The kandev gateway socket. Terminal/preview sockets are deliberately ignored. */
const GATEWAY_WS_SUFFIX = "/ws";

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

export type WaitForHttpOptions = {
  timeout?: number;
  /**
   * Extra filter applied after method+path match, e.g. `(r) => r.status() === 201`,
   * or a body check when several requests share one route.
   */
  predicate?: (response: Response) => boolean | Promise<boolean>;
};

/**
 * Resolve when the page receives an HTTP response whose method and URL path
 * match. Arm it before the action that triggers the request.
 *
 * `path` is matched against `URL.pathname` only, so query strings never
 * interfere and a pattern anchored with `$` still works.
 */
export async function waitForHttp(
  page: Page,
  method: string,
  path: RegExp,
  options: WaitForHttpOptions = {},
): Promise<Response> {
  const { timeout = DEFAULT_TIMEOUT, predicate } = options;
  try {
    return await page.waitForResponse(
      async (response) => {
        if (response.request().method() !== method) return false;
        if (!path.test(new URL(response.url()).pathname)) return false;
        return predicate ? await predicate(response) : true;
      },
      { timeout },
    );
  } catch (cause) {
    throw new Error(`waitForHttp: no ${method} response matching ${path} within ${timeout}ms`, {
      cause,
    });
  }
}

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

/**
 * A decoded gateway frame. The wire format is
 * `{id, type, action, payload, timestamp}`: client requests carry
 * `type: "request"`, server pushes carry `type: "notification"`, and replies
 * carry `type: "response"` / `"error"` echoing both the originating request's
 * `id` and its `action`.
 *
 * The echoed `action` is not enough to identify *your* round trip: several
 * `task.plan.get` requests can be in flight from different components, and a
 * reply to one sent before you started waiting would satisfy an action-only
 * match. {@link WsWatcher.waitForResponse} therefore correlates by `id`,
 * which is also what makes its arm-before-you-act guarantee real.
 */
export type WsFrame = {
  id?: string;
  type?: string;
  action?: string;
  payload: Record<string, unknown>;
};

export type WaitForWsOptions = {
  timeout?: number;
  /** Narrow to one subject, e.g. `(p) => p.task_id === task.id`. */
  where?: (payload: Record<string, unknown>) => boolean;
};

export type WsWatcher = {
  /**
   * Resolve on the next server-pushed notification carrying this `action`.
   * Arm before the action that makes the backend publish it.
   */
  waitForEvent(action: string, options?: WaitForWsOptions): Promise<WsFrame>;
  /**
   * Resolve when a WS request the client sends with this `action` *after
   * arming* gets its reply, correlated by frame `id`. Rejects if the backend
   * answers with an `error` frame.
   */
  waitForResponse(action: string, options?: { timeout?: number }): Promise<WsFrame>;
};

function decodeFrame(payload: string | Buffer | Uint8Array): WsFrame | null {
  const text =
    typeof payload === "string"
      ? payload
      : new TextDecoder("utf-8", { fatal: false }).decode(payload);
  try {
    const parsed = JSON.parse(text) as Record<string, unknown>;
    if (!parsed || typeof parsed !== "object") return null;
    const body = parsed.payload;
    return {
      id: typeof parsed.id === "string" ? parsed.id : undefined,
      type: typeof parsed.type === "string" ? parsed.type : undefined,
      action: typeof parsed.action === "string" ? parsed.action : undefined,
      payload: body && typeof body === "object" ? (body as Record<string, unknown>) : {},
    };
  } catch {
    return null; // binary terminal traffic and other non-JSON frames
  }
}

type FrameListener = (frame: WsFrame) => void;

type Channels = {
  sent: Set<FrameListener>;
  received: Set<FrameListener>;
};

/**
 * Start observing the gateway WebSocket so later `waitForEvent` /
 * `waitForResponse` calls have something to listen to.
 *
 * **Call this before `page.goto()`.** Playwright's `page.on("websocket")`
 * only fires for sockets opened after the listener is attached, so a watcher
 * created once the app is running observes nothing. Attaching early is free:
 * nothing is buffered, frames are dispatched straight to whichever waits are
 * armed at the time. The watcher survives `page.reload()`.
 */
export function watchWs(page: Page): WsWatcher {
  const channels: Channels = { sent: new Set(), received: new Set() };

  const dispatch = (listeners: Set<FrameListener>, payload: string | Buffer | Uint8Array) => {
    if (listeners.size === 0) return;
    const frame = decodeFrame(payload);
    if (!frame) return;
    for (const listener of [...listeners]) listener(frame);
  };

  page.on("websocket", (webSocket) => {
    if (!webSocket.url().endsWith(GATEWAY_WS_SUFFIX)) return;
    webSocket.on("framesent", (event) => dispatch(channels.sent, event.payload));
    webSocket.on("framereceived", (event) => dispatch(channels.received, event.payload));
  });

  return {
    waitForEvent: (action, options = {}) => waitForEvent(channels, action, options),
    waitForResponse: (action, options = {}) => waitForResponse(channels, action, options),
  };
}

function waitForEvent(
  channels: Channels,
  action: string,
  options: WaitForWsOptions,
): Promise<WsFrame> {
  const { timeout = DEFAULT_TIMEOUT, where } = options;
  return new Promise<WsFrame>((resolve, reject) => {
    const wait = armWsWait(
      timeout,
      () => `watchWs.waitForEvent: no "${action}" notification within ${timeout}ms`,
      reject,
    );
    wait.listen(channels.received, (frame) => {
      if (frame.action !== action) return;
      if (frame.type && frame.type !== "notification") return;
      if (where && !where(frame.payload)) return;
      wait.dispose();
      resolve(frame);
    });
  });
}

function waitForResponse(
  channels: Channels,
  action: string,
  options: { timeout?: number },
): Promise<WsFrame> {
  const { timeout = DEFAULT_TIMEOUT } = options;
  return new Promise<WsFrame>((resolve, reject) => {
    const requestIds = new Set<string>();
    const wait = armWsWait(
      timeout,
      () => `watchWs.waitForResponse: no reply to "${action}" within ${timeout}ms`,
      reject,
    );
    wait.listen(channels.sent, (frame) => {
      if (frame.action === action && frame.id) requestIds.add(frame.id);
    });
    wait.listen(channels.received, (frame) => {
      if (!frame.id || !requestIds.has(frame.id)) return;
      if (frame.type === "error") {
        wait.dispose();
        const detail = JSON.stringify(frame.payload);
        reject(new Error(`watchWs.waitForResponse: "${action}" was rejected: ${detail}`));
        return;
      }
      if (frame.type !== "response") return;
      wait.dispose();
      resolve(frame);
    });
  });
}

type ArmedWait = {
  listen: (channel: Set<FrameListener>, listener: FrameListener) => void;
  dispose: () => void;
};

/** Register-and-teardown bookkeeping shared by the two WS waits. */
function armWsWait(
  timeout: number,
  message: () => string,
  reject: (error: Error) => void,
): ArmedWait {
  const registered: Array<[Set<FrameListener>, FrameListener]> = [];
  const unregister = () => {
    for (const [channel, listener] of registered) channel.delete(listener);
    registered.length = 0;
  };
  const timer = setTimeout(() => {
    unregister();
    reject(new Error(message()));
  }, timeout);
  return {
    listen(channel, listener) {
      registered.push([channel, listener]);
      channel.add(listener);
    },
    dispose() {
      clearTimeout(timer);
      unregister();
    },
  };
}

// ---------------------------------------------------------------------------
// Wall clock
// ---------------------------------------------------------------------------

/**
 * The closed set of reasons a wall-clock wait can be legitimate, ordered by the
 * decision procedure in {@link dwell}. Exported as a const array so a ratchet
 * or report can enumerate categories without restating the union.
 */
export const DWELL_CATEGORIES = [
  /** The assertion is that something never happens, so there is no event. Permanent. */
  "negative-assertion",
  /** A `setTimeout`/debounce in our own code that publishes nothing. Fixable in principle. */
  "product-timer",
  /** A third-party timer we do not control: Radix open delay, dnd-kit sensor arming. */
  "library-timer",
  /** Forcing two writes apart so their timestamps or ordering stay distinguishable. */
  "clock-separation",
  /** Waiting out a polling loop. Usually the best candidate for conversion to `waitForHttp`. */
  "poll-interval",
  /** Browser-level chrome the page cannot observe: native dialogs, focus, print. */
  "browser-chrome",
  /** Pre-existing spacing that could not be tied to any timer. Debt, not intent. */
  "unverified",
] as const;

export type DwellCategory = (typeof DWELL_CATEGORIES)[number];

/**
 * Wait on the wall clock, on purpose, with a category and a reason.
 *
 * This is the **only** sanctioned way to sleep in a spec. Raw
 * `page.waitForTimeout()` and hand-rolled promise sleeps are not: they are
 * indistinguishable, at a glance and to a grep, from someone who could not
 * find the right event and reached for a number instead.
 *
 * ```ts
 * await dwell(page, 300, "negative-assertion", "asserting the tooltip never opens");
 * await dwell(page, 300, "library-timer", "Radix open delay publishes no event");
 * ```
 *
 * Backend fixtures, the API client's retry loops, docker probing and the office
 * routing helpers have no `Page` at all, so there is a page-less form under the
 * same name -- one greppable token either way:
 *
 * ```ts
 * await dwell(500, "poll-interval", "backend health poll; no page exists yet");
 * ```
 *
 * Pass the page whenever one is in scope: the wait then dies with the page
 * rather than hanging past it. The page-less form is a plain timer.
 *
 * ## Choosing the category
 *
 * Several can look applicable at once, so categorize by *what makes the delay
 * unavoidable*, first match wins:
 *
 * 1. Is the assertion that something **never** happens? → `negative-assertion`.
 *    You always know this from the test itself, and it outranks whatever timer
 *    happens to be nearby.
 * 2. Otherwise you are waiting out a timer. Can you **name** it? → the matching
 *    one of `product-timer`, `library-timer`, `clock-separation`,
 *    `poll-interval`, `browser-chrome`.
 * 3. Cannot name it? → `unverified`. Do not guess a plausible-looking timer;
 *    an honest debt marker is worth more than a confident wrong label.
 *
 * `unverified` is a debt marker to be driven down, not a resting place. It is
 * the one category that should trend toward zero.
 *
 * `reason` must say *why no event exists*, not what the code is doing. "Radix
 * opens after 300ms and publishes nothing" is a reason; "wait for the tooltip"
 * is not — that one is a {@link waitForHttp} or {@link watchWs} wait that has
 * not been found yet, and reaching for `dwell` to avoid looking is the misuse
 * this helper exists to make visible.
 *
 * Deliberately has no options and no defaults. All of its value is in the name
 * being greppable, and the category and reason being mandatory.
 */
export function dwell(ms: number, category: DwellCategory, reason: string): Promise<void>;
export function dwell(
  page: Page,
  ms: number,
  category: DwellCategory,
  reason: string,
): Promise<void>;
export function dwell(
  pageOrMs: Page | number,
  msOrCategory: number | DwellCategory,
  categoryOrReason: DwellCategory | string,
  maybeReason?: string,
): Promise<void> {
  // A Page is never a number, so the first argument discriminates the two forms.
  const pageless = typeof pageOrMs === "number";
  const page = pageless ? null : pageOrMs;
  const ms = pageless ? pageOrMs : (msOrCategory as number);
  const category = (pageless ? msOrCategory : categoryOrReason) as DwellCategory;
  const reason = pageless ? (categoryOrReason as string) : maybeReason;

  // Checked at runtime, not just by the type. `apps/web/tsconfig.json` excludes
  // `e2e`, and Playwright and vitest both strip types without checking them, so
  // nothing in CI would catch a typo'd category on the strength of the union
  // alone -- only an editor would. This keeps the closed set actually closed.
  if (!DWELL_CATEGORIES.includes(category)) {
    throw new Error(
      `dwell: unknown category "${category}", expected one of ${DWELL_CATEGORIES.join(", ")}`,
    );
  }
  if (typeof reason !== "string" || reason.trim() === "") {
    throw new Error("dwell: a reason is required, and must say why no event exists to wait on");
  }

  // With a page, delegate: the wait then dies with the page instead of hanging
  // past it. Without one, a plain timer is all there is.
  return page ? page.waitForTimeout(ms) : new Promise((resolve) => setTimeout(resolve, ms));
}
