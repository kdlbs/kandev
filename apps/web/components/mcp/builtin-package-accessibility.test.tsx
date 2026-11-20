import { useState } from "react";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AutomationBuiltinPackages } from "@/components/automations/automation-builtin-packages";
import type { BuiltinMCPDescriptor } from "@/lib/types/builtin-mcp";
import { BuiltinPackageDetail, type BuiltinMCPEditor } from "./builtin-package-detail";
import { BuiltinPackageList } from "./builtin-package-list";

const INHERIT_LABEL = "Use inherited value";

const item: BuiltinMCPDescriptor = {
  id: "diagnostics",
  runtime_name: "kandev-diagnostics",
  label_key: "settings:builtinMcp.packages.diagnostics.label",
  description_key: "settings:builtinMcp.packages.diagnostics.description",
  default_policy: "default",
  surfaces: ["kanban-task", "automation"],
  required_surfaces: null,
  tools: [],
};
function editor(): BuiltinMCPEditor {
  return {
    key: "test",
    view: {
      policy: { workspace_id: "ws", version: 1, revision: 0, packages: { diagnostics: "blocked" } },
      catalog: [item],
      can_edit: true,
      surface: "kanban-task",
    },
    draft: {},
    error: null,
    pending: false,
    dirty: false,
    setChoice: vi.fn(),
    save: vi.fn(),
    reload: vi.fn(),
    discard: vi.fn(),
    reset: vi.fn(),
  };
}
afterEach(cleanup);

it.each(["list", "detail"])("explains a blocked task switch in the %s", (surface) => {
  const state = editor();
  render(
    surface === "list" ? (
      <BuiltinPackageList editor={state} onSelect={vi.fn()} />
    ) : (
      <BuiltinPackageDetail editor={state} item={item} scope="tasks" onBack={vi.fn()} />
    ),
  );
  const toggle = screen.getByRole("switch", {
    description: /Inherited from workspace: Off.*Workspace policy blocks this package/i,
  });
  expect(toggle.hasAttribute("disabled")).toBe(true);
});

it.each(["list", "detail"])("explains a blocked automation switch in the %s", (surface) => {
  render(
    <AutomationBuiltinPackages
      options={{
        packages: [item],
        definitions: [],
        inherited: [],
        task_defaults: [],
        task_blocked: ["diagnostics"],
      }}
      value={{ builtin_packages: ["core"], definition_ids: [] }}
      onChange={vi.fn()}
      disabled={false}
      taskMode="normal_task"
    />,
  );
  if (surface === "detail")
    fireEvent.click(screen.getByTestId("automation-mcp-package-diagnostics"));
  const toggle = screen.getByRole("switch", { description: /blocked by workspace policy/i });
  expect(toggle.hasAttribute("disabled")).toBe(true);
  expect(screen.getAllByTestId("automation-mcp-choice-diagnostics")).toHaveLength(1);
});

it("groups workspace switches by their package name", () => {
  render(<BuiltinPackageList editor={editor()} scope="workspaces" onSelect={vi.fn()} />);
  const group = screen.getByRole("group", { name: "Diagnostics" });
  expect(within(group).getAllByRole("switch")).toHaveLength(2);
});

function TaskControlsHarness({ surface, blocked }: { surface: string; blocked: boolean }) {
  const [choice, setChoice] = useState<"on" | "off" | "inherit">("on");
  const state = editor();
  state.view!.policy.packages.diagnostics = blocked ? "blocked" : "default";
  state.draft = { diagnostics: choice };
  state.setChoice = (_id, value) => setChoice(value as typeof choice);
  return surface === "list" ? (
    <BuiltinPackageList editor={state} onSelect={vi.fn()} />
  ) : (
    <BuiltinPackageDetail editor={state} item={item} scope="tasks" onBack={vi.fn()} />
  );
}

it.each(["list", "detail"])("keeps focus in the %s after restoring inheritance", (surface) => {
  render(<TaskControlsHarness surface={surface} blocked={false} />);
  const reset = screen.getByRole("button", { name: INHERIT_LABEL });
  reset.focus();
  fireEvent.click(reset);
  expect(screen.queryByRole("button", { name: INHERIT_LABEL })).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("switch"));
});

it.each(["list", "detail"])(
  "keeps focus in the blocked %s after restoring inheritance",
  (surface) => {
    render(<TaskControlsHarness surface={surface} blocked />);
    const reset = screen.getByRole("button", { name: INHERIT_LABEL });
    reset.focus();
    fireEvent.click(reset);
    expect(screen.queryByRole("button", { name: INHERIT_LABEL })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("group", { name: "Diagnostics" }));
  },
);
