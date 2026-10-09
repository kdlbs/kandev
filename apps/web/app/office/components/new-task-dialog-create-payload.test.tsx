import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

import { NewTaskDialog } from "./new-task-dialog";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";

const { createTask } = vi.hoisted(() => ({
  createTask: vi.fn().mockResolvedValue({ id: "task-created" }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({ createTask }));

const WORKSPACE_ID = "workspace-assignee-contract";

function Harness() {
  const store = useAppStoreApi();
  store.getState().setActiveWorkspace(WORKSPACE_ID);
  return (
    <NewTaskDialog
      open
      onOpenChange={vi.fn()}
      defaultProjectId="proj-1"
      defaultAssigneeId="agent-office-1"
    />
  );
}

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  cleanup();
  createTask.mockClear();
});

// TestBetaOfficeCreateContract's frontend half: ISSUE-7 was the dialog
// sending the picked assignee inside metadata.assignee_agent_profile_id,
// a field the backend never read. This pins the fixed shape — a top-level
// field on the create payload, matching kanban-api.ts's createTask type —
// so a future regression back into metadata fails loudly here rather than
// silently dropping every Office task's assignee again.
describe("NewTaskDialog create payload", () => {
  it("sends assignee_agent_profile_id as a top-level field, not inside metadata", async () => {
    render(
      <StateProvider>
        <TooltipProvider>
          <Harness />
        </TooltipProvider>
      </StateProvider>,
    );

    fireEvent.change(screen.getByPlaceholderText("Task title"), {
      target: { value: "Ship the fix" },
    });
    fireEvent.click(screen.getByTestId("new-task-create-button"));

    await waitFor(() => expect(createTask).toHaveBeenCalledTimes(1));
    const payload = createTask.mock.calls[0][0];
    expect(payload.assignee_agent_profile_id).toBe("agent-office-1");
    expect(payload.metadata?.assignee_agent_profile_id).toBeUndefined();
    expect(payload.metadata?.execution_policy).toBeUndefined();
  });
});
