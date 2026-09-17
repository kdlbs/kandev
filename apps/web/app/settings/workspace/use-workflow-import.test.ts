import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { importWorkflowsAction, previewWorkflowImportAction } from "@/app/actions/workspaces";
import type { WorkflowImportPreview, Workspace } from "@/lib/types/http";
import { useWorkflowImport } from "./use-workflow-import";

vi.mock("@/app/actions/workspaces", () => ({
  importWorkflowsAction: vi.fn(),
  previewWorkflowImportAction: vi.fn(),
}));

const workspace = { id: "workspace-1", name: "Workspace" } as Workspace;
const profileA = {
  id: "profile-a",
  name: "Profile A",
  agent_name: "Agent A",
  model: "model-a",
  mode: "mode-a",
  updated_at: "2026-09-17T13:00:00Z",
};
const profileB = {
  id: "profile-b",
  name: "Profile B",
  agent_name: "Agent B",
  model: "model-b",
  mode: "mode-b",
  updated_at: "2026-09-17T13:00:00Z",
};

const preview: WorkflowImportPreview = {
  skipped: [],
  profiles: [profileA, profileB],
  steps: [
    {
      workflow_index: 0,
      workflow_name: "Imported",
      step_position: 0,
      step_name: "Build",
      requested_profile: { agent_name: "Missing A", model: "a", mode: "mode" },
      matched_profile: { id: profileA.id, updated_at: profileA.updated_at },
    },
    {
      workflow_index: 0,
      workflow_name: "Imported",
      step_position: 1,
      step_name: "Review",
      requested_profile: { agent_name: "Missing B", model: "b", mode: "mode" },
    },
  ],
};
const importYaml = "portable yaml";

function renderImportHook() {
  const routerMock = { refresh: vi.fn() };
  const toast = vi.fn();
  const view = renderHook(() =>
    useWorkflowImport({ workspace, router: routerMock as never, toast }),
  );
  return { ...view, router: routerMock, toast };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(previewWorkflowImportAction).mockResolvedValue(preview);
  vi.mocked(importWorkflowsAction).mockResolvedValue({ created: ["Imported"], skipped: [] });
});

it("previews first, keeps exact matches, and submits independent selections", async () => {
  const { result, router, toast } = renderImportHook();

  act(() => result.current.setImportYaml(importYaml));
  await act(async () => {
    await result.current.handleImport();
  });

  expect(previewWorkflowImportAction).toHaveBeenCalledWith(workspace.id, importYaml);
  expect(result.current.importPhase).toBe("resolving");
  expect(result.current.selections).toEqual({ "0:0": profileA.id });
  expect(importWorkflowsAction).not.toHaveBeenCalled();

  act(() => result.current.selectProfile("0:1", profileB.id));
  await act(async () => {
    await result.current.handleImport();
  });

  expect(importWorkflowsAction).toHaveBeenCalledWith(workspace.id, importYaml, [
    {
      workflow_index: 0,
      step_position: 0,
      requested_profile: preview.steps[0].requested_profile,
      profile_id: profileA.id,
      profile_updated_at: profileA.updated_at,
    },
    {
      workflow_index: 0,
      step_position: 1,
      requested_profile: preview.steps[1].requested_profile,
      profile_id: profileB.id,
      profile_updated_at: profileB.updated_at,
    },
  ]);
  expect(router.refresh).toHaveBeenCalledOnce();
  expect(toast).toHaveBeenCalled();
  expect(result.current.importYaml).toBe("");
});

it("ignores a stale preview after the draft changes", async () => {
  let resolvePreview!: (value: WorkflowImportPreview) => void;
  vi.mocked(previewWorkflowImportAction).mockReturnValueOnce(
    new Promise((resolve) => {
      resolvePreview = resolve;
    }),
  );
  const { result } = renderImportHook();

  act(() => result.current.setImportYaml("first"));
  const request = result.current.handleImport();
  act(() => result.current.setImportYaml("second"));
  resolvePreview(preview);
  await act(async () => {
    await request;
  });

  expect(result.current.preview).toBeNull();
  expect(result.current.importYaml).toBe("second");
  expect(importWorkflowsAction).not.toHaveBeenCalled();
});

it("retains valid selections and refreshes the preview after a profile conflict", async () => {
  const { result } = renderImportHook();
  const conflict = {
    workflow_index: 0,
    step_position: 0,
    workflow_name: "Imported",
    step_name: "Build",
    reason: "changed_profile",
  } as const;
  const conflictError = Object.assign(new Error("profile changed"), {
    body: {
      code: "workflow_import_profiles_required",
      error: "workflow import requires agent profile selections",
      steps: [conflict],
    },
  });
  vi.mocked(importWorkflowsAction).mockRejectedValueOnce(conflictError);
  const refreshedPreview: WorkflowImportPreview = {
    ...preview,
    profiles: [{ ...profileA, updated_at: "2026-09-17T14:00:00Z" }, profileB],
  };

  act(() => result.current.setImportYaml(importYaml));
  await act(async () => {
    await result.current.handleImport();
  });
  act(() => result.current.selectProfile("0:1", profileB.id));
  await act(async () => {
    await result.current.handleImport();
  });

  expect(result.current.importPhase).toBe("resolving");
  expect(result.current.profileConflicts).toEqual([conflict]);
  expect(result.current.selections).toEqual({ "0:1": profileB.id });

  vi.mocked(previewWorkflowImportAction).mockResolvedValueOnce(refreshedPreview);
  await act(async () => {
    await result.current.retryPreview();
  });

  expect(result.current.selections).toEqual({ "0:1": profileB.id });
  expect(result.current.missingSteps).toEqual([refreshedPreview.steps[0]]);
});

it("does not submit a late all-matched preview after cancel and reopen", async () => {
  let resolvePreview!: (value: WorkflowImportPreview) => void;
  vi.mocked(previewWorkflowImportAction).mockReturnValueOnce(
    new Promise((resolve) => {
      resolvePreview = resolve;
    }),
  );
  const { result } = renderImportHook();
  const allMatchedPreview: WorkflowImportPreview = {
    ...preview,
    steps: preview.steps.map((step) => ({
      ...step,
      matched_profile: { id: profileA.id, updated_at: profileA.updated_at },
    })),
  };

  act(() => result.current.setIsImportDialogOpen(true));
  act(() => result.current.setImportYaml(importYaml));
  let pendingPreview: Promise<void> | undefined;
  act(() => {
    pendingPreview = result.current.handleImport();
  });
  await waitFor(() => expect(result.current.importPhase).toBe("previewing"));

  act(() => result.current.setIsImportDialogOpen(false));
  act(() => result.current.setIsImportDialogOpen(true));
  resolvePreview(allMatchedPreview);
  await act(async () => {
    await pendingPreview;
  });

  expect(importWorkflowsAction).not.toHaveBeenCalled();
  expect(result.current.importPhase).toBe("editing");
});

it("ignores a late file-read result after the import dialog is canceled", () => {
  const reader = {
    onload: null,
    readAsText: vi.fn(),
  } as unknown as FileReader;
  function MockFileReader() {
    return reader;
  }
  const fileReader = vi
    .spyOn(globalThis, "FileReader")
    .mockImplementation(MockFileReader as unknown as typeof FileReader);

  try {
    const { result } = renderImportHook();
    act(() => result.current.setIsImportDialogOpen(true));
    act(() => {
      result.current.handleFileUpload({
        target: { files: [new File(["late yaml"], "late.yaml")], value: "" },
      } as never);
    });

    act(() => result.current.setIsImportDialogOpen(false));
    act(() => {
      reader.onload?.call(reader, {
        target: { result: "late yaml" },
      } as ProgressEvent<FileReader>);
    });

    expect(result.current.importYaml).toBe("");
  } finally {
    fileReader.mockRestore();
  }
});

it("does not submit twice while the final request is pending", async () => {
  const { result } = renderImportHook();
  const exactPreview: WorkflowImportPreview = {
    ...preview,
    steps: preview.steps.map((step) => ({
      ...step,
      matched_profile: { id: profileA.id, updated_at: profileA.updated_at },
    })),
  };
  vi.mocked(previewWorkflowImportAction).mockResolvedValueOnce(exactPreview);
  act(() => result.current.setImportYaml(importYaml));

  let resolveImport!: (value: { created: string[]; skipped: string[] }) => void;
  vi.mocked(importWorkflowsAction).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveImport = resolve;
    }),
  );
  let firstSubmit: Promise<void> | undefined;
  act(() => {
    firstSubmit = result.current.handleImport();
  });
  await waitFor(() => expect(result.current.importPhase).toBe("submitting"));
  act(() => result.current.setImportYaml("edited while submitting"));
  await act(async () => {
    await result.current.handleImport();
  });
  expect(previewWorkflowImportAction).toHaveBeenCalledOnce();
  expect(importWorkflowsAction).toHaveBeenCalledOnce();

  resolveImport({ created: ["Imported"], skipped: [] });
  await act(async () => {
    await firstSubmit;
  });
});
