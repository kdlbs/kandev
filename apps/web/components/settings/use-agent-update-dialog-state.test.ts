import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((complete, fail) => {
    resolve = complete;
    reject = fail;
  });
  return { promise, reject, resolve };
}

const FIRST_PREVIEW: AgentUpdatePreview = {
  agent_name: "claude-acp",
  package: "@agentclientprotocol/claude-agent-acp",
  current_version: "0.62.0",
  target_version: "0.63.0",
  command: ["npm", "exec"],
  command_string: "npm exec",
};
const AGENT_NAME = FIRST_PREVIEW.agent_name;
const UPDATE_ALREADY_RUNNING = "Update is already running";

describe("useAgentUpdateDialogState", () => {
  it("ignores a preview that resolves after close and reopen", async () => {
    const firstRequest = deferred<AgentUpdatePreview>();
    const secondRequest = deferred<AgentUpdatePreview>();
    const onPreview = vi
      .fn<(agentName: string) => Promise<AgentUpdatePreview>>()
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview,
        onUpdate: vi.fn(),
      }),
    );

    act(() => result.current.handleOpenChange(true));
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(1));
    act(() => result.current.handleOpenChange(false));
    act(() => result.current.handleOpenChange(true));
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(2));

    await act(async () => {
      firstRequest.resolve(FIRST_PREVIEW);
      await firstRequest.promise;
    });

    expect(result.current.preview).toBeNull();
    expect(result.current.loading).toBe(true);

    await act(async () => {
      secondRequest.resolve({ ...FIRST_PREVIEW, target_version: "0.64.0" });
      await secondRequest.promise;
    });

    await waitFor(() => expect(result.current.preview?.target_version).toBe("0.64.0"));
  });

  it("ignores an approval failure after the dialog closes", async () => {
    const updateRequest = deferred<AgentUpdateJob>();
    const onUpdate = vi
      .fn<(agentName: string) => Promise<AgentUpdateJob>>()
      .mockReturnValue(updateRequest.promise);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview: vi.fn().mockImplementation((_agentName: string, targetVersion?: string) =>
          Promise.resolve({
            ...FIRST_PREVIEW,
            target_version: targetVersion ?? FIRST_PREVIEW.target_version,
          }),
        ),
        onUpdate,
      }),
    );

    await act(async () => {
      await result.current.loadPreview();
    });
    await waitFor(() => expect(result.current.preview).toEqual(FIRST_PREVIEW));
    act(() => {
      void result.current.approve();
    });
    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    act(() => result.current.handleOpenChange(false));

    await act(async () => {
      updateRequest.reject(new Error(UPDATE_ALREADY_RUNNING));
      await updateRequest.promise.catch(() => undefined);
    });

    expect(result.current.previewError).toBeNull();
    expect(result.current.starting).toBe(false);
  });

  it("keeps a start failure separate from the update preview", async () => {
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview: vi.fn().mockResolvedValue(FIRST_PREVIEW),
        onUpdate: vi.fn().mockRejectedValue(new Error(UPDATE_ALREADY_RUNNING)),
      }),
    );

    await act(async () => {
      await result.current.loadPreview();
    });
    await waitFor(() => expect(result.current.preview).toEqual(FIRST_PREVIEW));
    await act(async () => {
      await result.current.approve();
    });

    expect(result.current.previewError).toBeNull();
    expect(result.current.approveError).toBe(UPDATE_ALREADY_RUNNING);
  });
});

describe("useAgentUpdateDialogState target selection", () => {
  it("keeps the current preview while a selected target is loading", async () => {
    const selectedTargetPreview = deferred<AgentUpdatePreview>();
    const onPreview = vi
      .fn<(agentName: string, targetVersion?: string) => Promise<AgentUpdatePreview>>()
      .mockResolvedValueOnce(FIRST_PREVIEW)
      .mockReturnValueOnce(selectedTargetPreview.promise);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview,
        onUpdate: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.loadPreview();
    });
    act(() => result.current.selectTarget("0.61.0"));

    await waitFor(() => expect(onPreview).toHaveBeenCalledWith(AGENT_NAME, "0.61.0"));
    expect(result.current.preview).toEqual(FIRST_PREVIEW);
    expect(result.current.loading).toBe(true);

    await act(async () => {
      selectedTargetPreview.resolve({ ...FIRST_PREVIEW, target_version: "0.61.0" });
      await selectedTargetPreview.promise;
    });
    await waitFor(() => expect(result.current.preview?.target_version).toBe("0.61.0"));
  });

  it("refreshes a selected target and ignores an older target response", async () => {
    const first = deferred<AgentUpdatePreview>();
    const olderTarget = deferred<AgentUpdatePreview>();
    const newerTarget = deferred<AgentUpdatePreview>();
    const onPreview = vi
      .fn<(agentName: string, targetVersion?: string) => Promise<AgentUpdatePreview>>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(olderTarget.promise)
      .mockReturnValueOnce(newerTarget.promise);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview,
        onUpdate: vi.fn(),
      }),
    );

    act(() => void result.current.loadPreview());
    await waitFor(() => expect(onPreview).toHaveBeenCalledWith(AGENT_NAME, undefined));
    await act(async () => {
      first.resolve(FIRST_PREVIEW);
      await first.promise;
    });

    act(() => result.current.selectTarget("0.61.0"));
    act(() => result.current.selectTarget("0.60.0"));
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(3));

    await act(async () => {
      olderTarget.resolve({ ...FIRST_PREVIEW, target_version: "0.61.0" });
      await olderTarget.promise;
    });
    expect(result.current.preview).toEqual(FIRST_PREVIEW);

    await act(async () => {
      newerTarget.resolve({ ...FIRST_PREVIEW, target_version: "0.60.0" });
      await newerTarget.promise;
    });
    expect(result.current.selectedTarget).toBe("0.60.0");
    expect(result.current.preview?.target_version).toBe("0.60.0");
  });
});

describe("useAgentUpdateDialogState approval", () => {
  it("approves the selected exact target", async () => {
    const onUpdate = vi.fn().mockResolvedValue({
      job_id: "job-1",
      agent_name: AGENT_NAME,
      status: "queued",
      started_at: "2026-01-01T00:00:00.000Z",
    } satisfies AgentUpdateJob);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview: vi.fn().mockImplementation((_agentName: string, targetVersion?: string) =>
          Promise.resolve({
            ...FIRST_PREVIEW,
            target_version: targetVersion ?? FIRST_PREVIEW.target_version,
          }),
        ),
        onUpdate,
      }),
    );
    await act(async () => {
      await result.current.loadPreview();
    });
    act(() => result.current.selectTarget("0.61.0"));
    await waitFor(() => expect(result.current.selectedTarget).toBe("0.61.0"));
    await act(async () => {
      await result.current.approve();
    });
    expect(onUpdate).toHaveBeenLastCalledWith(AGENT_NAME, "0.61.0");
  });

  it("previews and approves the Kandev default through structural arguments", async () => {
    const defaultPreview: AgentUpdatePreview = {
      ...FIRST_PREVIEW,
      default_version: "0.61.0",
      active_version: "0.62.0",
      effective_version: "0.62.0",
      target_version: "0.61.0",
      operation: "use_default",
    };
    const onPreview = vi.fn().mockResolvedValue(defaultPreview);
    const onUpdate = vi.fn().mockResolvedValue({
      job_id: "job-2",
      agent_name: AGENT_NAME,
      status: "queued",
      started_at: "2026-01-01T00:00:00.000Z",
    } satisfies AgentUpdateJob);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        onPreview,
        onUpdate,
      }),
    );

    await act(async () => {
      await result.current.loadPreview();
    });
    act(() => result.current.selectDefault());
    await waitFor(() => expect(onPreview).toHaveBeenLastCalledWith(AGENT_NAME, undefined, true));
    expect(result.current.selectedUseDefault).toBe(true);
    await act(async () => {
      await result.current.approve();
    });
    expect(onUpdate).toHaveBeenLastCalledWith(AGENT_NAME, "0.61.0", true);
  });
});

describe("useAgentUpdateDialogState failed target selection", () => {
  it("clears a failed active job when selecting a new target", async () => {
    const failedJob: AgentUpdateJob = {
      job_id: "runtime-update-job-1",
      agent_name: AGENT_NAME,
      status: "failed",
      operation: "update",
      current_version: "0.62.0",
      target_version: "0.63.0",
      started_at: "2026-01-01T00:00:00.000Z",
      finished_at: "2026-01-01T00:01:00.000Z",
      error: "The package registry is unavailable",
    };
    const onPreview = vi.fn().mockImplementation((_agentName: string, targetVersion?: string) =>
      Promise.resolve({
        ...FIRST_PREVIEW,
        target_version: targetVersion ?? FIRST_PREVIEW.target_version,
        operation: targetVersion ? "rollback" : "update",
      }),
    );
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: AGENT_NAME,
        job: failedJob,
        onPreview,
        onUpdate: vi.fn().mockResolvedValue(failedJob),
      }),
    );

    await act(async () => {
      await result.current.loadPreview();
    });
    await act(async () => {
      await result.current.approve();
    });
    await waitFor(() => expect(result.current.activeJob?.job_id).toBe(failedJob.job_id));

    act(() => result.current.selectTarget("0.61.0"));
    await waitFor(() => expect(result.current.preview?.target_version).toBe("0.61.0"));
    expect(result.current.activeJob).toBeUndefined();
  });
});
