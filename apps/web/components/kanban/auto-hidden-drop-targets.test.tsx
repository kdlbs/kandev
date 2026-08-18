import { cleanup, render, screen } from "@testing-library/react";
import { DndContext } from "@dnd-kit/core";
import { afterEach, describe, expect, it } from "vitest";
import { DesktopAutoHiddenDropTargets } from "./mobile-drop-targets";
import type { WorkflowStep } from "@/components/kanban-column";

afterEach(cleanup);

const HIDDEN_STEP = {
  id: "step-hidden",
  title: "Hidden QA",
  color: "#3b82f6",
} as WorkflowStep;

describe("DesktopAutoHiddenDropTargets", () => {
  it("renders hidden destinations in an overlay only while dragging", () => {
    const { rerender } = render(
      <DndContext>
        <DesktopAutoHiddenDropTargets steps={[HIDDEN_STEP]} isDragging={false} />
      </DndContext>,
    );
    expect(screen.queryByTestId("desktop-auto-hidden-drop-targets")).toBeNull();

    rerender(
      <DndContext>
        <DesktopAutoHiddenDropTargets steps={[HIDDEN_STEP]} isDragging />
      </DndContext>,
    );
    expect(screen.getByTestId("desktop-auto-hidden-drop-targets")).toBeTruthy();
    expect(screen.getByTestId("auto-hidden-drop-target-step-hidden").textContent).toContain(
      "Hidden QA",
    );
  });
});
