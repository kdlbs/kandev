import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { KanbanCardContextMenuItems } from "./kanban-card-menu-entry-renderers";
import { buildWorkflowMenuEntry } from "./kanban-card-menu-items";
const listWorkflowSteps = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("fetches one destination's metadata only when its submenu opens", async () => {
  listWorkflowSteps.mockResolvedValue({
    steps: [{ id: "target-step", name: "Incoming", position: 0 }],
  });
  const send = vi.fn();
  const entry = buildWorkflowMenuEntry({
    currentWorkflowId: "source",
    workflows: [
      { id: "source", name: "Source" },
      { id: "target", name: "Target" },
      { id: "other", name: "Other" },
    ],
    stepsByWorkflowId: {},
    disabled: false,
    isBulkSelection: true,
    onSendToWorkflow: send,
  });
  if (!entry) throw new Error("missing workflow menu");
  render(
    <ContextMenu>
      <ContextMenuTrigger data-testid="trigger">Task</ContextMenuTrigger>
      <ContextMenuContent>
        <KanbanCardContextMenuItems entries={[entry]} />
      </ContextMenuContent>
    </ContextMenu>,
  );
  fireEvent.contextMenu(screen.getByTestId("trigger"));
  fireEvent.pointerMove(screen.getByTestId("task-context-change-workflow-selection"), {
    pointerType: "mouse",
  });
  const target = await screen.findByTestId("task-context-workflow-target");
  expect(listWorkflowSteps).not.toHaveBeenCalled();
  fireEvent.pointerMove(target, { pointerType: "mouse" });
  const step = await screen.findByTestId("task-context-step-target-step");
  expect(listWorkflowSteps).toHaveBeenCalledExactlyOnceWith("target", {
    cache: "no-store",
    init: { signal: expect.any(AbortSignal) },
  });
  expect(listWorkflowSteps.mock.calls[0][1].init.signal.aborted).toBe(false);
  fireEvent.click(step);
  expect(send).toHaveBeenCalledExactlyOnceWith("target", "target-step");
  expect(listWorkflowSteps.mock.calls[0][1].init.signal.aborted).toBe(true);
});
