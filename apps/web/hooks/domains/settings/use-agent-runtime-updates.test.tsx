import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { StateProvider } from "@/components/state-provider";
import type { AgentUpdateJob, InstallJob } from "@/lib/api";

const getAgentUpdateJobMock = vi.fn();
const getInstallJobMock = vi.fn();
const previewAgentUpdateMock = vi.fn();
const updateAgentMock = vi.fn();

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  getAgentUpdateJob: (...args: unknown[]) => getAgentUpdateJobMock(...args),
  getInstallJob: (...args: unknown[]) => getInstallJobMock(...args),
  previewAgentUpdate: (...args: unknown[]) => previewAgentUpdateMock(...args),
  updateAgent: (...args: unknown[]) => updateAgentMock(...args),
}));

import { useAgentRuntimeUpdates } from "./use-agent-runtime-updates";

const AGENT = "grok-acp";

/** A 409 the backend returns when another maintenance job holds the agent. */
function conflict(activeKind: "install" | "update", jobId = "job-1") {
  return new ApiError("conflict", 409, { active_job_id: jobId, active_kind: activeKind });
}

function updateJob(overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    job_id: "job-1",
    agent_name: AGENT,
    status: "updating",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  } as AgentUpdateJob;
}

function installJob(overrides: Partial<InstallJob> = {}): InstallJob {
  return {
    job_id: "job-1",
    agent_name: AGENT,
    status: "running",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  } as InstallJob;
}

function render() {
  return renderHook(() => useAgentRuntimeUpdates(), {
    wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useAgentRuntimeUpdates", () => {
  // The throw is what tells the dialog "an install is already running" — it is
  // caught in use-agent-update-dialog-state and rendered through
  // `agents:unableToStartUpdate`. A renamed key would make `t()` return the key
  // name silently, so assert the resolved English.
  it("throws the localized message when a 409 reports an active install job", async () => {
    updateAgentMock.mockRejectedValueOnce(conflict("install"));
    getInstallJobMock.mockResolvedValueOnce(installJob());

    const { result } = render();

    await expect(result.current.startUpdate(AGENT)).rejects.toThrow(
      "Agent installation is already in progress.",
    );
    expect(getInstallJobMock).toHaveBeenCalledWith("job-1", { cache: "no-store" });
  });

  // The sibling branch must NOT throw — a concurrent *update* is adopted and
  // returned so the dialog attaches to the running job instead of erroring.
  it("adopts the running job when a 409 reports an active update job", async () => {
    updateAgentMock.mockRejectedValueOnce(conflict("update"));
    getAgentUpdateJobMock.mockResolvedValueOnce(updateJob());

    const { result } = render();

    await expect(result.current.startUpdate(AGENT)).resolves.toMatchObject({ job_id: "job-1" });
    expect(getAgentUpdateJobMock).toHaveBeenCalledWith("job-1", { cache: "no-store" });
  });

  it("rethrows a non-conflict failure untouched", async () => {
    updateAgentMock.mockRejectedValueOnce(new Error("network down"));

    const { result } = render();

    await expect(result.current.startUpdate(AGENT)).rejects.toThrow("network down");
    expect(getInstallJobMock).not.toHaveBeenCalled();
    expect(getAgentUpdateJobMock).not.toHaveBeenCalled();
  });

  it("returns the started job when no conflict occurs", async () => {
    updateAgentMock.mockResolvedValueOnce(updateJob({ status: "queued" }));

    const { result } = render();

    await expect(result.current.startUpdate(AGENT)).resolves.toMatchObject({ status: "queued" });
  });
});
