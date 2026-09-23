import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskCreateAdvancedSettings } from "./task-create-dialog-advanced-settings";
import { TaskCreateParentWorkspaceSetting } from "./task-create-dialog-parent-workspace-setting";

const touchState = vi.hoisted(() => ({ enabled: false }));

afterEach(() => {
  touchState.enabled = false;
  cleanup();
  vi.clearAllMocks();
});

const ADVANCED_SETTINGS_TRIGGER_TEST_ID = "task-create-advanced-settings-trigger";
const DEPENDENCY_TRIGGER_TEST_ID = "task-create-dependencies-trigger";

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "task:advancedSettings": "Advanced settings",
        "task:dependsOn": "Depends on",
        "task:dependencyInfoLabel": "About task dependencies",
        "task:dependencyInfo": "This task waits until every selected task completes successfully.",
        "task:priorityInfoLabel": "About task priority",
        "task:priorityInfo": "Priority shows how urgent this task is on the board.",
        "task:initialWorkspaceLayoutInfoLabel": "About parent workspace folder",
        "task:initialWorkspaceLayoutInfo":
          "Start above the repository so you can add sibling repositories later without moving the agent's working directory. Some agents may discover repository instructions and skills differently with this layout.",
        "task:initialWorkspaceLayoutLabel": "Start in a parent workspace folder",
        "task:initialWorkspaceLayoutMultiple":
          "A parent workspace is already used for multiple repositories.",
        "task:initialWorkspaceLayoutTitle": "Parent workspace folder",
      })[key] ?? key,
  }),
}));

vi.mock("@/components/task-create-dialog-dependencies", () => ({
  TaskCreateDependencies: ({
    value,
    onChange,
  }: {
    value: string[];
    onChange: (next: string[]) => void;
  }) => (
    <button
      type="button"
      data-testid={DEPENDENCY_TRIGGER_TEST_ID}
      onClick={() => onChange([...value, "task-2"])}
    >
      {value.length === 0 ? "No dependency" : `${value.length} dependencies`}
    </button>
  ),
}));

function renderAdvancedSettings(
  overrides: Partial<React.ComponentProps<typeof TaskCreateAdvancedSettings>> = {},
) {
  return render(
    <TooltipProvider>
      <TaskCreateAdvancedSettings
        isCreateMode
        isTaskStarted={false}
        blockedBy={[]}
        onBlockedByChange={() => {}}
        priority="medium"
        onPriorityChange={() => {}}
        initialWorkspaceLayout="repository"
        onInitialWorkspaceLayoutChange={() => {}}
        initialWorkspaceLayoutMode="unavailable"
        {...overrides}
      />
    </TooltipProvider>,
  );
}

// eslint-disable-next-line max-lines-per-function -- advanced settings coverage shares one fixture.
describe("TaskCreateAdvancedSettings", () => {
  it("starts collapsed and keeps the dependency selector hidden", () => {
    renderAdvancedSettings();

    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.className).toContain("min-h-12");
    expect(screen.queryByTestId(DEPENDENCY_TRIGGER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-create-priority-select")).toBeNull();
  });

  it("reveals the dependency selector when expanded", () => {
    renderAdvancedSettings();

    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).getAttribute("hidden")).toBeNull();
    expect(screen.getByTestId("task-create-priority-select").getAttribute("hidden")).toBeNull();
  });

  it("labels the dependency setting and provides contextual help", async () => {
    renderAdvancedSettings();

    fireEvent.click(screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID));

    expect(screen.getByTestId("task-create-dependency-setting-label").textContent).toContain(
      "Depends on",
    );
    const grid = screen.getByTestId("task-create-advanced-settings-grid");
    const row = screen.getByTestId("task-create-dependency-setting-row");
    const priorityRow = screen.getByTestId("task-create-priority-setting-row");
    const selectorContainer = screen.getByTestId("task-create-dependency-selector-container");
    expect(grid.className).toContain("md:grid-cols-2");
    expect(row.className).toContain("items-center");
    expect(row.className).toContain("gap-3");
    expect(row.className).not.toContain("flex-col");
    expect(row.parentElement).toBe(grid);
    expect(grid.firstElementChild).toBe(row);
    expect(grid.lastElementChild).toBe(priorityRow);
    expect(priorityRow.className).toContain("md:col-start-2");
    expect(priorityRow.className).toContain("md:justify-self-start");
    expect(priorityRow.className).not.toContain("md:justify-self-end");
    expect(selectorContainer.parentElement).toBe(row);
    const info = screen.getByTestId("task-create-dependency-setting-info");
    expect(info.getAttribute("aria-label")).toBe("About task dependencies");

    fireEvent.focus(info);
    await waitFor(() => {
      expect(screen.getByRole("tooltip").textContent).toContain(
        "This task waits until every selected task completes successfully.",
      );
    });
  });

  it("offers the parent workspace choice with keyboard accessible desktop help", async () => {
    const onLayoutChange = vi.fn();
    renderAdvancedSettings({
      initialWorkspaceLayoutMode: "single-repository",
      onInitialWorkspaceLayoutChange: onLayoutChange,
    });

    fireEvent.click(screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID));

    const checkbox = screen.getByTestId("task-create-initial-workspace-layout-checkbox");
    expect(checkbox.getAttribute("data-state")).toBe("unchecked");
    fireEvent.click(checkbox);
    expect(onLayoutChange).toHaveBeenCalledWith("task_root");

    const info = screen.getByTestId("task-create-initial-workspace-layout-info");
    fireEvent.focus(info);
    expect((await screen.findByRole("tooltip")).textContent).toContain(
      "Start above the repository so you can add sibling repositories later",
    );
  });

  it("opens the same parent workspace explanation in a drawer for touch pointers", () => {
    touchState.enabled = true;
    renderAdvancedSettings({ initialWorkspaceLayoutMode: "single-repository" });

    fireEvent.click(screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID));
    const info = screen.getByTestId("task-create-initial-workspace-layout-info");
    expect(info.getAttribute("aria-haspopup")).toBe("dialog");
    fireEvent.click(info);

    expect(
      screen.getByTestId("task-create-initial-workspace-layout-help-drawer").textContent,
    ).toContain("Start above the repository so you can add sibling repositories later");
    expect(screen.getByRole("heading").textContent).toContain("Parent workspace folder");
  });

  it("explains the fixed parent layout for multiple repositories", () => {
    const onLayoutChange = vi.fn();
    renderAdvancedSettings({
      initialWorkspaceLayout: "repository",
      initialWorkspaceLayoutMode: "multiple-repositories",
      onInitialWorkspaceLayoutChange: onLayoutChange,
    });

    fireEvent.click(screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID));

    expect(
      screen.getByTestId("task-create-initial-workspace-layout-multiple").textContent,
    ).toContain("A parent workspace is already used for multiple repositories.");
    expect(screen.queryByTestId("task-create-initial-workspace-layout-checkbox")).toBeNull();
    expect(onLayoutChange).toHaveBeenCalledWith("task_root");
  });

  it("restores the repository layout after a forced multi-repository transition", async () => {
    function Harness() {
      const [layout, setLayout] = useState<"repository" | "task_root">("repository");
      const [mode, setMode] = useState<"multiple-repositories" | "single-repository">(
        "multiple-repositories",
      );
      return (
        <TooltipProvider>
          <button type="button" onClick={() => setMode("single-repository")}>
            Remove repository
          </button>
          <span data-testid="current-layout">{layout}</span>
          <TaskCreateParentWorkspaceSetting
            initialWorkspaceLayout={layout}
            onInitialWorkspaceLayoutChange={setLayout}
            initialWorkspaceLayoutMode={mode}
          />
        </TooltipProvider>
      );
    }

    render(<Harness />);
    await waitFor(() => expect(screen.getByTestId("current-layout").textContent).toBe("task_root"));
    fireEvent.click(screen.getByRole("button", { name: "Remove repository" }));
    await waitFor(() =>
      expect(screen.getByTestId("current-layout").textContent).toBe("repository"),
    );
  });

  it("preserves selected dependencies across collapse and reopen", () => {
    function Harness() {
      const [blockedBy, setBlockedBy] = useState<string[]>(["task-1"]);
      return (
        <TooltipProvider>
          <TaskCreateAdvancedSettings
            isCreateMode
            isTaskStarted={false}
            blockedBy={blockedBy}
            onBlockedByChange={setBlockedBy}
            priority="medium"
            onPriorityChange={() => {}}
            initialWorkspaceLayout="repository"
            onInitialWorkspaceLayoutChange={() => {}}
            initialWorkspaceLayoutMode="unavailable"
          />
        </TooltipProvider>
      );
    }

    render(<Harness />);
    const trigger = screen.getByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID);
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID));
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).textContent).toContain("2 dependencies");

    fireEvent.click(trigger);
    expect(screen.queryByTestId(DEPENDENCY_TRIGGER_TEST_ID)).toBeNull();
    fireEvent.click(trigger);
    expect(screen.getByTestId(DEPENDENCY_TRIGGER_TEST_ID).textContent).toContain("2 dependencies");
  });

  it.each([
    ["edit mode", { isCreateMode: false, isTaskStarted: false }],
    ["started task", { isCreateMode: true, isTaskStarted: true }],
  ])("does not render for a %s", (_name, props) => {
    renderAdvancedSettings(props);

    expect(screen.queryByTestId(ADVANCED_SETTINGS_TRIGGER_TEST_ID)).toBeNull();
  });
});
