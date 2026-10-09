import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Automation } from "@/lib/types/automation";
import { AutomationsListPage } from "./automations-list-page";

const transport = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/automation-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/automation-api")>()),
  listAutomations: transport.list,
}));

function row(workspace: string, id: string, name: string): Automation {
  return {
    id,
    name,
    workspace_id: workspace,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-10-05T00:00:00Z",
    updated_at: "2026-10-05T00:00:00Z",
    triggers: [],
  };
}
function deferred() {
  let resolve!: (items: Automation[]) => void;
  const promise = new Promise<Automation[]>((yes) => {
    resolve = yes;
  });
  const request = { promise, resolve };
  pending.push(request);
  return request;
}
const pending: Array<{
  promise: Promise<Automation[]>;
  resolve: (items: Automation[]) => void;
}> = [];
function page(workspace: string) {
  return (
    <StateProvider>
      <SettingsSaveProvider>
        <TooltipProvider>
          <AutomationsListPage workspaceId={workspace} />
        </TooltipProvider>
      </SettingsSaveProvider>
    </StateProvider>
  );
}
beforeEach(() => {
  transport.list.mockReset();
  window.history.replaceState({}, "", "/settings/workspaces/a/automations");
  window.localStorage.clear();
});
afterEach(async () => {
  await act(async () => {
    pending.splice(0).forEach((request) => request.resolve([]));
  });
  cleanup();
  window.history.replaceState({}, "", "/");
  window.localStorage.clear();
});

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.12
it("keeps the real A table and row navigation scoped after cached return and late B", async () => {
  const a = deferred();
  const b = deferred();
  transport.list.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
  const { rerender } = render(page("a"));
  await act(async () => {
    a.resolve([row("a", "a-row", "A nightly report")]);
    await a.promise;
  });
  expect(screen.getByTestId("automation-row-a-row").textContent).toContain("A nightly report");
  rerender(page("b"));
  rerender(page("a"));
  await act(async () => {
    b.resolve([row("b", "b-row", "B maintenance")]);
    await b.promise;
  });
  expect(screen.queryByTestId("automation-row-b-row")).toBeNull();
  fireEvent.click(screen.getByTestId("automation-row-a-row"));
  expect(window.location.pathname).toBe("/settings/workspaces/a/automations/a-row");
  expect(transport.list).toHaveBeenCalledTimes(2);
});

it("renders initial loading and an accepted empty list through the real hook", async () => {
  const initial = deferred();
  transport.list.mockReturnValue(initial.promise);
  render(page("a"));
  expect(screen.getByText("Loading automations...")).toBeTruthy();
  expect(screen.queryByTestId("automations-empty")).toBeNull();
  await act(async () => {
    initial.resolve([]);
    await initial.promise;
  });
  expect(screen.getByTestId("automations-empty")).toBeTruthy();
});
