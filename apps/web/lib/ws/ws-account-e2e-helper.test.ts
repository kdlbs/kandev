import type { Page } from "@playwright/test";
import { describe, expect, it, vi } from "vitest";
import {
  computeWsDrops,
  reconcileExpectedWsDrops,
  registerExpectedWsDrop,
  type WsAccountSnapshot,
  type WsSentEvent,
  type WsSentFetcher,
} from "../../e2e/helpers/ws-account";

function pageWithSnapshot(snapshot: WsAccountSnapshot | null, url = "http://127.0.0.1:18080/") {
  return {
    evaluate: vi.fn(async () => snapshot),
    waitForFunction: vi.fn(async () => undefined),
    url: () => url,
  } as unknown as Page;
}

function noOpFetcher(): WsSentFetcher {
  return {
    getWsSent: vi.fn(async () => ({
      connection_id: "conn-1",
      events: [],
    })),
  };
}

function snapshot(): WsAccountSnapshot {
  return {
    connectionId: "conn-1",
    processedSeqs: [1],
    gaps: [],
    minSeq: 1,
    maxSeq: 1,
    receivedEvents: [],
    bySession: {},
  };
}

const SENT_AT = "2026-06-23T00:00:00Z";

function sentEvent(connectionSeq: number, action: string, sessionSeq?: number): WsSentEvent {
  return {
    connection_seq: connectionSeq,
    ...(sessionSeq ? { session_seq: sessionSeq, session_id: "session-1" } : {}),
    type: "notification",
    action,
    sent_at: SENT_AT,
  };
}

function fetcherWithEvents(
  connectionEvents: WsSentEvent[],
  sessionEvents: WsSentEvent[] = [],
): WsSentFetcher {
  return {
    getWsSent: vi.fn(async (_connectionId, _sinceSeq, sessionId) => ({
      connection_id: "conn-1",
      events: sessionId ? sessionEvents : connectionEvents,
    })),
  };
}

describe("computeWsDrops strict mode", () => {
  it("fails strict accounting when an app page has no browser hook", async () => {
    const page = pageWithSnapshot(null);

    await expect(computeWsDrops(page, noOpFetcher(), { strict: true })).rejects.toThrow(
      "Strict WS accounting could not read the browser hook",
    );
    expect(page.waitForFunction).toHaveBeenCalled();
  });

  it("waits for the browser hook after app navigation before strict accounting fails", async () => {
    const readySnapshot = snapshot();
    const page = {
      evaluate: vi.fn().mockResolvedValueOnce(null).mockResolvedValueOnce(readySnapshot),
      waitForFunction: vi.fn(async () => undefined),
      url: () => "http://127.0.0.1:18080/settings/agents",
    } as unknown as Page;

    await expect(computeWsDrops(page, noOpFetcher(), { strict: true })).resolves.toEqual([]);
    expect(page.waitForFunction).toHaveBeenCalled();
  });

  it("allows strict accounting when the hook is installed before any frames arrive", async () => {
    const page = pageWithSnapshot({
      connectionId: null,
      processedSeqs: [],
      gaps: [],
      minSeq: null,
      maxSeq: null,
      receivedEvents: [],
      bySession: {},
    });

    await expect(computeWsDrops(page, noOpFetcher(), { strict: true })).resolves.toEqual([]);
  });

  it("fails strict accounting when stamped frames omit the connection id", async () => {
    const page = pageWithSnapshot({
      connectionId: null,
      processedSeqs: [1],
      gaps: [],
      minSeq: 1,
      maxSeq: 1,
      receivedEvents: [],
      bySession: {},
    });

    await expect(computeWsDrops(page, noOpFetcher(), { strict: true })).rejects.toThrow(
      "Strict WS accounting parsed stamped WS envelopes without a browser connection id",
    );
  });

  it("does not fail strict accounting before a page has loaded the app", async () => {
    const page = pageWithSnapshot(null, "about:blank");

    await expect(computeWsDrops(page, noOpFetcher(), { strict: true })).resolves.toEqual([]);
    expect(page.waitForFunction).not.toHaveBeenCalled();
  });

  it("fails strict accounting when the backend sent-log lookup fails", async () => {
    const page = pageWithSnapshot(snapshot());
    const fetcher: WsSentFetcher = {
      getWsSent: vi.fn(async () => {
        throw new Error("missing route");
      }),
    };

    await expect(computeWsDrops(page, fetcher, { strict: true })).rejects.toThrow(
      "Strict WS accounting sent-log lookup failed",
    );
  });
});

describe("computeWsDrops connection sequences", () => {
  it("ignores connection events sent after the browser snapshot max sequence", async () => {
    const page = pageWithSnapshot({
      connectionId: "conn-1",
      processedSeqs: [1, 2],
      gaps: [],
      minSeq: 1,
      maxSeq: 2,
      receivedEvents: [],
      bySession: {},
    });
    const fetcher = fetcherWithEvents([
      sentEvent(1, "one"),
      sentEvent(2, "two"),
      sentEvent(3, "in-flight"),
    ]);

    await expect(computeWsDrops(page, fetcher, { strict: true })).resolves.toEqual([]);
  });

  it("still reports connection gaps at or below the browser snapshot max sequence", async () => {
    const page = pageWithSnapshot({
      connectionId: "conn-1",
      processedSeqs: [1, 3],
      gaps: [2],
      minSeq: 1,
      maxSeq: 3,
      receivedEvents: [],
      bySession: {},
    });
    const missing = sentEvent(2, "two");
    const fetcher = fetcherWithEvents([sentEvent(1, "one"), missing, sentEvent(3, "three")]);

    await expect(computeWsDrops(page, fetcher, { strict: true })).resolves.toEqual([missing]);
  });
});

describe("computeWsDrops session sequences", () => {
  it("ignores session events sent after the browser snapshot max session sequence", async () => {
    const page = pageWithSnapshot({
      connectionId: "conn-1",
      processedSeqs: [1, 2],
      gaps: [],
      minSeq: 1,
      maxSeq: 2,
      receivedEvents: [],
      bySession: {
        "session-1": { processedSeqs: [1], gaps: [], minSeq: 1, maxSeq: 1 },
      },
    });
    const fetcher = fetcherWithEvents(
      [sentEvent(1, "one"), sentEvent(2, "two")],
      [sentEvent(1, "one", 1), sentEvent(2, "in-flight", 2)],
    );

    await expect(computeWsDrops(page, fetcher, { strict: true })).resolves.toEqual([]);
  });

  it("still reports session gaps at or below the browser snapshot max session sequence", async () => {
    const page = pageWithSnapshot({
      connectionId: "conn-1",
      processedSeqs: [1, 2, 3],
      gaps: [],
      minSeq: 1,
      maxSeq: 3,
      receivedEvents: [],
      bySession: {
        "session-1": { processedSeqs: [1, 3], gaps: [2], minSeq: 1, maxSeq: 3 },
      },
    });
    const missing = sentEvent(2, "two", 2);
    const fetcher = fetcherWithEvents(
      [sentEvent(1, "one"), sentEvent(2, "two"), sentEvent(3, "three")],
      [sentEvent(1, "one", 1), missing, sentEvent(3, "three", 3)],
    );

    await expect(computeWsDrops(page, fetcher, { strict: true })).resolves.toEqual([missing]);
  });
});

describe("reconcileExpectedWsDrops", () => {
  it("consumes matching expected drops and preserves unexpected drops", () => {
    const page = {} as Page;
    registerExpectedWsDrop(page, {
      type: "notification",
      action: "session.message.added",
      sessionId: "session-1",
    });

    const expected = {
      connection_seq: 1,
      session_seq: 1,
      session_id: "session-1",
      type: "notification",
      action: "session.message.added",
      sent_at: "2026-06-23T00:00:00Z",
    };
    const unexpected = {
      connection_seq: 2,
      session_seq: 2,
      session_id: "session-1",
      type: "notification",
      action: "session.message.updated",
      sent_at: "2026-06-23T00:00:01Z",
    };

    expect(reconcileExpectedWsDrops(page, [expected, unexpected])).toEqual({
      unexpected: [unexpected],
      missing: [],
    });
  });

  it("reports registered expected drops that were not observed", () => {
    const page = {} as Page;
    const missing = { action: "session.message.added", reason: "intentional drop" };
    registerExpectedWsDrop(page, missing);

    expect(reconcileExpectedWsDrops(page, [])).toEqual({
      unexpected: [],
      missing: [missing],
    });
  });
});
