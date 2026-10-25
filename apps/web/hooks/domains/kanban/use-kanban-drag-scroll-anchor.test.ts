import { describe, expect, it } from "vitest";
import {
  getDragDisplaySteps,
  getPreservedScrollLeft,
  getRenderedStepKey,
  getTemporaryStepIds,
} from "./use-kanban-drag-scroll-anchor";
import type { WorkflowStep } from "@/components/kanban-column";

describe("getPreservedScrollLeft", () => {
  it("keeps the source at the same viewport position when columns expand", () => {
    expect(getPreservedScrollLeft(840, 72, 352)).toBe(1120);
  });

  it("preserves the drag-time scroll when the source column disappears after drop", () => {
    expect(getPreservedScrollLeft(1400, 72, undefined)).toBe(1400);
  });
});

describe("getRenderedStepKey", () => {
  it("is stable when a render recreates the same ordered steps", () => {
    const first = [{ id: "todo" }, { id: "done" }] as WorkflowStep[];
    const second = [{ id: "todo" }, { id: "done" }] as WorkflowStep[];

    expect(getRenderedStepKey(first)).toBe(getRenderedStepKey(second));
  });
});

describe("getDragDisplaySteps", () => {
  const visible = [{ id: "review", title: "Review" }] as WorkflowStep[];
  const targets = [
    { id: "backlog", title: "Backlog" },
    { id: "review", title: "Review" },
  ] as WorkflowStep[];

  it("reveals every real target during desktop drag", () => {
    expect(getDragDisplaySteps(visible, targets, true, false)).toBe(targets);
  });

  it("keeps a display-only orphan sentinel after every real drag target", () => {
    const orphan = { id: "__orphan__", title: "Needs Reassignment" } as WorkflowStep;

    expect(getDragDisplaySteps([...visible, orphan], targets, true, false)).toEqual([
      ...targets,
      orphan,
    ]);
  });

  it("keeps mobile columns stable because mobile has separate drop targets", () => {
    expect(getDragDisplaySteps(visible, targets, true, true)).toBe(visible);
  });

  it("marks only temporarily revealed real steps", () => {
    expect([...getTemporaryStepIds(visible, targets)]).toEqual(["backlog"]);
  });
});
