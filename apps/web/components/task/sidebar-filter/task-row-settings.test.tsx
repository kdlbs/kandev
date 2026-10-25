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
const SETTINGS_TOGGLE_TEST_ID = "task-row-settings-toggle";

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
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));
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
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));
    fireEvent.click(screen.getByTestId("task-row-detail-toggle-repository"));

    expect(onChange).toHaveBeenCalledWith({
      ...DEFAULT_VALUE,
      visibleDetails: ["relative_time", "pull_request_number"],
    });
  });

  it("moves a task-row detail through More while preserving visibility", () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={{ ...DEFAULT_VALUE, visibleDetails: ["relative_time", "pull_request_number"] }}
        sort={{ key: "lastActivityAt", direction: "desc" }}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    fireEvent.pointerDown(screen.getByTestId("task-row-detail-more-pull_request_number"), {
      button: 0,
      pointerType: "mouse",
    });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("task-row-detail-more-pull_request_number-move-up"));

    expect(onChange).toHaveBeenCalledWith({
      ...DEFAULT_VALUE,
      detailOrder: ["relative_time", "pull_request_number", "repository"],
      visibleDetails: ["relative_time", "pull_request_number"],
    });
  });

  it("moves a task-row detail with the keyboard and preserves visibility", async () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={{ ...DEFAULT_VALUE, visibleDetails: ["relative_time", "pull_request_number"] }}
        sort={{ key: "lastActivityAt", direction: "desc" }}
        onChange={onChange}
      />,
    );
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    const handle = screen.getByTestId("task-row-detail-handle-repository");
    handle.focus();
    fireEvent.keyDown(handle, { key: " ", code: "Space" });
    await new Promise((resolve) => setTimeout(resolve, 0));
    fireEvent.keyDown(handle, { key: "ArrowUp", code: "ArrowUp" });
    fireEvent.keyDown(handle, { key: " ", code: "Space" });

    expect(onChange).toHaveBeenCalledWith({
      ...DEFAULT_VALUE,
      detailOrder: ["repository", "relative_time", "pull_request_number"],
      visibleDetails: ["relative_time", "pull_request_number"],
    });
  });
});

describe("TaskRowSettings presentation", () => {
  it("marks relative time as shown on the right when it is the trailing value", () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={{ ...DEFAULT_VALUE, trailing: "relative_time" }}
        sort={{ key: "state", direction: "asc" }}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    expect(screen.getByTestId("task-row-relative-time-shown-on-right").textContent).toBe(
      "Shown on right",
    );
  });

  it("keeps the mobile trailing selector touch-sized while matching compact desktop selectors", () => {
    const onChange = vi.fn();
    render(
      <TaskRowSettings
        value={DEFAULT_VALUE}
        sort={{ key: "state", direction: "asc" }}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    expect(screen.getByTestId("task-row-trailing-select").className).toContain("min-h-11");
    expect(screen.getByTestId("task-row-trailing-select").className).toContain("md:h-7");
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
