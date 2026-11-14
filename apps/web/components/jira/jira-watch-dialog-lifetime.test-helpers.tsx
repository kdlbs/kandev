import { StrictMode, type ReactNode, useState } from "react";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { useJiraIssueWatches } from "@/hooks/domains/jira/use-jira-issue-watches";
import { defaultState } from "@/lib/state/default-state";
import type { JiraIssueWatch } from "@/lib/types/jira";
import { JiraIssueWatchersSection } from "./jira-issue-watchers-section";
import { JiraIssueWatchDialog } from "./jira-issue-watch-dialog";

export const workspace = "workspace-a";
export const otherWorkspace = "workspace-b";
export const rawOld = "  project = SUBMITTED  ";
export const rawNew = " raw-newer-query ";
export const placeholder = 'project = PROJ AND status = "Open"';
export function watch(id = "watch-a", workspaceId = workspace): JiraIssueWatch {
  return {
    id,
    workspaceId,
    workflowId: `workflow-${workspaceId}`,
    workflowStepId: `step-${workspaceId}`,
    repositoryId: "",
    baseBranch: "",
    jql: "project = FIXTURE",
    agentProfileId: "",
    executorProfileId: "",
    prompt: "Investigate fixture",
    enabled: false,
    pollIntervalSeconds: 300,
    maxInflightTasks: 5,
    createdAt: "2026-10-22T10:00:00Z",
    updatedAt: "2026-10-22T10:00:00Z",
  };
}
export let store: ReturnType<typeof useAppStoreApi>;
function Probe() {
  store = useAppStoreApi();
  return null;
}
export function Providers({
  children,
  active = workspace,
}: {
  children: ReactNode;
  active?: string;
}) {
  return (
    <StrictMode>
      <StateProvider
        initialState={{
          workspaces: {
            activeId: active || null,
            items: [workspace, otherWorkspace].map((id) => ({
              id,
              name: id,
              owner_id: "owner-a",
              created_at: watch().createdAt,
              updated_at: watch().updatedAt,
            })),
          },
          settingsData: { ...defaultState.settingsData, agentsLoaded: true, executorsLoaded: true },
          availableAgents: { ...defaultState.availableAgents, loaded: true },
          prompts: { ...defaultState.prompts, loaded: true },
        }}
      >
        <ToastProvider>
          <TooltipProvider>
            <SettingsSaveProvider>
              {children}
              <Probe />
            </SettingsSaveProvider>
          </TooltipProvider>
        </ToastProvider>
      </StateProvider>
    </StrictMode>
  );
}
export function Parent({ visible = true }: { visible?: boolean }) {
  return <Providers>{visible && <JiraIssueWatchersSection />}</Providers>;
}
export type Selection = { watch: JiraIssueWatch | null; workspaceId?: string; open?: boolean };
// A real domain consumer for prop lifetimes the install-wide UI does not expose.
export function Consumer({ watch: selected, workspaceId, open = true }: Selection) {
  const actions = useJiraIssueWatches();
  const [show, setShow] = useState(true);
  return (
    <JiraIssueWatchDialog
      open={open && show}
      watch={selected}
      workspaceId={workspaceId}
      onOpenChange={setShow}
      onCreate={actions.create}
      onUpdate={(id, payload) => actions.update(id, payload, selected!.workspaceId)}
    />
  );
}
function readResponse(
  u: URL,
  method: string,
  searches: Array<{ workspace: string | null; body: unknown }>,
) {
  const json = (data: unknown) => new Response(JSON.stringify(data), { status: 200 });
  if (u.pathname === "/api/v1/jira/watches/issue")
    return Promise.resolve(json({ watches: [watch(), watch("watch-b", otherWorkspace)] }));
  if (u.pathname === "/api/v1/workflows") {
    const scope = u.searchParams.get("workspace_id") ?? workspace;
    return Promise.resolve(
      json({
        workflows: [
          {
            id: `workflow-${scope}`,
            workspace_id: scope,
            name: `Workflow ${scope}`,
            hidden: false,
          },
        ],
      }),
    );
  }
  if (u.pathname.endsWith("/steps")) {
    const scope = u.pathname.includes(otherWorkspace) ? otherWorkspace : workspace;
    return Promise.resolve(
      json({
        steps: [
          {
            id: `step-${scope}`,
            name: `Step ${scope}`,
            position: 0,
            workflow_id: `workflow-${scope}`,
          },
        ],
      }),
    );
  }
  if (u.pathname.endsWith("/repositories")) {
    const scope = u.pathname.includes(otherWorkspace) ? otherWorkspace : workspace;
    return Promise.resolve(
      json({
        repositories: [
          {
            id: `repo-${scope}`,
            workspace_id: scope,
            name: `Repository ${scope}`,
            local_path: `/fixture/${scope}`,
            default_branch: "main",
          },
        ],
      }),
    );
  }
  if (u.pathname.endsWith("/branches"))
    return Promise.resolve(json({ branches: [{ name: "main", type: "local" }] }));
  if (u.pathname === "/api/v1/agents") return Promise.resolve(json({ agents: [] }));
  if (u.pathname === "/api/v1/prompts") return Promise.resolve(json({ prompts: [] }));
  if (u.pathname === "/api/v1/system/logs/frontend-errors" && method === "POST")
    return Promise.resolve(json({}));
  if (u.pathname === "/api/v1/jira/tickets") {
    searches.push({
      workspace: u.searchParams.get("workspace_id"),
      body: u.searchParams.get("jql"),
    });
    return Promise.resolve(json({ tickets: [], total: 0 }));
  }
  return undefined;
}
export function transport() {
  const writes: Array<{
    method: string;
    path: string;
    workspace: string | null;
    body: Record<string, unknown>;
    resolve: (r: Response) => void;
    done: boolean;
  }> = [];
  const unexpected: string[] = [];
  const searches: Array<{ workspace: string | null; body: unknown }> = [];
  const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const u = new URL(value, "http://localhost");
      const method = init?.method ?? "GET";
      if (
        /^\/api\/v1\/jira\/watches\/issue(?:\/[^/]+)?$/.test(u.pathname) &&
        ["POST", "PATCH"].includes(method)
      ) {
        const held = Promise.withResolvers<Response>();
        writes.push({
          method,
          path: u.pathname,
          workspace: u.searchParams.get("workspace_id"),
          body: JSON.parse(init!.body as string),
          resolve: held.resolve,
          done: false,
        });
        return held.promise;
      }
      const read = readResponse(u, method, searches);
      if (read) return read;
      unexpected.push(`${method} ${u.pathname}`);
      return Promise.reject(new Error(`Unexpected fixture request ${method} ${u.pathname}`));
    }),
  );
  function resolve(index: number, success: boolean) {
    const write = writes[index];
    if (write.done) return;
    write.done = true;
    write.resolve(
      json(
        success
          ? {
              ...watch(),
              ...write.body,
              workspaceId: write.workspace ?? write.body.workspaceId,
              id: write.method === "POST" ? `created-${index}` : write.path.split("/").at(-1),
            }
          : { error: `rejected-save-${index}` },
        success ? 200 : 500,
      ),
    );
  }
  return {
    writes,
    unexpected,
    searches,
    async settle(index: number, success = true) {
      await act(async () => resolve(index, success));
    },
    async dispose() {
      await act(async () => writes.forEach((_, i) => resolve(i, true)));
      expect(unexpected).toEqual([]);
    },
  };
}
export function dialog() {
  return screen.getByRole("dialog");
}
export function query() {
  return within(dialog()).getByPlaceholderText(placeholder) as HTMLTextAreaElement;
}
export function change(value: string) {
  fireEvent.change(query(), { target: { value } });
}
export function save() {
  fireEvent.click(within(dialog()).getByRole("button", { name: /^(Update|Create)$/ }));
}
export function cancel() {
  fireEvent.click(within(dialog()).getByRole("button", { name: "Cancel" }));
}
export async function openEdit(id = "watch-a") {
  const expected = store.getState().jiraIssueWatches.items.find((w) => w.id === id)?.jql;
  fireEvent.click(await screen.findByTestId(`jira-watch-row-${id}`));
  await screen.findByRole("dialog");
  await waitFor(() => expect(query().value).toBe(expected ?? watch().jql));
}
export async function choose(label: string, value: string) {
  const labelNode = within(dialog()).getByText(label, { exact: true });
  const trigger = within(labelNode.parentElement!).getByRole("combobox");
  fireEvent.click(trigger);
  fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: value }));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
}
export async function workflow(scope = workspace) {
  await waitFor(() =>
    expect(store.getState().workflows.items.some((w) => w.id === `workflow-${scope}`)).toBe(true),
  );
  await choose("Workflow", `Workflow ${scope}`);
  await waitFor(() =>
    expect(
      within(dialog())
        .getByText("Workflow Step", { exact: true })
        .parentElement!.querySelector("button"),
    ).toHaveProperty("disabled", false),
  );
  await choose("Workflow Step", `Step ${scope}`);
}
export async function openCreate() {
  fireEvent.click(screen.getByRole("button", { name: "New watcher" }));
  await screen.findByRole("dialog");
  await workflow();
}
export function focusDraft() {
  change(rawNew);
  const field = query();
  field.focus();
  field.setSelectionRange(2, 6);
  return field;
}
export function assertDraft(field: HTMLTextAreaElement) {
  expect(query()).toBe(field);
  expect(field.value).toBe(rawNew);
  expect(document.activeElement).toBe(field);
  expect(field.selectionStart).toBe(2);
  expect(field.selectionEnd).toBe(6);
}
export function expectOriginal(active: ReturnType<typeof transport>) {
  expect(active.writes[0]).toMatchObject({
    method: "PATCH",
    path: "/api/v1/jira/watches/issue/watch-a",
    workspace,
    body: {
      jql: rawOld,
      workflowId: `workflow-${workspace}`,
      workflowStepId: `step-${workspace}`,
      repositoryId: "",
      baseBranch: "",
      agentProfileId: "",
      executorProfileId: "",
      prompt: "Investigate fixture",
      enabled: false,
      pollIntervalSeconds: 300,
      maxInflightTasks: 5,
    },
  });
}
export function setActive(id: string) {
  act(() => store.setState((s) => ({ workspaces: { ...s.workspaces, activeId: id } })));
}
