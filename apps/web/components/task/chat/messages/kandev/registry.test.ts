import { describe, expect, it } from "vitest";
import { getKandevRenderer } from "./registry";
import {
  ProposeMessageRenderer,
  ProposeMoveRenderer,
  ProposeResumeRenderer,
  ProposeTaskRenderer,
} from "./propose-task-renderer";

describe("getKandevRenderer proposal stems", () => {
  it("attaches the proposal card renderer to every propose tool", () => {
    expect(getKandevRenderer("propose_task")).toBe(ProposeTaskRenderer);
    expect(getKandevRenderer("propose_resume")).toBe(ProposeResumeRenderer);
    expect(getKandevRenderer("propose_message")).toBe(ProposeMessageRenderer);
    expect(getKandevRenderer("propose_move")).toBe(ProposeMoveRenderer);
  });
});
