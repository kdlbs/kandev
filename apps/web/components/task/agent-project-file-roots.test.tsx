import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentProjectTaskProvider } from "./agent-project-task-context";
import type { AgentProjectTaskContextValue } from "./agent-project-task-context";

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("./agent-project-context-panel", () => ({
  AgentProjectContextPanel: () => <div>Project context</div>,
}));

import { AgentProjectFileRoots } from "./agent-project-file-roots";

const taskContext = (taskId: string, tier: AgentProjectTaskContextValue["tier"]) => ({
  taskId,
  workspaceId: "ws-1",
  projectId: "project-1",
  tier,
});

function renderRoots(value: AgentProjectTaskContextValue) {
  return render(
    <AgentProjectTaskProvider value={value}>
      <AgentProjectFileRoots>
        <div>Repository files</div>
      </AgentProjectFileRoots>
    </AgentProjectTaskProvider>,
  );
}

describe("AgentProjectFileRoots", () => {
  it("resets the selected root to the new task tier during task navigation", () => {
    const view = renderRoots(taskContext("coordinator-1", "coordinator"));
    expect(screen.getByRole("tab", { name: "Context" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.click(screen.getByRole("tab", { name: "Workspace" }));

    view.rerender(
      <AgentProjectTaskProvider value={taskContext("worker-1", "economy")}>
        <AgentProjectFileRoots>
          <div>Repository files</div>
        </AgentProjectFileRoots>
      </AgentProjectTaskProvider>,
    );
    expect(screen.getByRole("tab", { name: "Workspace" }).getAttribute("aria-selected")).toBe(
      "true",
    );

    view.rerender(
      <AgentProjectTaskProvider value={taskContext("coordinator-2", "coordinator")}>
        <AgentProjectFileRoots>
          <div>Repository files</div>
        </AgentProjectFileRoots>
      </AgentProjectTaskProvider>,
    );
    expect(screen.getByRole("tab", { name: "Context" }).getAttribute("aria-selected")).toBe("true");
  });
});
