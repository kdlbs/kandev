import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";

const mocks = vi.hoisted(() => ({
  value: { enabled: false, rules: [] } as SidebarTaskColorAutomation,
  update: vi.fn(),
}));

vi.mock("./use-sidebar-task-color-automation", () => ({
  useSidebarTaskColorAutomation: () => ({
    value: mocks.value,
    update: mocks.update,
    saving: false,
    error: null,
  }),
}));

vi.mock("./task-color-rule-options", () => ({
  taskColorDimensionLabelKey: (dimension: string) =>
    ({
      workflow_step: "task:automaticColorsDimensionWorkflowStep",
      repository: "task:automaticColorsDimensionRepository",
      workflow: "task:automaticColorsDimensionWorkflow",
      executor_profile: "task:automaticColorsDimensionExecutorProfile",
      task_state: "task:automaticColorsDimensionTaskState",
      priority: "task:automaticColorsDimensionPriority",
      origin: "task:automaticColorsDimensionOrigin",
    })[dimension] ?? `task:${dimension}`,
  taskColorRuleOptionKey: (value: unknown) => JSON.stringify(value),
  useTaskColorRuleOptions: () => ({
    workflow_step: [],
    repository: [],
    workflow: [],
    executor_profile: [],
    task_state: [{ key: JSON.stringify("TODO"), value: "TODO", label: "To do", available: true }],
    priority: [],
    origin: [],
  }),
}));

vi.mock("./use-repository-rule-catalog", () => ({
  useRepositoryRuleCatalog: () => ({
    options: [],
    query: "",
    setQuery: vi.fn(),
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "workspace-1" },
      kanbanMulti: { snapshots: {} },
      workflows: { items: [] },
      executors: { items: [] },
    }),
}));

import { AutomaticColorSettings } from "./automatic-color-settings";

const SETTINGS_TOGGLE_TEST_ID = "automatic-color-settings-toggle";

function renderSettings(isDrawerLayout = false) {
  return render(
    <TooltipProvider>
      <AutomaticColorSettings isDrawerLayout={isDrawerLayout} />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.value = { enabled: false, rules: [] };
});

describe("AutomaticColorSettings timing guidance", () => {
  it("keeps scope and timing guidance behind a focusable info tooltip", () => {
    const { rerender } = renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    expect(screen.getByTestId("automatic-colors-help").getAttribute("aria-label")).toBe(
      "When automatic colors apply",
    );
    expect(screen.queryByTestId("automatic-colors-timing")).toBeNull();
    expect(
      screen.queryByText("Personal setting. Applies across sidebar views and workspaces.", {
        exact: true,
      }),
    ).toBeNull();
    expect(
      screen.queryByText("The first matching rule wins. Manual colors remain as fallback.", {
        exact: true,
      }),
    ).toBeNull();

    rerender(
      <TooltipProvider>
        <AutomaticColorSettings isDrawerLayout />
      </TooltipProvider>,
    );
    expect(screen.getByTestId("automatic-colors-help")).toBeTruthy();
    expect(screen.queryByTestId("automatic-colors-timing")).toBeNull();
  });
});

describe("AutomaticColorSettings rule headings", () => {
  it("includes the localized condition in each rule heading", () => {
    mocks.value = {
      enabled: true,
      rules: [
        {
          id: "origin-rule",
          enabled: false,
          condition: { dimension: "origin", value: "kanban", label: "Kanban" },
          output: { kind: "fixed", color: "red" },
        },
      ],
    };
    renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    expect(screen.getByText("Rule 1 Origin", { exact: true })).toBeTruthy();
  });
});

describe("AutomaticColorSettings", () => {
  it("adds a disabled incomplete rule without enabling the global setting", () => {
    renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));
    fireEvent.click(screen.getByTestId("automatic-color-add-rule"));

    expect(mocks.update).toHaveBeenCalledWith({
      enabled: false,
      rules: [
        expect.objectContaining({
          enabled: false,
          condition: { dimension: "task_state", value: null, label: "" },
          output: { kind: "fixed", color: "blue" },
        }),
      ],
    });
  });

  it("keeps an enabled rule editable when its target is unavailable", () => {
    mocks.value = {
      enabled: true,
      rules: [
        {
          id: "missing",
          enabled: true,
          condition: { dimension: "task_state", value: "IN_PROGRESS", label: "In progress" },
          output: { kind: "fixed", color: "red" },
        },
      ],
    };
    renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    const enabledSwitch = screen.getByTestId(
      "automatic-color-rule-enabled-missing",
    ) as HTMLButtonElement;
    expect(enabledSwitch.disabled).toBe(false);
    fireEvent.click(enabledSwitch);
    expect(mocks.update).toHaveBeenCalledWith({
      enabled: true,
      rules: [expect.objectContaining({ id: "missing", enabled: false })],
    });
  });

  it("shows one color swatch in the selected fixed output", () => {
    mocks.value = {
      enabled: true,
      rules: [
        {
          id: "fixed-output",
          enabled: true,
          condition: { dimension: "task_state", value: "TODO", label: "To do" },
          output: { kind: "fixed", color: "red" },
        },
      ],
    };
    renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    const output = screen.getByTestId("automatic-color-output-fixed-output");
    expect(output.querySelectorAll("span.inline-block.rounded-full")).toHaveLength(1);
  });

  it("keeps an incomplete rule disabled until its target is available", () => {
    mocks.value = {
      enabled: true,
      rules: [
        {
          id: "todo",
          enabled: false,
          condition: { dimension: "task_state", value: null, label: "" },
          output: { kind: "fixed", color: "red" },
        },
      ],
    };
    renderSettings();
    fireEvent.click(screen.getByTestId(SETTINGS_TOGGLE_TEST_ID));

    expect(
      (screen.getByTestId("automatic-color-rule-enabled-todo") as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByTestId("automatic-color-target-todo"));
    fireEvent.click(screen.getByRole("option", { name: "To do" }));
    expect(mocks.update).toHaveBeenCalledWith({
      enabled: true,
      rules: [
        expect.objectContaining({
          condition: { dimension: "task_state", value: "TODO", label: "To do" },
        }),
      ],
    });
  });
});
