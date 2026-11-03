import { Command, CommandList } from "@kandev/ui/command";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useInlineTaskSearchEffect } from "@/hooks/use-command-panel-task-results";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import { CommandsListContent } from "./command-panel-results";

const listTasksByWorkspace = vi.fn();
vi.mock("@/lib/api", () => ({
  listTasksByWorkspace: (...args: unknown[]) => listTasksByWorkspace(...args),
  searchTaskConversations: vi.fn(),
}));

const steps = [
  { id: "hidden", position: 0, show_in_command_panel: false },
  { id: "visible", position: 1, show_in_command_panel: true },
];

function task(id: string, step: string): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("workspace"),
    workflow_id: workflowId("workflow"),
    workflow_step_id: step,
    position: 0,
    title: id,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function Panel({ onSelect }: { onSelect: (task: Task) => void }) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [searching, setSearching] = useState(false);
  useInlineTaskSearchEffect({
    mode: "commands",
    search: "",
    open: true,
    workspaceId: "workspace",
    ...{ workflowId: "workflow" },
    steps,
    setTaskResults: setTasks,
    setIsSearching: setSearching,
  });
  return (
    <Command shouldFilter={false}>
      <CommandList>
        <CommandsListContent
          commands={[]}
          grouped={[]}
          search=""
          onSelect={() => {}}
          taskResults={tasks}
          isSearching={searching}
          stepMap={new Map()}
          repoMap={new Map()}
          liveTasksById={new Map()}
          lastStepIdByWorkflowId={new Map()}
          onTaskSelect={onSelect}
        />
      </CommandList>
    </Command>
  );
}

afterEach(() => {
  cleanup();
  listTasksByWorkspace.mockReset();
});

describe("idle command-panel task discovery", () => {
  // @covers AC-TASKS-WORKSPACE-SEARCH-002.1
  for (const hiddenCount of [0, 20]) {
    it(`opens an eligible task with ${hiddenCount} newer hidden rows`, async () => {
      const selected = task("Eligible running task", "visible");
      const rows = [
        ...Array.from({ length: hiddenCount }, (_, i) => task(`hidden-${i}`, "hidden")),
        selected,
      ];
      listTasksByWorkspace.mockImplementation(async (_workspace, params) => {
        const eligible = params.commandPanelVisible
          ? rows.filter((row) => row.workflow_step_id === "visible")
          : rows;
        const start = (params.page - 1) * params.pageSize;
        return { tasks: eligible.slice(start, start + params.pageSize), total: eligible.length };
      });
      const navigate = vi.fn();
      render(<Panel onSelect={navigate} />);
      await waitFor(() =>
        expect(screen.getByRole("option", { name: /Eligible running task/ })).toBeTruthy(),
      );
      fireEvent.click(screen.getByRole("option", { name: /Eligible running task/ }));
      expect(navigate).toHaveBeenCalledWith(selected);
    });
  }
});
