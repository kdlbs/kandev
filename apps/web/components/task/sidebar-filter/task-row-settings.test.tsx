import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarTaskRowPresentation } from "@/lib/state/slices/ui/sidebar-task-row-presentation";
import { TaskRowSettings, reorderSidebarTaskRowDetails } from "./task-row-settings";

const DEFAULT_VALUE: SidebarTaskRowPresentation = {
  detailsEnabled: true,
  detailOrder: ["relative_time", "repository", "pull_request_number"],
  visibleDetails: ["relative_time", "repository", "pull_request_number"],
  trailing: "git_changes",
};

afterEach(cleanup);

describe("TaskRowSettings", () => {
  it("starts collapsed and does not create a draft while opening", () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={DEFAULT_VALUE}
        sort={{ key: "state", direction: "asc" }}
        onChange={onChange}
      />,
    );

    expect(screen.queryByTestId("task-row-details-toggle")).toBeNull();
    fireEvent.click(screen.getByTestId("task-row-settings-toggle"));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByTestId("task-row-details-toggle")).toBeTruthy();
  });

  it("updates detail visibility without changing the saved order", () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={DEFAULT_VALUE}
        sort={{ key: "lastActivityAt", direction: "desc" }}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByTestId("task-row-settings-toggle"));
    fireEvent.click(screen.getByTestId("task-row-detail-toggle-repository"));

    expect(onChange).toHaveBeenCalledWith({
      ...DEFAULT_VALUE,
      visibleDetails: ["relative_time", "pull_request_number"],
    });
  });

  it("reorders fields with the same stable keys used by the drag handles", () => {
    expect(
      reorderSidebarTaskRowDetails(
        ["relative_time", "repository", "pull_request_number"],
        "pull_request_number",
        "relative_time",
      ),
    ).toEqual(["pull_request_number", "relative_time", "repository"]);
  });
});
