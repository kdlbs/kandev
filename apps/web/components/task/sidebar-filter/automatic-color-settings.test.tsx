import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
  taskColorDimensionLabelKey: (dimension: string) => `task:${dimension}`,
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
      agentProfiles: { items: [] },
    }),
}));

import { AutomaticColorSettings } from "./automatic-color-settings";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.value = { enabled: false, rules: [] };
});

describe("AutomaticColorSettings", () => {
  it("adds a disabled incomplete rule without enabling the global setting", () => {
    render(<AutomaticColorSettings isDrawerLayout={false} />);
    fireEvent.click(screen.getByTestId("automatic-color-settings-toggle"));
    fireEvent.click(screen.getByTestId("automatic-color-add-rule"));

    expect(mocks.update).toHaveBeenCalledWith({
      enabled: false,
      rules: [
        expect.objectContaining({
          enabled: false,
          condition: { dimension: "task_state", value: null, label: "" },
          output: { kind: "fixed", color: "gray" },
        }),
      ],
    });
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
    render(<AutomaticColorSettings isDrawerLayout={false} />);
    fireEvent.click(screen.getByTestId("automatic-color-settings-toggle"));

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
