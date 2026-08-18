import { describe, expect, it } from "vitest";
import {
  getDragDisplaySteps,
  getPreservedScrollLeft,
  getTemporaryStepIds,
} from "./use-kanban-drag-scroll-anchor";
import type { WorkflowStep } from "@/components/kanban-column";

describe("getPreservedScrollLeft", () => {
  it("keeps the source at the same viewport position when columns expand", () => {
    expect(getPreservedScrollLeft(840, 72, 352, 840)).toBe(1120);
  });

  it("restores the original scroll when the source column disappears", () => {
    expect(getPreservedScrollLeft(1120, 72, undefined, 840)).toBe(840);
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

  it("keeps mobile columns stable because mobile has separate drop targets", () => {
    expect(getDragDisplaySteps(visible, targets, true, true)).toBe(visible);
  });

  it("marks only temporarily revealed real steps", () => {
    expect([...getTemporaryStepIds(visible, targets)]).toEqual(["backlog"]);
  });
});
