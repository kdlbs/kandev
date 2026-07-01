import { describe, expect, it } from "vitest";
import { WsAccount, detectGaps } from "./ws-account";

function envelope(
  connectionSeq: number,
  overrides: Partial<{
    connectionId: string;
    sessionSeq: number;
    sessionId: string;
    action: string;
    type: string;
  }> = {},
) {
  return {
    connection_id: overrides.connectionId ?? "conn-1",
    connection_seq: connectionSeq,
    session_id: overrides.sessionId,
    session_seq: overrides.sessionSeq,
    type: overrides.type ?? "notification",
    action: overrides.action ?? "session.message.added",
    payload: overrides.sessionId ? { session_id: overrides.sessionId } : {},
  };
}

describe("detectGaps", () => {
  it("returns missing integers between the smallest and largest seq", () => {
    expect(detectGaps([1, 2, 4, 7])).toEqual([3, 5, 6]);
  });

  it("does not report gaps for empty, single, or contiguous input", () => {
    expect(detectGaps([])).toEqual([]);
    expect(detectGaps([8])).toEqual([]);
    expect(detectGaps([8, 9, 10])).toEqual([]);
  });
});

describe("WsAccount", () => {
  it("records parsed connection envelopes and detects connection gaps", () => {
    const account = new WsAccount();
    account.recordEnvelope(envelope(1));
    account.recordEnvelope(envelope(2));
    account.recordEnvelope(envelope(4));

    expect(account.snapshot()).toMatchObject({
      connectionId: "conn-1",
      processedSeqs: [1, 2, 4],
      gaps: [3],
      minSeq: 1,
      maxSeq: 4,
    });
  });

  it("resets buckets when the backend connection id changes", () => {
    const account = new WsAccount();
    account.recordEnvelope(
      envelope(1, { connectionId: "conn-a", sessionId: "session-1", sessionSeq: 1 }),
    );
    account.recordEnvelope(
      envelope(1, { connectionId: "conn-b", sessionId: "session-1", sessionSeq: 1 }),
    );

    expect(account.snapshot()).toMatchObject({
      connectionId: "conn-b",
      processedSeqs: [1],
      bySession: {
        "session-1": {
          processedSeqs: [1],
          gaps: [],
        },
      },
    });
  });

  it("clears the current connection id and tracked events", () => {
    const account = new WsAccount();
    account.recordEnvelope(envelope(1, { connectionId: "conn-a" }));
    account.clear();

    expect(account.snapshot()).toMatchObject({
      connectionId: null,
      processedSeqs: [],
      bySession: {},
    });
  });

  it("tracks independent per-session sequence buckets", () => {
    const account = new WsAccount();
    account.recordEnvelope(envelope(1, { sessionId: "session-a", sessionSeq: 1 }));
    account.recordEnvelope(envelope(2, { sessionId: "session-b", sessionSeq: 1 }));
    account.recordEnvelope(envelope(3, { sessionId: "session-a", sessionSeq: 3 }));

    const snapshot = account.snapshot();
    expect(snapshot.gaps).toEqual([]);
    expect(snapshot.bySession["session-a"]).toMatchObject({
      processedSeqs: [1, 3],
      gaps: [2],
    });
    expect(snapshot.bySession["session-b"]).toMatchObject({
      processedSeqs: [1],
      gaps: [],
    });
  });

  it("uses only the stamped top-level session id for session buckets", () => {
    const account = new WsAccount();
    account.recordEnvelope({
      connection_id: "conn-1",
      connection_seq: 1,
      session_seq: 1,
      type: "notification",
      action: "session.message.added",
      session_id: "session-stamped",
      payload: { session_id: "session-payload" },
    });
    account.recordEnvelope({
      connection_id: "conn-1",
      connection_seq: 2,
      session_seq: 1,
      type: "notification",
      action: "session.message.added",
      payload: { session_id: "payload-only" },
    });

    expect(account.snapshot().bySession).toEqual({
      "session-stamped": {
        processedSeqs: [1],
        gaps: [],
        minSeq: 1,
        maxSeq: 1,
      },
    });
  });

  it("evicts oldest entries per bucket", () => {
    const account = new WsAccount(2);
    account.recordEnvelope(envelope(1, { sessionId: "session-a", sessionSeq: 1 }));
    account.recordEnvelope(envelope(2, { sessionId: "session-a", sessionSeq: 2 }));
    account.recordEnvelope(envelope(3, { sessionId: "session-a", sessionSeq: 3 }));

    const snapshot = account.snapshot();
    expect(snapshot.processedSeqs).toEqual([2, 3]);
    expect(snapshot.bySession["session-a"].processedSeqs).toEqual([2, 3]);
  });
});
