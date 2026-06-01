import { describe, it, expect } from "vitest";
import { WsAccount, detectGaps, WS_ACCOUNT_MAX_ENTRIES } from "./ws-account";

describe("detectGaps", () => {
  const cases: Array<{ name: string; input: number[]; expected: number[] }> = [
    { name: "empty input", input: [], expected: [] },
    { name: "single element", input: [42], expected: [] },
    { name: "contiguous", input: [1, 2, 3, 4, 5], expected: [] },
    { name: "single gap in middle", input: [1, 2, 4, 5], expected: [3] },
    { name: "multiple gaps", input: [1, 2, 4, 7], expected: [3, 5, 6] },
    { name: "gap right after min", input: [10, 13, 14], expected: [11, 12] },
    { name: "gap right before max", input: [10, 11, 14], expected: [12, 13] },
    { name: "two element contiguous", input: [5, 6], expected: [] },
    { name: "two element with gap", input: [5, 9], expected: [6, 7, 8] },
    {
      name: "high values",
      input: [1000, 1001, 1005],
      expected: [1002, 1003, 1004],
    },
  ];

  for (const c of cases) {
    it(c.name, () => {
      expect(detectGaps(c.input)).toEqual(c.expected);
    });
  }

  it("handles a long contiguous range without false positives", () => {
    const input = Array.from({ length: 1000 }, (_, i) => i + 1);
    expect(detectGaps(input)).toEqual([]);
  });

  it("finds many gaps in a large input", () => {
    // 1..100 with even numbers removed (i.e. only odd seqs).
    const input = Array.from({ length: 50 }, (_, i) => i * 2 + 1);
    const gaps = detectGaps(input);
    // Expect every even integer between 2 and 98 inclusive.
    expect(gaps).toEqual(Array.from({ length: 49 }, (_, i) => (i + 1) * 2));
  });
});

const DEFAULT_ACTION = "session.message.added";

function entry(
  seq: number,
  overrides: Partial<{
    type: string;
    action: string;
    sessionId: string | null;
    sessionSeq: number;
  }> = {},
) {
  return {
    seq,
    sessionSeq: overrides.sessionSeq,
    type: overrides.type ?? "notification",
    action: overrides.action ?? DEFAULT_ACTION,
    sessionId: overrides.sessionId === undefined ? null : overrides.sessionId,
    receivedAt: seq * 1000,
  };
}

const EMPTY_SNAPSHOT = {
  connectionId: null,
  processedSeqs: [],
  gaps: [],
  maxSeq: null,
  minSeq: null,
  receivedEvents: [],
  bySession: {},
};

describe("WsAccount — basic recording (connection bucket)", () => {
  it("snapshot is empty by default", () => {
    const acc = new WsAccount();
    expect(acc.snapshot()).toEqual(EMPTY_SNAPSHOT);
  });

  it("records single envelope with connectionId", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    expect(acc.snapshot()).toEqual({
      connectionId: "conn-a",
      processedSeqs: [1],
      gaps: [],
      maxSeq: 1,
      minSeq: 1,
      receivedEvents: [
        {
          seq: 1,
          sessionSeq: undefined,
          action: DEFAULT_ACTION,
          sessionId: null,
          type: "notification",
        },
      ],
      bySession: {},
    });
  });

  it("snapshot.receivedEvents carries action and sessionId per seq", () => {
    const acc = new WsAccount();
    acc.record(entry(1, { action: DEFAULT_ACTION, sessionId: "sess-1", sessionSeq: 1 }), "conn-a");
    acc.record(entry(2, { action: "task.updated", sessionId: null }), "conn-a");
    expect(acc.snapshot().receivedEvents).toEqual([
      { seq: 1, sessionSeq: 1, action: DEFAULT_ACTION, sessionId: "sess-1", type: "notification" },
      {
        seq: 2,
        sessionSeq: undefined,
        action: "task.updated",
        sessionId: null,
        type: "notification",
      },
    ]);
  });

  it("records multiple envelopes and surfaces gaps", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    acc.record(entry(2), "conn-a");
    acc.record(entry(4), "conn-a");
    acc.record(entry(7), "conn-a");
    const snap = acc.snapshot();
    expect(snap.processedSeqs).toEqual([1, 2, 4, 7]);
    expect(snap.gaps).toEqual([3, 5, 6]);
    expect(snap.minSeq).toBe(1);
    expect(snap.maxSeq).toBe(7);
  });

  it("sorts out-of-order seqs in snapshot", () => {
    const acc = new WsAccount();
    acc.record(entry(5), "conn-a");
    acc.record(entry(2), "conn-a");
    acc.record(entry(8), "conn-a");
    const snap = acc.snapshot();
    expect(snap.processedSeqs).toEqual([2, 5, 8]);
    expect(snap.gaps).toEqual([3, 4, 6, 7]);
  });

  it("deduplicates repeated seqs", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    acc.record(entry(1), "conn-a");
    acc.record(entry(2), "conn-a");
    expect(acc.size()).toBe(2);
    expect(acc.snapshot().processedSeqs).toEqual([1, 2]);
  });

  it("records without connectionId leave it null", () => {
    const acc = new WsAccount();
    acc.record(entry(1));
    expect(acc.snapshot().connectionId).toBeNull();
    expect(acc.snapshot().processedSeqs).toEqual([1]);
  });
});

describe("WsAccount — per-session buckets", () => {
  it("session-scoped event populates BOTH connection and per-session buckets", () => {
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    const snap = acc.snapshot();
    expect(snap.processedSeqs).toEqual([1]);
    expect(snap.bySession).toEqual({
      "sess-A": {
        processedSeqs: [1],
        gaps: [],
        minSeq: 1,
        maxSeq: 1,
      },
    });
  });

  it("connection-wide event (no sessionId) does NOT create a session bucket", () => {
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: null }), "conn-a");
    expect(acc.snapshot().bySession).toEqual({});
    expect(acc.sessionBucketCount()).toBe(0);
  });

  it("session event without sessionSeq is connection-only", () => {
    // Backend always stamps sessionSeq when sessionID is non-empty, but
    // older bundles may emit a sessionId on the payload without a session_seq
    // on the envelope. Treat that as connection-bucket-only to avoid keying
    // a per-session bucket by 0.
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: "sess-A" }), "conn-a");
    expect(acc.snapshot().bySession).toEqual({});
  });

  it("interleaved sessions produce independent monotonic per-session streams", () => {
    const acc = new WsAccount();
    // Per-connection seq grows monotonically across both sessions, but each
    // session has its OWN monotonic session_seq counter on the backend.
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    acc.record(entry(2, { sessionId: "sess-B", sessionSeq: 1 }), "conn-a");
    acc.record(entry(3, { sessionId: "sess-A", sessionSeq: 2 }), "conn-a");
    acc.record(entry(4, { sessionId: "sess-B", sessionSeq: 2 }), "conn-a");
    acc.record(entry(5, { sessionId: "sess-A", sessionSeq: 3 }), "conn-a");

    const snap = acc.snapshot();
    expect(snap.processedSeqs).toEqual([1, 2, 3, 4, 5]);
    expect(snap.gaps).toEqual([]);
    expect(snap.bySession["sess-A"]).toEqual({
      processedSeqs: [1, 2, 3],
      gaps: [],
      minSeq: 1,
      maxSeq: 3,
    });
    expect(snap.bySession["sess-B"]).toEqual({
      processedSeqs: [1, 2],
      gaps: [],
      minSeq: 1,
      maxSeq: 2,
    });
  });

  it("surfaces a per-session gap that the connection bucket misses", () => {
    // Scenario: backend stamped session_seq=3 for sess-A but the FE only
    // received seqs 1 and 3 (event 2 dropped to handler). Per-connection
    // stream is contiguous (different connection seqs), per-session is not.
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    acc.record(entry(2, { sessionId: "sess-B", sessionSeq: 1 }), "conn-a");
    acc.record(entry(3, { sessionId: "sess-A", sessionSeq: 3 }), "conn-a");

    const snap = acc.snapshot();
    expect(snap.gaps).toEqual([]); // connection bucket: 1,2,3 contiguous
    expect(snap.bySession["sess-A"].gaps).toEqual([2]);
  });

  it("clearSession drops one bucket but leaves the rest and the connection bucket", () => {
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    acc.record(entry(2, { sessionId: "sess-B", sessionSeq: 1 }), "conn-a");
    acc.clearSession("sess-A");
    const snap = acc.snapshot();
    expect(Object.keys(snap.bySession)).toEqual(["sess-B"]);
    expect(snap.processedSeqs).toEqual([1, 2]); // connection bucket untouched
  });

  it("adding then dropping a session leaves zero per-session buckets", () => {
    // Regression test: long-lived connections must not accumulate ring
    // buffers for sessions that have come and gone.
    const acc = new WsAccount();
    for (let i = 0; i < 100; i++) {
      const sid = `sess-${i}`;
      acc.record(entry(i + 1, { sessionId: sid, sessionSeq: 1 }), "conn-a");
      acc.clearSession(sid);
    }
    expect(acc.sessionBucketCount()).toBe(0);
    expect(acc.snapshot().bySession).toEqual({});
  });
});

describe("WsAccount — connection rollover & clearing", () => {
  it("clears on new connection id but preserves new id", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    acc.record(entry(2), "conn-a");
    acc.record(entry(1), "conn-b");
    const snap = acc.snapshot();
    expect(snap.connectionId).toBe("conn-b");
    expect(snap.processedSeqs).toEqual([1]);
  });

  it("rollover also clears per-session buckets", () => {
    const acc = new WsAccount();
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    acc.record(entry(2, { sessionId: "sess-A", sessionSeq: 2 }), "conn-a");
    acc.record(entry(1, { sessionId: "sess-A", sessionSeq: 1 }), "conn-b");
    const snap = acc.snapshot();
    expect(snap.connectionId).toBe("conn-b");
    expect(snap.bySession["sess-A"].processedSeqs).toEqual([1]);
  });

  it("clear() empties entries but keeps connectionId", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    acc.record(entry(2, { sessionId: "sess-A", sessionSeq: 1 }), "conn-a");
    acc.clear();
    expect(acc.snapshot()).toEqual({
      ...EMPTY_SNAPSHOT,
      connectionId: "conn-a",
    });
    expect(acc.sessionBucketCount()).toBe(0);
  });

  it("connection rollover via a later record with a new id", () => {
    const acc = new WsAccount();
    acc.record(entry(1), "conn-a");
    acc.record(entry(2), "conn-a");
    acc.record(entry(3), "conn-a");
    // Reconnect: backend seq starts at 1 again on conn-b.
    acc.record(entry(1), "conn-b");
    acc.record(entry(2), "conn-b");
    const snap = acc.snapshot();
    expect(snap.connectionId).toBe("conn-b");
    expect(snap.processedSeqs).toEqual([1, 2]);
    expect(snap.gaps).toEqual([]);
  });
});

describe("WsAccount — eviction", () => {
  it("evicts oldest entries when capacity is exceeded", () => {
    const acc = new WsAccount(3);
    acc.record(entry(1), "conn-a");
    acc.record(entry(2), "conn-a");
    acc.record(entry(3), "conn-a");
    acc.record(entry(4), "conn-a");
    expect(acc.size()).toBe(3);
    expect(acc.snapshot().processedSeqs).toEqual([2, 3, 4]);
  });

  it("default max is 5000 and evicts above it", () => {
    const acc = new WsAccount();
    for (let i = 1; i <= WS_ACCOUNT_MAX_ENTRIES + 10; i++) {
      acc.record(entry(i), "conn-a");
    }
    expect(acc.size()).toBe(WS_ACCOUNT_MAX_ENTRIES);
    const snap = acc.snapshot();
    expect(snap.minSeq).toBe(11);
    expect(snap.maxSeq).toBe(WS_ACCOUNT_MAX_ENTRIES + 10);
  });

  it("per-session bucket honors the same cap independently", () => {
    const acc = new WsAccount(3);
    for (let i = 1; i <= 5; i++) {
      acc.record(entry(i, { sessionId: "sess-A", sessionSeq: i }), "conn-a");
    }
    const snap = acc.snapshot();
    // Connection bucket evicts down to the last 3 too.
    expect(snap.processedSeqs).toEqual([3, 4, 5]);
    expect(snap.bySession["sess-A"].processedSeqs).toEqual([3, 4, 5]);
  });
});
