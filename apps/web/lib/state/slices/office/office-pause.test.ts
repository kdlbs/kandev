import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, WorkspacePauseOutcome, WorkspacePauseRecord } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

const WS_1 = "ws-1";
const WS_2 = "ws-2";

function makeRecord(overrides: Partial<WorkspacePauseRecord> = {}): WorkspacePauseRecord {
  return {
    id: "pause-1",
    reason: "incident",
    createdBy: "default-user",
    createdByKind: "user",
    createdAt: "2026-09-08T00:00:00Z",
    ...overrides,
  };
}

function readSuccess(
  paused: boolean,
  record: WorkspacePauseRecord | null = null,
): WorkspacePauseOutcome {
  return { kind: "read-success", paused, record };
}

describe("office pause store actions: begin/reset", () => {
  it("beginPauseRequest returns a strictly increasing tag", () => {
    const store = makeStore();
    const first = store.getState().beginPauseRequest();
    const second = store.getState().beginPauseRequest();
    expect(second).toBeGreaterThan(first);
    expect(store.getState().office.pause.requestSeq).toBe(second);
  });

  it("resetPauseState clears the record and marks status unknown", () => {
    const store = makeStore();
    const tag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(tag, WS_1, WS_1, readSuccess(true, makeRecord()));
    store.getState().resetPauseState();
    expect(store.getState().office.pause.status).toBe("unknown");
    expect(store.getState().office.pause.record).toBeNull();
  });
});

describe("office pause store actions: read-success", () => {
  it("applies the record when the workspace matches and the tag is fresh", () => {
    const store = makeStore();
    const tag = store.getState().beginPauseRequest();
    const applied = store
      .getState()
      .applyPauseResponse(tag, WS_1, WS_1, readSuccess(true, makeRecord()));
    expect(applied).toBe(true);
    expect(store.getState().office.pause.status).toBe("known");
    expect(store.getState().office.pause.record?.id).toBe("pause-1");
  });

  it("with paused=false clears the record", () => {
    const store = makeStore();
    const tag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(tag, WS_1, WS_1, readSuccess(false));
    expect(store.getState().office.pause.status).toBe("known");
    expect(store.getState().office.pause.record).toBeNull();
  });

  it("for a workspace other than the active one is discarded (F51)", () => {
    const store = makeStore();
    const tag = store.getState().beginPauseRequest();
    const applied = store
      .getState()
      .applyPauseResponse(tag, WS_1, WS_2, readSuccess(true, makeRecord()));
    expect(applied).toBe(false);
    expect(store.getState().office.pause.status).toBe("unknown");
    expect(store.getState().office.pause.record).toBeNull();
  });

  it("a superseded (stale-tag) response is discarded", () => {
    const store = makeStore();
    const older = store.getState().beginPauseRequest();
    const newer = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(newer, WS_1, WS_1, readSuccess(false));
    const applied = store
      .getState()
      .applyPauseResponse(older, WS_1, WS_1, readSuccess(true, makeRecord()));
    expect(applied).toBe(false);
    // The newer (already-applied) response's outcome must survive.
    expect(store.getState().office.pause.record).toBeNull();
  });
});

describe("office pause store actions: read-failure and F51 guards", () => {
  it("sets status unknown but does not clear an existing record", () => {
    const store = makeStore();
    const readTag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(readTag, WS_1, WS_1, readSuccess(true, makeRecord()));
    const failTag = store.getState().beginPauseRequest();
    const applied = store
      .getState()
      .applyPauseResponse(failTag, WS_1, WS_1, { kind: "read-failure" });
    expect(applied).toBe(true);
    expect(store.getState().office.pause.status).toBe("unknown");
    expect(store.getState().office.pause.record?.id).toBe("pause-1");
  });

  // F51: the GET/POST failure rows carry the same two supersession guards
  // as the success rows — a stale or wrong-workspace failure must not
  // downgrade a freshly-applied `known` state back to `unknown`.
  it("F51: a wrong-workspace read-failure does not flip status to unknown", () => {
    const store = makeStore();
    const readTag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(readTag, WS_2, WS_2, readSuccess(false));
    const failTag = store.getState().beginPauseRequest();
    const applied = store
      .getState()
      .applyPauseResponse(failTag, WS_1, WS_2, { kind: "read-failure" });
    expect(applied).toBe(false);
    expect(store.getState().office.pause.status).toBe("known");
  });

  it("F51: a superseded (stale-tag) read-failure does not flip a later known status", () => {
    const store = makeStore();
    const olderFail = store.getState().beginPauseRequest();
    const newerSuccess = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(newerSuccess, WS_1, WS_1, readSuccess(true, makeRecord()));
    const applied = store
      .getState()
      .applyPauseResponse(olderFail, WS_1, WS_1, { kind: "read-failure" });
    expect(applied).toBe(false);
    expect(store.getState().office.pause.status).toBe("known");
    expect(store.getState().office.pause.record?.id).toBe("pause-1");
  });
});

describe("office pause store actions: mutate outcomes", () => {
  it("mutate-failure leaves status and record unchanged", () => {
    const store = makeStore();
    const readTag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(readTag, WS_1, WS_1, readSuccess(false));
    const failTag = store.getState().beginPauseRequest();
    const applied = store
      .getState()
      .applyPauseResponse(failTag, WS_1, WS_1, { kind: "mutate-failure" });
    expect(applied).toBe(true);
    expect(store.getState().office.pause.status).toBe("known");
    expect(store.getState().office.pause.record).toBeNull();
  });

  it("mutate-success (pause) sets a record; mutate-success (resume) clears it", () => {
    const store = makeStore();
    const pauseTag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(pauseTag, WS_1, WS_1, {
      kind: "mutate-success",
      paused: true,
      record: makeRecord(),
    });
    expect(store.getState().office.pause.record?.id).toBe("pause-1");

    const resumeTag = store.getState().beginPauseRequest();
    store.getState().applyPauseResponse(resumeTag, WS_1, WS_1, {
      kind: "mutate-success",
      paused: false,
      record: null,
    });
    expect(store.getState().office.pause.record).toBeNull();
    expect(store.getState().office.pause.status).toBe("known");
  });
});
