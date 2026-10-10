import { beforeEach, describe, expect, it } from "vitest";
import { createCopilotStore, useCopilotStore } from "./copilot-store";
import { createWorkspaceCopilotStore } from "./workspace-copilot-store";

let store: ReturnType<typeof createWorkspaceCopilotStore>;

beforeEach(() => {
  store = createWorkspaceCopilotStore();
});

describe("createCopilotStore", () => {
  it("makes independent instances of the phase-1 slot", () => {
    const a = createCopilotStore();
    const b = createCopilotStore();
    a.getState().setOpen("c-1", true);
    expect(a.getState().open).toBe(true);
    expect(b.getState().open).toBe(false);
    expect(useCopilotStore.getState().open).toBe(false);
  });
});

describe("workspace copilot store panel ownership", () => {
  it("opens the copilot and claims the right panel in one write", () => {
    const seen: Array<[boolean, string | null]> = [];
    store.subscribe((s) => seen.push([s.open, s.rightPanel]));
    store.getState().openFor("ws-1", "c-1");
    expect(store.getState()).toMatchObject({
      open: true,
      rightPanel: "copilot",
      workspaceId: "ws-1",
      coordinatorId: "c-1",
    });
    expect(seen).toEqual([[true, "copilot"]]);
  });

  it("releases the claim when the slot closes, so no stale claim remains", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().setOpen("c-1", false);
    expect(store.getState()).toMatchObject({ open: false, rightPanel: null });
  });

  it("a removed slot entry also releases the claim", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().removeEntry("c-1");
    expect(store.getState()).toMatchObject({ open: false, rightPanel: null });
  });

  it("opening a preview closes the panel and takes the claim", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().claimPreview();
    expect(store.getState()).toMatchObject({ open: false, rightPanel: "preview" });
  });

  it("opening the copilot while a preview holds the claim takes it over", () => {
    store.getState().claimPreview();
    store.getState().openFor("ws-1", "c-1");
    expect(store.getState().rightPanel).toBe("copilot");
  });

  it("closing the copilot never overwrites a preview claim", () => {
    store.getState().claimPreview();
    store.getState().setOpen("c-1", false);
    expect(store.getState().rightPanel).toBe("preview");
    store.getState().reset();
    expect(store.getState().rightPanel).toBe("preview");
  });

  it("releasePreview nulls only while the preview holds the claim", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().releasePreview();
    expect(store.getState().rightPanel).toBe("copilot");
    store.getState().claimPreview();
    store.getState().releasePreview();
    expect(store.getState().rightPanel).toBeNull();
  });

  it("a slot write that does not touch open leaves the claim alone", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().markDraftsSwept("c-1");
    expect(store.getState()).toMatchObject({ open: true, rightPanel: "copilot" });
  });
});

describe("workspace copilot store reset and switch", () => {
  it("reset returns to the initial slot and forgets typed-text sweep, dismissal and workspace", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().markDraftsSwept("c-1");
    store.getState().dismissChip("kanban:wf-1");
    store.getState().reset();
    expect(store.getState()).toMatchObject({
      open: false,
      coordinatorId: null,
      draftsSwept: false,
      workspaceId: null,
      chipDismissedFor: null,
      rightPanel: null,
    });
  });

  it("a plain close keeps the slot, so reopening finds the typed-text sweep done", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().markDraftsSwept("c-1");
    store.getState().setOpen("c-1", false);
    store.getState().openFor("ws-1", "c-1");
    expect(store.getState().draftsSwept).toBe(true);
  });

  it("switchTo gives the new coordinator a fresh slot and keeps the workspace and dismissal", () => {
    store.getState().openFor("ws-1", "c-1");
    store.getState().markDraftsSwept("c-1");
    store.getState().dismissChip("task:t-1");
    store.getState().switchTo("c-2");
    expect(store.getState()).toMatchObject({
      coordinatorId: "c-2",
      open: true,
      rightPanel: "copilot",
      draftsSwept: false,
      workspaceId: "ws-1",
      chipDismissedFor: "task:t-1",
    });
  });

  it("dismissChip is cleared by clearDismissalUnless when the route key differs", () => {
    store.getState().dismissChip("task:t-1");
    store.getState().clearDismissalUnless("task:t-1");
    expect(store.getState().chipDismissedFor).toBe("task:t-1");
    store.getState().clearDismissalUnless("task:t-2");
    expect(store.getState().chipDismissedFor).toBeNull();
  });
});
