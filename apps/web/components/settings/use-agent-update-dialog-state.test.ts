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
        onPreview: vi.fn().mockResolvedValue(FIRST_PREVIEW),
        onUpdate,
      }),
    );

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
      await result.current.approve();
    });

    expect(result.current.previewError).toBeNull();
    expect(result.current.approveError).toBe(UPDATE_ALREADY_RUNNING);
  });
});
