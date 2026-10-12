import { describe, expect, it } from "vitest";
import type { ControlDraft } from "./control-draft";
import {
  buildPutRequest,
  isWatchesInvalid,
  mergeStored,
  sameWatches,
  settleAfterSave,
  switchOffWatches,
} from "./control-draft";

const base = (): ControlDraft => ({
  actions: {
    create_task: "requires_approval",
    start_agent: "denied",
    message: "denied",
    move: "denied",
    resume: "denied",
    stop: "denied",
  },
  watches: { scope: "all", workflowIds: [] },
});

const withAction = (
  d: ControlDraft,
  a: keyof ControlDraft["actions"],
  v: "denied" | "requires_approval",
) => ({
  ...d,
  actions: { ...d.actions, [a]: v },
});

describe("buildPutRequest", () => {
  it("is empty when nothing differs", () => {
    expect(buildPutRequest(base(), base())).toEqual({});
  });

  it("sends only the policy, naming all six actions", () => {
    const req = buildPutRequest(withAction(base(), "message", "requires_approval"), base());
    expect(req.watches).toBeUndefined();
    expect(Object.keys(req.policy?.actions ?? {})).toHaveLength(6);
    expect(req.policy?.actions.message).toBe("requires_approval");
  });

  it("sends only watches when only they differ, without ids for all", () => {
    const stored = { ...base(), watches: { scope: "selected" as const, workflowIds: ["a"] } };
    const draft = { ...stored, watches: { scope: "all" as const, workflowIds: ["a"] } };
    expect(buildPutRequest(draft, stored)).toEqual({ watches: { scope: "all" } });
  });

  it("sends both members when both differ", () => {
    const draft = withAction(base(), "move", "requires_approval");
    draft.watches = { scope: "selected", workflowIds: ["a", "b"] };
    const req = buildPutRequest(draft, base());
    expect(req.policy).toBeDefined();
    expect(req.watches).toEqual({ scope: "selected", workflow_ids: ["a", "b"] });
  });

  it("does not send an unedited stored empty selected set", () => {
    const stored = { ...base(), watches: { scope: "selected" as const, workflowIds: [] } };
    const req = buildPutRequest(withAction(stored, "message", "requires_approval"), stored);
    expect(req.watches).toBeUndefined();
    expect(req.policy).toBeDefined();
  });
});

describe("isWatchesInvalid", () => {
  it("is valid for an unedited stored empty set and invalid once edited to empty", () => {
    const empty = { ...base(), watches: { scope: "selected" as const, workflowIds: [] } };
    expect(isWatchesInvalid(empty, empty)).toBe(false);
    const stored = { ...base(), watches: { scope: "selected" as const, workflowIds: ["a"] } };
    expect(
      isWatchesInvalid({ ...stored, watches: { scope: "selected", workflowIds: [] } }, stored),
    ).toBe(true);
  });
});

describe("sameWatches", () => {
  it("compares selected ids as a set and ignores ids for all", () => {
    expect(
      sameWatches(
        { scope: "selected", workflowIds: ["a", "b"] },
        { scope: "selected", workflowIds: ["b", "a"] },
      ),
    ).toBe(true);
    expect(
      sameWatches({ scope: "all", workflowIds: ["a"] }, { scope: "all", workflowIds: [] }),
    ).toBe(true);
  });
});

describe("mergeStored", () => {
  it("keeps an edited action and follows the new stored value for the others", () => {
    const old = base();
    const draft = withAction(old, "message", "requires_approval");
    const next = withAction(withAction(old, "move", "requires_approval"), "message", "denied");
    const merged = mergeStored(draft, old, next);
    expect(merged.actions.message).toBe("requires_approval");
    expect(merged.actions.move).toBe("requires_approval");
  });

  it("keeps a dirty Watches draft as one unit and follows stored when clean", () => {
    const old = base();
    const next = { ...old, watches: { scope: "selected" as const, workflowIds: ["x"] } };
    expect(mergeStored(old, old, next).watches).toEqual(next.watches);
    const dirty = { ...old, watches: { scope: "selected" as const, workflowIds: ["y"] } };
    expect(mergeStored(dirty, old, next).watches).toEqual(dirty.watches);
  });
});

describe("settleAfterSave", () => {
  it("keeps a member changed after the request was sent", () => {
    const sent = withAction(base(), "message", "requires_approval");
    const current = withAction(sent, "move", "requires_approval");
    const settled = settleAfterSave(current, sent, sent);
    expect(settled.actions.move).toBe("requires_approval");
    expect(settled.actions.message).toBe("requires_approval");
  });

  it("takes the response for members unchanged since sending", () => {
    const sent = withAction(base(), "message", "requires_approval");
    const response = withAction(sent, "resume", "requires_approval");
    expect(settleAfterSave(sent, sent, response).actions.resume).toBe("requires_approval");
  });
});

describe("switchOffWatches", () => {
  it("refuses a workspace with no board", () => {
    expect(switchOffWatches([])).toEqual({ ok: false, reason: "no-boards" });
  });

  it("starts from every board in order", () => {
    expect(switchOffWatches(["a", "b"])).toEqual({
      ok: true,
      watches: { scope: "selected", workflowIds: ["a", "b"] },
      capped: false,
    });
  });

  it("caps at 50 in workspace order", () => {
    const ids = Array.from({ length: 60 }, (_, i) => `w${i}`);
    const result = switchOffWatches(ids);
    expect(result.ok && result.watches.workflowIds).toEqual(ids.slice(0, 50));
    expect(result.ok && result.capped).toBe(true);
  });
});
