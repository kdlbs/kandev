import { describe, expect, it } from "vitest";
import type { ControlDraft, ProjectsDraft } from "./control-draft";
import {
  buildPutRequest,
  isProjectsInvalid,
  isWatchesInvalid,
  sameProjects,
  switchOffProjects,
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
  projects: null,
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

describe("Projects draft", () => {
  const selected = (ids: string[], includeNoRepository = false): ProjectsDraft => ({
    scope: "selected",
    entries: ids.map((id) => ({ kind: "repository" as const, id })),
    includeNoRepository,
  });
  const withProjects = (projects: ProjectsDraft | null): ControlDraft => ({ ...base(), projects });

  it("compares selected entries as a set and the toggle, and ignores both under all", () => {
    expect(sameProjects(selected(["a", "b"]), selected(["b", "a"]))).toBe(true);
    expect(sameProjects(selected(["a"]), selected(["a"], true))).toBe(false);
    expect(
      sameProjects(
        { ...selected(["a"]), scope: "all" },
        { ...selected(["b"], true), scope: "all" },
      ),
    ).toBe(true);
    expect(sameProjects({ ...selected(["a"]), scope: "all" }, selected(["a"]))).toBe(false);
    expect(sameProjects(null, null)).toBe(true);
  });

  it("sends the projects member only when it changed", () => {
    const stored = withProjects(selected(["a"]));
    expect(buildPutRequest(stored, stored)).toEqual({});
    expect(buildPutRequest(withProjects(selected(["a", "b"], true)), stored)).toEqual({
      projects: {
        scope: "selected",
        entries: [
          { kind: "repository", id: "a" },
          { kind: "repository", id: "b" },
        ],
        include_no_repository: true,
      },
    });
    expect(buildPutRequest(withProjects({ ...selected(["a"]), scope: "all" }), stored)).toEqual({
      projects: { scope: "all" },
    });
  });

  it("never sends projects while the scope is not offered", () => {
    expect(buildPutRequest(withProjects(null), withProjects(null))).toEqual({});
  });

  it("marks an edited selection with no entry and no toggle invalid", () => {
    const stored = withProjects(selected(["a"]));
    expect(isProjectsInvalid(withProjects(selected([])), stored)).toBe(true);
    expect(isProjectsInvalid(withProjects(selected([], true)), stored)).toBe(false);
    expect(isProjectsInvalid(stored, stored)).toBe(false);
  });

  it("keeps an edited projects member through a re-read and a save", () => {
    const old = withProjects(selected(["a"]));
    const next = withProjects(selected(["z"]));
    const dirty = withProjects(selected(["a", "b"]));
    expect(mergeStored(old, old, next).projects).toEqual(next.projects);
    expect(mergeStored(dirty, old, next).projects).toEqual(dirty.projects);
    expect(settleAfterSave(dirty, old, next).projects).toEqual(dirty.projects);
    expect(settleAfterSave(old, old, next).projects).toEqual(next.projects);
  });

  it("restores stored entries when switching off, else starts from every project capped at 50", () => {
    const all: ProjectsDraft = {
      scope: "all",
      entries: [{ kind: "repository", id: "a" }],
      includeNoRepository: false,
    };
    expect(switchOffProjects(all, [])?.entries).toEqual([{ kind: "repository", id: "a" }]);
    const none: ProjectsDraft = { scope: "all", entries: [], includeNoRepository: false };
    expect(switchOffProjects(none, [])).toBeNull();
    const choices = Array.from({ length: 60 }, (_, i) => ({
      kind: "repository" as const,
      id: `r${i}`,
      name: `r${i}`,
    }));
    const started = switchOffProjects(none, choices);
    expect(started?.scope).toBe("selected");
    expect(started?.entries).toHaveLength(50);
    expect(started?.includeNoRepository).toBe(false);
  });
});
