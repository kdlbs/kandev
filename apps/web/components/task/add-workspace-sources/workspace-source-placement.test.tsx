import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceSourcePlacement } from "./workspace-source-placement";
import { repositoryId, taskId } from "@/lib/types/ids";

afterEach(() => cleanup());

describe("WorkspaceSourcePlacement", () => {
  it("requires a placement before showing a selected result", () => {
    render(<WorkspaceSourcePlacement onPlacementChange={vi.fn()} />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Choose where to add the repositories.",
    );
    expect(screen.queryByTestId("workspace-source-placement-preview")).toBeNull();
  });

  it("renders the server preview and prevents unsupported expansion", () => {
    const onPlacementChange = vi.fn();
    render(
      <WorkspaceSourcePlacement
        placement="kandev_directory"
        onPlacementChange={onPlacementChange}
        preview={{
          task_id: taskId("task-1"),
          revision: "rev-1",
          workspace_path: "/tasks/task-1/repo",
          placement: "kandev_directory",
          sources: [
            {
              repository_id: repositoryId("repo-1"),
              repository_name: "payments",
              workspace_relative_path: "repo/kandev/payments",
            },
          ],
          supported_placements: [
            { placement: "kandev_directory", enabled: true },
            { placement: "current_root", enabled: true },
            {
              placement: "expand_root",
              enabled: false,
              reason: "This option needs explicit session recovery.",
            },
          ],
        }}
      />,
    );

    expect(screen.getByTestId("workspace-source-placement-preview").textContent).toContain(
      "repo/kandev/payments",
    );
    const radios = screen.getAllByRole("radio") as HTMLButtonElement[];
    expect(radios).toHaveLength(3);
    expect(radios[2]?.disabled).toBe(true);

    fireEvent.click(radios[1]!);
    expect(onPlacementChange).toHaveBeenCalledWith("current_root");
  });

  it("uses server layout capabilities for an existing parent workspace", () => {
    const onPlacementChange = vi.fn();
    render(
      <WorkspaceSourcePlacement
        placement="kandev_directory"
        onPlacementChange={onPlacementChange}
        preview={{
          task_id: taskId("task-1"),
          revision: "rev-1",
          workspace_path: "/tasks/task-1",
          placement: "kandev_directory",
          sources: [],
          supported_placements: [
            {
              placement: "kandev_directory",
              enabled: false,
              reason: "The task already uses the parent workspace layout.",
            },
            { placement: "current_root", enabled: true },
            {
              placement: "expand_root",
              enabled: false,
              reason: "This option needs explicit session recovery.",
            },
          ],
        }}
      />,
    );

    const radios = screen.getAllByRole("radio") as HTMLButtonElement[];
    expect(radios[0]?.disabled).toBe(true);
    expect(radios[1]?.disabled).toBe(false);
    expect(radios[2]?.disabled).toBe(true);
    expect(screen.getByText("The task already uses the parent workspace layout.")).toBeTruthy();
    fireEvent.click(radios[0]!);
    expect(onPlacementChange).not.toHaveBeenCalled();
    fireEvent.click(radios[1]!);
    expect(onPlacementChange).toHaveBeenCalledWith("current_root");
  });
});
