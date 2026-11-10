import { StrictMode, type ReactNode } from "react";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";
import { defaultState } from "@/lib/state/default-state";
import type { SentryIssueWatch } from "@/lib/types/sentry";

export const workspace = "workspace-a";
const originalQuery = "is:unresolved";
const originalPrompt = "Investigate fixture";
const workflowId = "workflow-a";
const orgSlug = "fixture-org";
export const rawOld = "  is:unresolved level:error  ";
export const rawNew = "  is:unresolved transaction:newer  ";
export function watch(id = "watch-a", workspaceId = workspace): SentryIssueWatch {
  return {
    id,
    workspaceId,
    sentryInstanceId: "instance-a",
    workflowId,
    workflowStepId: "step-a",
    repositoryId: "",
    baseBranch: "",
    filter: {
      orgSlug,
      projectSlugs: ["fixture-project"],
      query: originalQuery,
      levels: ["error"],
      statuses: ["unresolved"],
      statsPeriod: "24h",
    },
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
  return <Providers>{visible && <SentryIssueWatchersSection workspaceId={workspace} />}</Providers>;
}
function readResponse(u: URL, method: string): Promise<Response> | undefined {
  const scope = u.searchParams.get("workspace_id") ?? workspace;
  const json = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
  if (u.pathname === "/api/v1/sentry/watches/issue")
    return Promise.resolve(json({ watches: [watch("watch-a", scope), watch("watch-b", scope)] }));
  if (u.pathname === "/api/v1/sentry/instances")
    return Promise.resolve(
      json({
        instances: [
          {
            id: "instance-a",
            workspaceId: scope,
            name: "Fixture Sentry",
            authMethod: "auth_token",
            url: "https://sentry.fixture",
            hasSecret: true,
            lastOk: true,
            createdAt: watch().createdAt,
            updatedAt: watch().updatedAt,
          },
        ],
      }),
    );
  if (u.pathname === "/api/v1/sentry/organizations")
    return Promise.resolve(
      json({ organizations: [{ id: "org-a", slug: orgSlug, name: "Fixture Org" }] }),
    );
  if (u.pathname === "/api/v1/sentry/projects")
    return Promise.resolve(
      json({
        projects: [
          {
            id: "project-a",
            slug: "fixture-project",
            name: "Fixture project",
            orgSlug,
          },
        ],
      }),
    );
  if (u.pathname === "/api/v1/workflows")
    return Promise.resolve(
      json({
        workflows: [
          { id: workflowId, workspace_id: scope, name: "Fixture workflow", hidden: false },
        ],
      }),
    );
  if (u.pathname === "/api/v1/workflows/workflow-a/workflow/steps")
    return Promise.resolve(
      json({
        steps: [{ id: "step-a", name: "Fixture step", position: 0, workflow_id: workflowId }],
      }),
    );
  if (u.pathname.endsWith("/repositories")) return Promise.resolve(json({ repositories: [] }));
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
        /^\/api\/v1\/sentry\/watches\/issue(?:\/[^/]+)?$/.test(u.pathname) &&
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
  return within(dialog()).getByPlaceholderText(
    "is:unresolved transaction:/api/checkout",
  ) as HTMLInputElement;
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
  fireEvent.click(await screen.findByTestId(`sentry-watch-row-${id}`));
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
  await waitFor(() => expect(within(dialog()).getByText("Fixture Sentry")).toBeTruthy());
  await waitFor(() => expect(within(dialog()).getByText("fixture-org")).toBeTruthy());
  fireEvent.click(within(dialog()).getByTestId("sentry-watch-project-trigger"));
  fireEvent.click(await screen.findByRole("option", { name: "Fixture project (fixture-project)" }));
  fireEvent.keyDown(document, { key: "Escape" });
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
    path: "/api/v1/sentry/watches/issue/watch-a",
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
  expect(Object.keys(f.writes[0].body)).not.toContain("sentryInstanceId");
}
