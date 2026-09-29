import { beforeEach, describe, expect, it } from "vitest";
import { useCopilotStore, INITIAL_COPILOT_ENTRY } from "./copilot-store";

function reset() {
  useCopilotStore.setState({
    coordinatorId: null,
    open: false,
    chip: null,
    draft: "",
    draftsSwept: false,
  });
}

const WHY_KAN_418 = "Why is KAN-418 here?";

describe("copilot store", () => {
  beforeEach(reset);

  it("returns the initial entry for a coordinator never touched", () => {
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual(INITIAL_COPILOT_ENTRY);
  });

  it("askAboutThis sets the chip, draft and opens the popover", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: { id: "KAN-418", label: "KAN-418" },
      draft: WHY_KAN_418,
    });
  });

  it("a second askAboutThis call replaces the chip and draft again", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-9", "Why is KAN-9 here?");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: { id: "KAN-9", label: "KAN-9" },
      draft: "Why is KAN-9 here?",
    });
  });

  it("reads as the initial entry for a coordinator the slot does not belong to", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    expect(useCopilotStore.getState().getEntry("coord-2")).toEqual(INITIAL_COPILOT_ENTRY);
  });

  it("clearDraft resets the draft to empty once it has been applied", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().clearDraft("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: { id: "KAN-418", label: "KAN-418" },
      draft: "",
    });
  });

  it("setOpen updates only the open flag", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().setOpen("coord-1", false);
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: false,
      chip: { id: "KAN-418", label: "KAN-418" },
      draft: WHY_KAN_418,
    });
  });

  it("setOpen(true) opens the launcher's own entry with no chip", () => {
    useCopilotStore.getState().setOpen("coord-1", true);
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: null,
      draft: "",
    });
  });

  it("removeChip clears the chip and leaves the draft as-is", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().removeChip("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: null,
      draft: WHY_KAN_418,
    });
  });

  it("clearChipAndDraft clears both but keeps open unchanged (a 404 while open)", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().clearChipAndDraft("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: true,
      chip: null,
      draft: "",
    });
  });

  it("clearChipAndDraft keeps open=false unchanged (a 404 while closed)", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().setOpen("coord-1", false);
    useCopilotStore.getState().clearChipAndDraft("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual({
      open: false,
      chip: null,
      draft: "",
    });
  });

  it("removeEntry resets the coordinator back to the initial entry", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().removeEntry("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual(INITIAL_COPILOT_ENTRY);
  });
});

describe("copilot store slot ownership", () => {
  beforeEach(reset);

  it("acting for another coordinator replaces the slot instead of keeping both", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().setOpen("coord-2", true);
    expect(useCopilotStore.getState().getEntry("coord-2")).toEqual({
      open: true,
      chip: null,
      draft: "",
    });
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual(INITIAL_COPILOT_ENTRY);
  });

  it("keepOnlyFor keeps the slot for the same coordinator and resets it otherwise", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().keepOnlyFor("coord-1");
    expect(useCopilotStore.getState().getEntry("coord-1").chip?.id).toBe("KAN-418");
    useCopilotStore.getState().keepOnlyFor("coord-2");
    expect(useCopilotStore.getState().coordinatorId).toBeNull();
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual(INITIAL_COPILOT_ENTRY);
  });

  it("keepOnlyFor(null) resets the slot on a non-coordinator path", () => {
    useCopilotStore.getState().askAboutThis("coord-1", "KAN-418", WHY_KAN_418);
    useCopilotStore.getState().markDraftsSwept("coord-1");
    useCopilotStore.getState().keepOnlyFor(null);
    expect(useCopilotStore.getState().getEntry("coord-1")).toEqual(INITIAL_COPILOT_ENTRY);
    expect(useCopilotStore.getState().draftsSwept).toBe(false);
  });

  it("markDraftsSwept is scoped to the slot's coordinator", () => {
    useCopilotStore.getState().setOpen("coord-1", true);
    useCopilotStore.getState().markDraftsSwept("coord-1");
    expect(useCopilotStore.getState().draftsSwept).toBe(true);
    useCopilotStore.getState().setOpen("coord-2", true);
    expect(useCopilotStore.getState().draftsSwept).toBe(false);
  });
});
