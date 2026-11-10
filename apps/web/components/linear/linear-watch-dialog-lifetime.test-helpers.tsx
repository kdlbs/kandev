import { StrictMode, type ReactNode } from "react";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { LinearIssueWatchersSection } from "./linear-issue-watchers-section";
import { defaultState } from "@/lib/state/default-state";
import type { LinearIssueWatch } from "@/lib/types/linear";

export const workspace = "workspace-a";
const originalQuery = "initial issue query";
const originalPrompt = "Investigate fixture";
const workflowId = "workflow-a";
export const rawOld = "  submitted old query  ";
export const rawNew = "  later raw query  ";
export function watch(id = "watch-a", workspaceId = workspace): LinearIssueWatch {
  return {
    id,
    workspaceId,
    workflowId,
    workflowStepId: "step-a",
    repositoryId: "",
    baseBranch: "",
    filter: { query: originalQuery },
    agentProfileId: "",
    executorProfileId: "",
    prompt: originalPrompt,
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
export function Providers({ children }: { children: ReactNode }) {
  return (
    <StrictMode>
      <StateProvider
        initialState={{
          workspaces: {
            activeId: workspace,
            items: [
              {
                id: workspace,
                name: workspace,
                owner_id: "owner-a",
                created_at: watch().createdAt,
                updated_at: watch().updatedAt,
              },
              {
                id: "workspace-b",
                name: "workspace-b",
                owner_id: "owner-a",
                created_at: watch().createdAt,
                updated_at: watch().updatedAt,
              },
            ],
          },
          settingsData: { ...defaultState.settingsData, agentsLoaded: true, executorsLoaded: true },
          availableAgents: { ...defaultState.availableAgents, loaded: true },
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
  return <Providers>{visible && <LinearIssueWatchersSection />}</Providers>;
}
function readProviderResponse(u: URL, scope: string) {
  const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
  if (u.pathname === "/api/v1/linear/teams")
    return Promise.resolve(json({ teams: [{ id: "team-a", key: "ENG", name: "Engineering" }] }));
  if (u.pathname === "/api/v1/linear/states")
    return Promise.resolve(
      json({ states: [{ id: `state-${scope}`, name: "Todo", type: "unstarted", position: 0 }] }),
    );
  if (u.pathname === "/api/v1/linear/labels")
    return Promise.resolve(json({ labels: [{ id: `label-${scope}`, name: "Bug" }] }));
  if (u.pathname === "/api/v1/linear/users")
    return Promise.resolve(json({ users: [{ id: `user-${scope}`, name: "Fixture creator" }] }));
  if (u.pathname === "/api/v1/linear/projects")
    return Promise.resolve(
      json({ projects: [{ id: `project-${scope}`, name: "Fixture project" }] }),
    );
}
function readResponse(u: URL, method: string): Promise<Response> | undefined {
  const scope = u.searchParams.get("workspace_id") ?? workspace;
  const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
  if (u.pathname === "/api/v1/linear/watches/issue")
    return Promise.resolve(json({ watches: [watch("watch-a", scope), watch("watch-b", scope)] }));
  const provider = readProviderResponse(u, scope);
  if (provider) return provider;
  if (u.pathname === "/api/v1/workflows")
    return Promise.resolve(
      json({
        workflows: [
          {
            id: scope === workspace ? workflowId : "workflow-b",
            workspace_id: scope,
            name: "Fixture workflow",
            hidden: false,
          },
        ],
      }),
    );
  if (/^\/api\/v1\/workflows\/workflow-[ab]\/workflow\/steps$/.test(u.pathname))
    return Promise.resolve(
      json({
        steps: [
          {
            id: u.pathname.includes("workflow-b") ? "step-b" : "step-a",
            name: "Fixture step",
            position: 0,
            workflow_id: u.pathname.split("/")[4],
          },
        ],
      }),
    );
  if (u.pathname.endsWith("/repositories"))
    return Promise.resolve(
      json({
        repositories: [
          {
            id: `repo-${scope}`,
            workspace_id: scope,
            name: "Fixture repository",
            local_path: "/tmp/fixture",
            default_branch: "main",
            created_at: watch().createdAt,
            updated_at: watch().updatedAt,
          },
        ],
      }),
    );
  if (u.pathname.endsWith("/branches"))
    return Promise.resolve(
      json({ branches: [{ name: "main", type: "local", is_default: true, is_current: true }] }),
    );
  if (u.pathname === "/api/v1/agents") return Promise.resolve(json({ agents: [] }));
  if (u.pathname === "/api/v1/prompts") return Promise.resolve(json({ prompts: [] }));
  if (u.pathname === "/api/v1/system/logs/frontend-errors" && method === "POST")
    return Promise.resolve(json({}));
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
  const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      const u = new URL(input, "http://localhost");
      const method = init?.method ?? "GET";
      if (
        /^\/api\/v1\/linear\/watches\/issue(?:\/[^/]+)?$/.test(u.pathname) &&
        ["PATCH", "POST"].includes(method)
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
      const read = readResponse(u, method);
      if (read) return read;
      unexpected.push(`${method} ${u.pathname}`);
      return Promise.reject(new Error(`Unexpected fixture request ${method} ${u.pathname}`));
    }),
  );
  function resolve(index: number, success: boolean) {
    const w = writes[index];
    if (w.done) return;
    w.done = true;
    w.resolve(
      json(
        success
          ? {
              ...watch(),
              ...w.body,
              workspaceId: w.workspace ?? w.body.workspaceId,
              id: w.method === "POST" ? `created-${index}` : w.path.split("/").at(-1),
            }
          : { error: `rejected-save-${index}` },
        success ? 200 : 500,
      ),
    );
  }
  return {
    writes,
    unexpected,
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
  return within(dialog()).getByPlaceholderText("auth bug") as HTMLInputElement;
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
  fireEvent.click(await screen.findByTestId(`linear-watch-row-${id}`));
  await screen.findByRole("dialog");
  await waitFor(() => expect(query().value).not.toBe(""));
}
export async function choose(label: string, option: string) {
  const trigger = within(dialog())
    .getByText(label, { exact: true })
    .parentElement!.querySelector('button[role="combobox"]')!;
  await waitFor(() => expect(trigger).toHaveProperty("disabled", false));
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("option", { name: option }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
}
export async function openCreate() {
  fireEvent.click(screen.getByRole("button", { name: "New watcher" }));
  await screen.findByRole("dialog");
  await choose("Workflow", "Fixture workflow");
  await choose("Workflow Step", "Fixture step");
}
export function focusDraft() {
  change(rawNew);
  const field = query();
  field.focus();
  field.setSelectionRange(3, 9);
  return field;
}
export function assertDraft(field: HTMLInputElement) {
  expect(query()).toBe(field);
  expect(field.value).toBe(rawNew);
  expect(document.activeElement).toBe(field);
  expect([field.selectionStart, field.selectionEnd]).toEqual([3, 9]);
}
export function expectOriginal(f: ReturnType<typeof transport>) {
  expect(f.writes[0]).toMatchObject({
    method: "PATCH",
    path: "/api/v1/linear/watches/issue/watch-a",
    workspace,
    body: {
      filter: { ...watch().filter, query: rawOld.trim() },
      workflowId,
      workflowStepId: "step-a",
      repositoryId: "",
      baseBranch: "",
      agentProfileId: "",
      executorProfileId: "",
      prompt: originalPrompt,
      enabled: false,
      pollIntervalSeconds: 300,
      maxInflightTasks: 5,
    },
  });
}
