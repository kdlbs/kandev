import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProject } from "@/lib/types/http-agent-projects";

const mocks = vi.hoisted(() => ({ remove: vi.fn() }));

vi.mock("@/hooks/domains/agent-projects/use-agent-projects", () => ({
  projectWorkers: () => [],
  useAgentProjectMutations: () => ({ remove: mocks.remove }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

import { AgentProjectActionDialog } from "./agent-project-action-dialog";

const project = {
  id: "project-1",
  name: "Release migration",
  main_task_id: "coordinator-1",
  tasks: [],
} as unknown as AgentProject;

describe("AgentProjectActionDialog", () => {
  beforeEach(() => mocks.remove.mockReset().mockResolvedValue(undefined));

  it("sends explicit consent to discard project worktree changes", async () => {
    render(
      <AgentProjectActionDialog
        open
        onOpenChange={vi.fn()}
        workspaceId="ws-1"
        project={project}
        action="delete"
      />,
    );

    fireEvent.click(
      screen.getByRole("checkbox", { name: /permanently discard tracked and untracked changes/i }),
    );
    fireEvent.click(screen.getByTestId("agent-project-delete-confirm"));

    await waitFor(() => {
      expect(mocks.remove).toHaveBeenCalledWith("ws-1", "project-1", false, true);
    });
  });
});
