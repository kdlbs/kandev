import type { Page } from "@playwright/test";

export type WsAccountReceivedEvent = {
  connectionSeq: number;
  sessionSeq?: number;
  action: string;
  sessionId: string | null;
  type: string;
};

export type WsAccountSessionSnapshot = {
  processedSeqs: number[];
  gaps: number[];
  minSeq: number | null;
  maxSeq: number | null;
};

export type WsAccountSnapshot = {
  connectionId: string | null;
  processedSeqs: number[];
  gaps: number[];
  maxSeq: number | null;
  minSeq: number | null;
  receivedEvents: WsAccountReceivedEvent[];
  bySession: Record<string, WsAccountSessionSnapshot>;
};

export type WsSentEvent = {
  connection_seq: number;
  session_seq?: number;
  session_id?: string;
  type: string;
  action: string;
  sent_at: string;
};

export type WsSentResponse = {
  connection_id: string;
  session_id?: string;
  events: WsSentEvent[];
  max_connection_seq?: number;
  max_session_seq?: number;
};

export type DroppedEvent = WsSentEvent;

export type WsSentFetcher = {
  getWsSent(connectionId: string, sinceSeq?: number, sessionId?: string): Promise<WsSentResponse>;
};

export type ComputeWsDropsOptions = {
  strict?: boolean;
};

export type ExpectedWsDrop = {
  action?: string;
  type?: string;
  sessionId?: string;
  reason?: string;
};

export type ExpectedWsDropResult = {
  unexpected: DroppedEvent[];
  missing: ExpectedWsDrop[];
};

const expectedDropsByPage = new WeakMap<Page, ExpectedWsDrop[]>();

export async function readWsAccount(page: Page): Promise<WsAccountSnapshot | null> {
  return page.evaluate(() => {
    type Hook = () => WsAccountSnapshot;
    const win = window as unknown as { __kandev_ws_account__?: Hook };
    return win.__kandev_ws_account__ ? win.__kandev_ws_account__() : null;
  });
}

async function readWsAccountAfterNavigationSettles(page: Page): Promise<WsAccountSnapshot | null> {
  const snapshot = await readWsAccountWithNavigationRetry(page);
  if (snapshot || !pageHasLoadedApp(page)) return snapshot;
  await waitForWsAccountHook(page);
  return readWsAccountWithNavigationRetry(page);
}

async function waitForWsAccountHook(page: Page): Promise<void> {
  await page
    .waitForFunction(
      () => {
        const win = window as Window & { __kandev_ws_account__?: unknown };
        return typeof win.__kandev_ws_account__ === "function";
      },
      undefined,
      { timeout: 1000 },
    )
    .catch(() => undefined);
}

async function readWsAccountWithNavigationRetry(page: Page): Promise<WsAccountSnapshot | null> {
  let lastError: unknown;
  for (let attempt = 0; attempt < 5; attempt++) {
    try {
      return await readWsAccount(page);
    } catch (error) {
      if (!isNavigationEvaluationError(error)) throw error;
      lastError = error;
      await page.waitForLoadState("domcontentloaded", { timeout: 1000 }).catch(() => undefined);
      await page.waitForTimeout(50);
    }
  }
  throw lastError;
}

function isNavigationEvaluationError(error: unknown): boolean {
  const message = String(error);
  return (
    message.includes("Execution context was destroyed") ||
    message.includes("Cannot find context with specified id") ||
    message.includes("Frame was detached")
  );
}

export async function clearWsAccount(page: Page): Promise<void> {
  await page.evaluate(() => {
    type Hook = () => void;
    const win = window as unknown as { __kandev_ws_account_clear__?: Hook };
    win.__kandev_ws_account_clear__?.();
  });
}

export function registerExpectedWsDrop(page: Page, expected: ExpectedWsDrop): void {
  const existing = expectedDropsByPage.get(page) ?? [];
  existing.push(expected);
  expectedDropsByPage.set(page, existing);
}

export function reconcileExpectedWsDrops(page: Page, drops: DroppedEvent[]): ExpectedWsDropResult {
  const expected = [...(expectedDropsByPage.get(page) ?? [])];
  expectedDropsByPage.delete(page);
  const unexpected: DroppedEvent[] = [];

  for (const drop of drops) {
    const matchIndex = expected.findIndex((candidate) => expectedDropMatches(candidate, drop));
    if (matchIndex === -1) {
      unexpected.push(drop);
      continue;
    }
    expected.splice(matchIndex, 1);
  }

  return { unexpected, missing: expected };
}

export async function computeWsDrops(
  page: Page,
  fetcher: WsSentFetcher,
  options: ComputeWsDropsOptions = {},
): Promise<DroppedEvent[]> {
  const snapshot = await readWsAccountAfterNavigationSettles(page);
  if (!snapshot) {
    if (options.strict && pageHasLoadedApp(page)) {
      throw new Error(
        "Strict WS accounting could not read the browser hook; " +
          "the page either did not install the E2E hook or loaded before it was available.",
      );
    }
    return [];
  }
  if (!snapshot.connectionId) {
    if (options.strict && snapshot.processedSeqs.length > 0) {
      throw new Error(
        "Strict WS accounting parsed stamped WS envelopes without a browser connection id.",
      );
    }
    return [];
  }
  const sinceSeq = snapshot.minSeq === null ? undefined : Math.max(0, snapshot.minSeq - 1);
  const connectionSent = await getWsSent(fetcher, snapshot.connectionId, sinceSeq, undefined, {
    strict: options.strict,
  });
  const connectionDrops = connectionSent ? diffConnectionDrops(snapshot, connectionSent) : [];
  const sessionDrops = await computeSessionDrops(fetcher, snapshot, options);
  return dedupeDrops([...connectionDrops, ...sessionDrops]);
}

export function formatDroppedEvents(drops: DroppedEvent[]): string {
  if (drops.length === 0) return "";
  const lines = drops.slice(0, 20).map((drop) => {
    const session = drop.session_id ? ` session=${drop.session_id}` : "";
    const sessionSeq = drop.session_seq ? ` session_seq=${drop.session_seq}` : "";
    return `  connection_seq=${drop.connection_seq}${sessionSeq}${session} ${drop.type}/${drop.action} sent_at=${drop.sent_at}`;
  });
  const more = drops.length > 20 ? `\n  ... and ${drops.length - 20} more` : "";
  return `${drops.length} WS event(s) sent by backend but not parsed by frontend:\n${lines.join("\n")}${more}`;
}

export function formatMissingExpectedDrops(missing: ExpectedWsDrop[]): string {
  if (missing.length === 0) return "";
  const lines = missing.slice(0, 20).map((drop) => {
    const session = drop.sessionId ? ` session=${drop.sessionId}` : "";
    const action = drop.action ? ` action=${drop.action}` : "";
    const type = drop.type ? ` type=${drop.type}` : "";
    const reason = drop.reason ? ` reason=${drop.reason}` : "";
    return `  expected WS drop${type}${action}${session}${reason}`;
  });
  const more = missing.length > 20 ? `\n  ... and ${missing.length - 20} more` : "";
  return `${missing.length} expected WS drop(s) were not observed:\n${lines.join("\n")}${more}`;
}

function diffConnectionDrops(
  snapshot: WsAccountSnapshot,
  backendSent: WsSentResponse,
): DroppedEvent[] {
  const maxSeq = snapshot.maxSeq;
  if (maxSeq === null) return [];
  const received = new Set(snapshot.processedSeqs);
  return backendSent.events.filter(
    (event) => event.connection_seq <= maxSeq && !received.has(event.connection_seq),
  );
}

async function computeSessionDrops(
  fetcher: WsSentFetcher,
  snapshot: WsAccountSnapshot,
  options: ComputeWsDropsOptions,
): Promise<DroppedEvent[]> {
  if (!snapshot.connectionId) return [];
  const drops: DroppedEvent[] = [];
  for (const sessionId of Object.keys(snapshot.bySession)) {
    const sent = await getWsSent(fetcher, snapshot.connectionId, undefined, sessionId, {
      strict: options.strict,
    });
    if (!sent) continue;
    const sessionSnapshot = snapshot.bySession[sessionId];
    if (!sessionSnapshot || sessionSnapshot.minSeq === null || sessionSnapshot.maxSeq === null) {
      continue;
    }
    const received = new Set(sessionSnapshot.processedSeqs);
    for (const event of sent.events) {
      if (
        event.session_seq &&
        event.session_seq >= sessionSnapshot.minSeq &&
        event.session_seq <= sessionSnapshot.maxSeq &&
        !received.has(event.session_seq)
      ) {
        drops.push(event);
      }
    }
  }
  return drops;
}

async function getWsSent(
  fetcher: WsSentFetcher,
  connectionId: string,
  sinceSeq?: number,
  sessionId?: string,
  options: ComputeWsDropsOptions = {},
): Promise<WsSentResponse | null> {
  try {
    return await fetcher.getWsSent(connectionId, sinceSeq, sessionId);
  } catch (error) {
    if (options.strict) {
      throw new Error(`Strict WS accounting sent-log lookup failed: ${String(error)}`);
    }
    return null;
  }
}

function dedupeDrops(drops: DroppedEvent[]): DroppedEvent[] {
  const seen = new Set<number>();
  const unique: DroppedEvent[] = [];
  for (const drop of drops) {
    if (seen.has(drop.connection_seq)) continue;
    seen.add(drop.connection_seq);
    unique.push(drop);
  }
  return unique;
}

function expectedDropMatches(expected: ExpectedWsDrop, drop: DroppedEvent): boolean {
  if (expected.action && drop.action !== expected.action) return false;
  if (expected.type && drop.type !== expected.type) return false;
  if (expected.sessionId && drop.session_id !== expected.sessionId) return false;
  return true;
}

function pageHasLoadedApp(page: Page): boolean {
  return page.url() !== "about:blank";
}
