import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";

import type { AgentUpdateJob } from "@/lib/api";

const startUpdateMock = vi.fn();
const refreshStatusesMock = vi.fn();
let updatePromise: Promise<AgentUpdateJob> | undefined;
let onUpdateCallback:
  | ((
      name: string,
      target: string,
      useDefault: boolean,
      mode: "self_update",
    ) => Promise<AgentUpdateJob>)
  | undefined;

vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));
vi.mock("@/hooks/domains/settings/use-agent-discovery", () => ({
  useAgentDiscovery: () => ({
    items: [
      {
        name: "omp-acp",
        available: true,
        installation_paths: [],
        matched_path: "omp",
        supports_mcp: false,
      },
    ],
    loading: false,
  }),
}));
vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({
    items: [
      {
        name: "omp-acp",
        display_name: "omp",
        runtime_update: { supported: true, update_mode: "self_update" },
      },
    ],
  }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-updates", () => ({
  useAgentRuntimeUpdates: () => ({
    updateJobs: {},
    previewUpdate: vi.fn(),
    startUpdate: startUpdateMock,
  }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ statusByAgent: {}, refresh: refreshStatusesMock }),
}));
vi.mock("@/components/settings/installed-agent-card", () => ({
  InstalledAgentCard: ({
    onUpdate,
  }: {
    onUpdate?: (
      name: string,
      target: string,
      useDefault: boolean,
      mode: "self_update",
    ) => Promise<AgentUpdateJob>;
  }) => {
    onUpdateCallback = onUpdate;
    return null;
  },
}));
vi.mock("@/components/settings/agents/agent-profiles-section", () => ({
  AgentProfilesSubList: () => null,
}));
vi.mock("@/components/settings/custom-tui-mcp-card", () => ({ CustomTUIMcpCard: () => null }));
vi.mock("@/components/settings/dynamic-agents-card", () => ({ DynamicAgentsCard: () => null }));
vi.mock("@/components/settings/host-shell-dialog", () => ({ HostShellDialog: () => null }));
vi.mock("@/components/settings/add-tui-agent-dialog", () => ({ AddTUIAgentDialog: () => null }));
vi.mock("./agent-options-dialog", () => ({
  AgentOptionsDialog: () => null,
}));

import AgentsSettingsPage from "./page";

afterEach(() => {
  vi.clearAllMocks();
  updatePromise = undefined;
  onUpdateCallback = undefined;
});

describe("Agents settings self-update approval", () => {
  it("starts one status refresh per terminal no-job response without awaiting a slow or failed read", async () => {
    const pending = Promise.withResolvers<void>();
    refreshStatusesMock
      .mockReturnValueOnce(pending.promise)
      .mockRejectedValueOnce(new Error("offline"));
    const terminal: AgentUpdateJob = {
      update_mode: "self_update",
      job_id: "",
      agent_name: "omp-acp",
      status: "succeeded",
      operation: "up_to_date",
      started_at: "2026-09-26T12:00:00Z",
    };
    startUpdateMock.mockResolvedValue(terminal);
    render(
      <StateProvider>
        <ToastProvider>
          <AgentsSettingsPage />
        </ToastProvider>
      </StateProvider>,
    );

    if (!onUpdateCallback) throw new Error("installed agent update callback was not rendered");
    updatePromise = onUpdateCallback("omp-acp", "", false, "self_update");
    await waitFor(() => expect(refreshStatusesMock).toHaveBeenCalledTimes(1));
    const firstUpdate = updatePromise;
    if (!firstUpdate) throw new Error("update callback did not return a promise");
    await expect(firstUpdate).resolves.toEqual(terminal);
    if (!onUpdateCallback) throw new Error("installed agent update callback was not rendered");
    updatePromise = onUpdateCallback("omp-acp", "", false, "self_update");
    await waitFor(() => expect(refreshStatusesMock).toHaveBeenCalledTimes(2));
    const secondUpdate = updatePromise;
    if (!secondUpdate) throw new Error("second update callback did not return a promise");
    await expect(secondUpdate).resolves.toEqual(terminal);
    expect(startUpdateMock).toHaveBeenCalledTimes(2);
    pending.resolve();
  });

  it("does not refresh statuses for an accepted job with a job ID", async () => {
    startUpdateMock.mockResolvedValue({
      update_mode: "self_update",
      job_id: "job-1",
      agent_name: "omp-acp",
      status: "queued",
      started_at: "2026-09-26T12:00:00Z",
    } satisfies AgentUpdateJob);
    render(
      <StateProvider>
        <ToastProvider>
          <AgentsSettingsPage />
        </ToastProvider>
      </StateProvider>,
    );

    if (!onUpdateCallback) throw new Error("installed agent update callback was not rendered");
    updatePromise = onUpdateCallback("omp-acp", "", false, "self_update");
    await waitFor(() => expect(startUpdateMock).toHaveBeenCalledOnce());

    expect(refreshStatusesMock).not.toHaveBeenCalled();
  });
});
