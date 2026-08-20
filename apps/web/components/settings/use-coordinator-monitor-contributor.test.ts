import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getCoordinatorMonitoringAction,
  setCoordinatorMonitoringAction,
} from "@/app/actions/workspaces";
import { workspaceId } from "@/lib/types/ids";
import type { SettingsSaveContributor } from "./settings-save-provider";
import { useCoordinatorMonitorContributor } from "./use-coordinator-monitor-contributor";

const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = workspaceId("ws-1");
const STEP_ID = "step-1";
const EDITED_PROMPT = "check this";

function configMap(prompt: string) {
  return { [STEP_ID]: { selected: true, prompt } };
}

const mockToast = vi.fn();
const captured = vi.hoisted(() => ({ contributor: null as SettingsSaveContributor | null }));

vi.mock("@/app/actions/workspaces", () => ({
  getCoordinatorMonitoringAction: vi.fn(),
  setCoordinatorMonitoringAction: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("./settings-save-provider", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./settings-save-provider")>();
  return {
    ...actual,
    useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
      captured.contributor = contributor;
    },
  };
});

function contributor(): SettingsSaveContributor {
  if (!captured.contributor) throw new Error("Contributor was not registered");
  return captured.contributor;
}

async function renderLoaded(workflowId: string = WORKFLOW_ID) {
  const view = renderHook(() =>
    useCoordinatorMonitorContributor({ workflowId, workspaceId: WORKSPACE_ID }),
  );
  await waitFor(() => expect(view.result.current.loading).toBe(false));
  return view;
}

beforeEach(() => {
  vi.clearAllMocks();
  captured.contributor = null;
  vi.mocked(getCoordinatorMonitoringAction).mockResolvedValue({ entries: [] });
});

describe("useCoordinatorMonitorContributor: loading", () => {
  it("starts in loading for persisted workflows so edits stay disabled until the load settles", () => {
    const view = renderHook(() =>
      useCoordinatorMonitorContributor({ workflowId: WORKFLOW_ID, workspaceId: WORKSPACE_ID }),
    );

    expect(view.result.current.loading).toBe(true);
  });

  it("loads saved entries into both draft and saved config", async () => {
    vi.mocked(getCoordinatorMonitoringAction).mockResolvedValueOnce({
      entries: [{ workflow_step_id: "step-1", selected: true, prompt: "watch closely" }],
    });

    const view = await renderLoaded();

    expect(getCoordinatorMonitoringAction).toHaveBeenCalledWith(WORKFLOW_ID);
    expect(view.result.current.draftConfig).toEqual({
      "step-1": { selected: true, prompt: "watch closely" },
    });
    expect(view.result.current.isDirty).toBe(false);
  });

  it("skips loading for a not-yet-persisted temporary workflow", async () => {
    const view = renderHook(() =>
      useCoordinatorMonitorContributor({
        workflowId: "temp-workflow-abc",
        workspaceId: WORKSPACE_ID,
      }),
    );

    await Promise.resolve();
    expect(getCoordinatorMonitoringAction).not.toHaveBeenCalled();
    expect(view.result.current.draftConfig).toEqual({});
  });

  it("shows an error toast when loading fails", async () => {
    vi.mocked(getCoordinatorMonitoringAction).mockRejectedValueOnce(new Error("network error"));

    await renderLoaded();

    expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("clears stale draft state and reloads when workflowId changes", async () => {
    vi.mocked(getCoordinatorMonitoringAction)
      .mockResolvedValueOnce({
        entries: [{ workflow_step_id: "step-1", selected: true, prompt: "first workflow" }],
      })
      .mockResolvedValueOnce({
        entries: [{ workflow_step_id: "step-2", selected: true, prompt: "second workflow" }],
      });

    const view = renderHook(
      ({ workflowId }) =>
        useCoordinatorMonitorContributor({ workflowId, workspaceId: WORKSPACE_ID }),
      { initialProps: { workflowId: "workflow-1" } },
    );

    await waitFor(() => expect(view.result.current.loading).toBe(false));
    expect(view.result.current.draftConfig).toEqual({
      "step-1": { selected: true, prompt: "first workflow" },
    });

    act(() => {
      view.rerender({ workflowId: "workflow-2" });
    });

    expect(view.result.current.loading).toBe(true);
    expect(view.result.current.draftConfig).toEqual({});

    await waitFor(() => expect(view.result.current.loading).toBe(false));
    expect(view.result.current.draftConfig).toEqual({
      "step-2": { selected: true, prompt: "second workflow" },
    });
  });
});

describe("useCoordinatorMonitorContributor: dirty tracking and save", () => {
  it("marks the contributor dirty when the draft config is edited", async () => {
    const view = await renderLoaded();
    expect(contributor().isDirty).toBe(false);

    act(() => {
      view.result.current.setDraftConfig(configMap(EDITED_PROMPT));
    });

    await waitFor(() => expect(contributor().isDirty).toBe(true));
  });

  it("saves only the changed entries and clears dirty state", async () => {
    vi.mocked(setCoordinatorMonitoringAction).mockResolvedValueOnce({
      entries: [{ workflow_step_id: STEP_ID, selected: true, prompt: EDITED_PROMPT }],
    });
    const view = await renderLoaded();

    act(() => {
      view.result.current.setDraftConfig(configMap(EDITED_PROMPT));
    });
    await waitFor(() => expect(contributor().isDirty).toBe(true));

    await act(async () => contributor().save(contributor().revision));

    expect(setCoordinatorMonitoringAction).toHaveBeenCalledWith(WORKFLOW_ID, {
      workspace_id: WORKSPACE_ID,
      entries: [{ workflow_step_id: STEP_ID, selected: true, prompt: EDITED_PROMPT }],
    });
    expect(view.result.current.draftConfig).toEqual(configMap(EDITED_PROMPT));
    await waitFor(() => expect(contributor().isDirty).toBe(false));
  });

  it("discards edits back to the last-saved config", async () => {
    vi.mocked(getCoordinatorMonitoringAction).mockResolvedValueOnce({
      entries: [{ workflow_step_id: "step-1", selected: true, prompt: "original" }],
    });
    const view = await renderLoaded();

    act(() => {
      view.result.current.setDraftConfig({ "step-1": { selected: true, prompt: "edited" } });
    });
    await waitFor(() => expect(contributor().isDirty).toBe(true));

    await act(async () => contributor().discard());

    expect(view.result.current.draftConfig).toEqual({
      "step-1": { selected: true, prompt: "original" },
    });
    expect(setCoordinatorMonitoringAction).not.toHaveBeenCalled();
  });
});
