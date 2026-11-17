import { StrictMode, type ReactNode } from "react";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { vi, expect } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { defaultState } from "@/lib/state/default-state";
import type { Executor } from "@/lib/types/http";
import type { ReviewWatch, IssueWatch } from "@/lib/types/gitlab";
import { GitLabWatchSettings } from "./watch-settings";

export type Kind = "review" | "issue";
export type Watch = ReviewWatch & IssueWatch;
export const rawOld = "  state=opened&search=original  ";
export const rawNew = "  state=opened&search=unsent  ";
export const workspace = "watch-workspace-a";
export let store: ReturnType<typeof useAppStoreApi>;
function Probe() {
  store = useAppStoreApi();
  return null;
}
export function row(id = "watch-a", workspaceId = workspace): Watch {
  return {
    id,
    workspace_id: workspaceId,
    workflow_id: "workflow-a",
    workflow_step_id: "step-a",
    projects: [{ path: `group/${id}` }],
    agent_profile_id: "agent-profile-a",
    executor_profile_id: "executor-profile-a",
    prompt: "Investigate {{issue.url}}",
    custom_query: "state=opened",
    review_scope: "user",
    labels: ["bug"],
    enabled: false,
    poll_interval_seconds: 300,
    cleanup_policy: "auto",
    created_at: "2026-10-22T00:00:00Z",
    updated_at: "2026-10-22T00:00:00Z",
  };
}
const timestamp = "2026-10-22T00:00:00Z";
const executor: Executor = {
  id: "worktree-a",
  name: "Worktree",
  type: "worktree",
  status: "active",
  is_system: true,
  created_at: timestamp,
  updated_at: timestamp,
  profiles: [
    {
      id: "executor-profile-a",
      executor_id: "worktree-a",
      name: "Watch worktree",
      prepare_script: "",
      cleanup_script: "",
      created_at: timestamp,
      updated_at: timestamp,
    },
  ],
};
export function Providers({ children }: { children: ReactNode }) {
  return (
    <StrictMode>
      <StateProvider
        initialState={{
          settingsData: { ...defaultState.settingsData, agentsLoaded: true, executorsLoaded: true },
          availableAgents: { ...defaultState.availableAgents, loaded: true },
          executors: { ...defaultState.executors, items: [executor] },
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
export function Parent({
  visible = true,
  workspaceId = workspace,
}: {
  visible?: boolean;
  workspaceId?: string;
}) {
  return <Providers>{visible && <GitLabWatchSettings workspaceId={workspaceId} />}</Providers>;
}
const agentCatalogue = {
  agents: [
    {
      id: "agent-a",
      name: "mock-agent",
      supports_mcp: true,
      created_at: timestamp,
      updated_at: timestamp,
      profiles: [
        {
          id: "agent-profile-a",
          agent_id: "agent-a",
          agent_display_name: "Mock",
          name: "Watch agent",
          enabled: true,
        },
      ],
    },
  ],
  total: 1,
};
export function transport() {
  const writes: Array<{
    method: string;
    path: string;
    workspace: string | null;
    body: Record<string, unknown>;
    resolve: (value: Response) => void;
    done: boolean;
  }> = [];
  const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      const url = new URL(input, "http://localhost");
      const method = init?.method ?? "GET";
      const match = url.pathname.match(
        /^\/api\/v1\/gitlab\/watches\/(review|issue)(?:\/([^/]+))?$/,
      );
      if (match && method !== "GET") {
        const held = Promise.withResolvers<Response>();
        writes.push({
          method,
          path: url.pathname,
          workspace: url.searchParams.get("workspace_id"),
          body: JSON.parse(init!.body as string),
          resolve: held.resolve,
          done: false,
        });
        return held.promise;
      }
      if (match)
        return Promise.resolve(
          json({
            watches: [row(), row("watch-b")].map((w) => ({
              ...w,
              workspace_id: url.searchParams.get("workspace_id"),
            })),
          }),
        );
      if (url.pathname === "/api/v1/workflows")
        return Promise.resolve(
          json({
            workflows: [
              {
                id: "workflow-a",
                name: "Watch workflow",
                workspace_id: url.searchParams.get("workspace_id"),
                hidden: false,
              },
            ],
          }),
        );
      if (url.pathname === "/api/v1/workflows/workflow-a/workflow/steps")
        return Promise.resolve(
          json({ steps: [{ id: "step-a", name: "Watch step", position: 0 }] }),
        );
      if (/\/repositories$/.test(url.pathname)) return Promise.resolve(json({ repositories: [] }));
      if (url.pathname === "/api/v1/agents") return Promise.resolve(json(agentCatalogue));
      throw new Error(`Unexpected fixture request: ${method} ${url.pathname}`);
    }),
  );
  function resolve(index: number, success = true) {
    const write = writes[index];
    if (write.done) return;
    write.done = true;
    write.resolve(
      json(
        success
          ? {
              ...row(),
              ...write.body,
              workspace_id: write.workspace,
              id: write.method === "POST" ? `created-${index}` : write.path.split("/").at(-1),
            }
          : { error: `rejected-save-${index}` },
        success ? 200 : 500,
      ),
    );
  }
  return {
    writes,
    async settle(index: number, success = true) {
      await act(async () => resolve(index, success));
    },
    async dispose() {
      await act(async () => writes.forEach((_, i) => resolve(i)));
    },
  };
}
export function dialog() {
  return screen.getByRole("dialog");
}
export function query() {
  return within(dialog()).getByLabelText("GitLab query parameters") as HTMLInputElement;
}
export function change(value: string) {
  fireEvent.change(query(), { target: { value } });
}
export function save() {
  fireEvent.click(within(dialog()).getByRole("button", { name: /^(Update|Create) watch$/ }));
}
// i18n-exempt: English-locale test selector, never rendered as product copy.
export function close(name = "Cancel") {
  fireEvent.click(within(dialog()).getByRole("button", { name }));
}
export async function openEdit(kind: Kind, id = "watch-a") {
  const card = await screen.findByTestId(`gitlab-${kind}-watches-card`);
  const table = within(card).getByTestId("gitlab-watch-desktop-table");
  const label = await within(table).findByText(`group/${id}`, { exact: true });
  fireEvent.click(within(label.closest("tr")!).getByRole("button", { name: "Edit watch" }));
  await waitFor(() => expect(query().value).toBe("state=opened"));
}
export async function openCreate(kind: Kind) {
  // i18n-exempt: English-locale test selectors, never rendered as product copy.
  const heading = await screen.findByText(
    kind === "review" ? "Merge request review watches" : "Issue watches",
    { exact: true },
  );
  const section =
    heading.closest("section") ?? heading.parentElement!.parentElement!.parentElement!;
  fireEvent.click(within(section).getByRole("button", { name: "New watch" }));
  fireEvent.click(within(dialog()).getByLabelText("Workflow"));
  fireEvent.click(await screen.findByRole("option", { name: "Watch workflow" }));
  await waitFor(() =>
    expect((within(dialog()).getByLabelText("Workflow step") as HTMLButtonElement).disabled).toBe(
      false,
    ),
  );
  fireEvent.click(within(dialog()).getByLabelText("Workflow step"));
  fireEvent.click(await screen.findByRole("option", { name: "Watch step" }));
  for (const [label, name] of [
    ["Agent profile", "Mock • Watch agent"],
    ["Executor profile", "Watch worktree"],
  ]) {
    fireEvent.click(within(dialog()).getByLabelText(label));
    fireEvent.click(await screen.findByRole("option", { name }));
    await waitFor(() =>
      expect(document.activeElement).toBe(within(dialog()).getByLabelText(label)),
    );
  }
}
export function items(kind: Kind) {
  return store.getState()[kind === "review" ? "gitlabReviewWatches" : "gitlabIssueWatches"].items;
}
