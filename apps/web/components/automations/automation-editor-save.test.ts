import { useState } from "react";
import { addTrigger, getAutomation } from "@/lib/api/domains/automation-api";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSaveHandler, type SaveHandlerOpts } from "./automation-editor-save";
import type { FormState } from "./automation-payload";
import type { AutomationMCPSelection } from "@/lib/types/automation";
const TRIGGER_WRITE_FAILED = "trigger write failed";
const mocks = vi.hoisted(() => ({ resolve: vi.fn() }));
vi.mock("./automation-payload", async (original) => ({
  ...(await original<typeof import("./automation-payload")>()),
  resolveRepositoryIdsForMode: mocks.resolve,
}));
vi.mock("@/lib/api/domains/automation-api", () => ({
  addTrigger: vi.fn(),
  getAutomation: vi.fn(),
  deleteTrigger: vi.fn(),
  updateTrigger: vi.fn(),
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
/** Build a complete form with explicit defaults for automation save tests. */
function baseForm(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "Test",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "automation_run",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Run it",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
    continuationPolicy: "new_task",
    retryPolicy: {
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    },
    pluginTools: [],
    ...overrides,
  };
}

/** Keep saved MCP grants separate from edited grants when exercising save failures. */
function saveOptions(): SaveHandlerOpts {
  const savedForm = baseForm({
    mcpSelection: { builtin_packages: ["core"], definition_ids: [] },
    mcpSelectionRevision: 1,
  });
  const form = {
    ...savedForm,
    mcpSelection: {
      builtin_packages: ["core", "configuration"],
      definition_ids: [],
    } as FormState["mcpSelection"],
  };
  mocks.resolve.mockResolvedValue({ repositories: [], selections: [] });
  return {
    isNew: false,
    workspaceId: "workspace",
    currentId: "automation",
    form,
    savedForm,
    supportsMultiRepo: false,
    create: vi.fn(),
    update: vi.fn().mockResolvedValue({
      id: "automation",
      workspace_id: "workspace",
      mcp_selection: form.mcpSelection,
      mcp_selection_revision: 2,
    }),
    setSaving: vi.fn(),
    setCurrentId: vi.fn(),
    setForm: vi.fn(),
    setCreatedWebhook: vi.fn(),
    onSaved: vi.fn(),
    onPluginToolsSaveFailed: vi.fn(),
    onMCPSelectionSaveFailed: vi.fn(),
    triggerActions: {
      baselineTriggers: [],
      pending: [],
      persistDrafts: vi.fn().mockResolvedValue([]),
    } as unknown as SaveHandlerOpts["triggerActions"],
    router: { push: vi.fn() } as unknown as SaveHandlerOpts["router"],
  };
}
describe("automation MCP save failures", () => {
  it.each(["repository", "create", "update", "trigger"])(
    "reports MCP failure only for a failed update: %s",
    async (stage) => {
      const opts = saveOptions();
      const error = new Error(stage);
      if (stage === "repository") mocks.resolve.mockRejectedValue(error);
      if (stage === "create") {
        opts.isNew = true;
        opts.currentId = null;
        vi.mocked(opts.create).mockRejectedValue(error);
      }
      if (stage === "update") vi.mocked(opts.update).mockRejectedValue(error);
      if (stage === "trigger")
        vi.mocked(opts.triggerActions.persistDrafts).mockRejectedValue(error);
      const { result } = renderHook(() => useSaveHandler(opts));
      await act(async () => {
        await expect(result.current()).rejects.toBe(error);
      });
      expect(opts.onMCPSelectionSaveFailed).toHaveBeenCalledTimes(stage === "update" ? 1 : 0);
    },
  );
  it("offers MCP recovery when a mode-only update clears the selection", async () => {
    const opts = saveOptions();
    opts.form = { ...opts.savedForm, taskMode: "managed_conversation" };
    vi.mocked(opts.update).mockRejectedValue(new Error("conflict"));
    const { result } = renderHook(() => useSaveHandler(opts));
    await act(async () => {
      await expect(result.current()).rejects.toThrow("conflict");
    });
    expect(opts.onMCPSelectionSaveFailed).toHaveBeenCalledOnce();
  });
});

/** Exercise form and trigger baselines together at the native save boundary. */
function renderEditorSave(opts: SaveHandlerOpts) {
  return renderHook(() => {
    const [form, setForm] = useState(opts.form);
    const [savedForm, setSavedForm] = useState(opts.savedForm);
    const triggerActions = useAutomationTriggerDrafts(opts.currentId);
    const save = useSaveHandler({
      ...opts,
      form,
      savedForm,
      setForm,
      triggerActions,
      onSaved: (next) => setSavedForm(next),
    });
    return {
      form,
      savedForm,
      setForm,
      triggerActions,
      save,
      reset: () => {
        setForm(savedForm);
        triggerActions.discardDrafts();
      },
    };
  });
}

describe("partial automation saves", () => {
  it("resets to persisted MCP grants and uses their revision after a trigger save fails", async () => {
    const opts = saveOptions();
    vi.mocked(addTrigger).mockRejectedValue(new Error(TRIGGER_WRITE_FAILED));
    vi.mocked(getAutomation).mockResolvedValue({ triggers: [] } as never);
    const { result } = renderEditorSave(opts);
    await act(() => result.current.triggerActions.handleAdd("webhook", {}));
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow(TRIGGER_WRITE_FAILED);
    });
    expect(result.current.savedForm.mcpSelection).toEqual(opts.form.mcpSelection);
    expect(result.current.savedForm.mcpSelectionRevision).toBe(2);
    expect(result.current.triggerActions.pending).toHaveLength(1);
    expect(result.current.triggerActions.baselineTriggers).toEqual([]);
    act(() => result.current.reset());
    expect(result.current.form.mcpSelection).toEqual(opts.form.mcpSelection);
    expect(result.current.form.mcpSelectionRevision).toBe(2);
    expect(result.current.triggerActions.pending).toEqual([]);

    const selection: AutomationMCPSelection = {
      builtin_packages: ["core", "diagnostics"],
      definition_ids: [],
    };
    act(() => result.current.setForm((prev) => ({ ...prev, mcpSelection: selection })));
    vi.mocked(opts.update).mockResolvedValue({
      id: "automation",
      workspace_id: "workspace",
      mcp_selection: selection,
      mcp_selection_revision: 3,
    } as never);
    await act(() => result.current.save());
    expect(opts.update).toHaveBeenLastCalledWith(
      "automation",
      expect.objectContaining({
        mcp_selection: selection,
        expected_mcp_selection_revision: 2,
      }),
    );
    act(() => result.current.reset());
    expect(result.current.form.mcpSelection).toEqual(selection);
    expect(result.current.form.mcpSelectionRevision).toBe(3);
  });
});

const discovered = {
  kind: "discovered" as const,
  path: "/repo",
  name: "Repo",
  defaultBranch: "main",
  branch: "main",
  key: "row-1",
};
const promoted = {
  kind: "registered" as const,
  id: "registered-1",
  branch: "main",
  key: "row-1",
};

describe("repository promotion during automation saves", () => {
  it("retains registration after a trigger failure so retry does not register again", async () => {
    const opts = saveOptions();
    opts.form = { ...opts.form, repositoryMode: "selected", repositorySelections: [discovered] };
    let registrations = 0;
    mocks.resolve.mockImplementation(async (_workspace, selections) => {
      const next = selections.map((item: typeof discovered | typeof promoted) => {
        if (item.kind !== "discovered") return item;
        registrations += 1;
        return { ...promoted, id: `registered-${registrations}` };
      });
      return {
        selections: next,
        repositories: next.map((item: typeof promoted) => ({
          repository_id: item.id,
          base_branch: item.branch,
        })),
      };
    });
    const created = { id: "trigger-1", type: "webhook", config: {}, enabled: true };
    vi.mocked(addTrigger)
      .mockRejectedValueOnce(new Error(TRIGGER_WRITE_FAILED))
      .mockResolvedValueOnce(created as never);
    vi.mocked(getAutomation).mockResolvedValue({ triggers: [created] } as never);
    const { result } = renderEditorSave(opts);
    await act(() => result.current.triggerActions.handleAdd("webhook", {}));
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow(TRIGGER_WRITE_FAILED);
    });
    expect(result.current.form.repositorySelections).toEqual([promoted]);
    expect(result.current.triggerActions.pending).toHaveLength(1);
    await act(() => result.current.save());
    expect(registrations).toBe(1);
    expect(mocks.resolve.mock.calls[1][1]).toEqual([promoted]);
    expect(result.current.triggerActions.pending).toEqual([]);
  });

  const replacement = { ...discovered, path: "/other", name: "Other" };
  const additional = { ...replacement, key: "row-2" };
  it.each([
    {
      name: "branch",
      changes: [{ ...discovered, branch: "feature/new" }],
      expected: [{ ...promoted, branch: "feature/new" }],
    },
    { name: "replacement", changes: [replacement], expected: [replacement] },
    { name: "removal", changes: [], expected: [] },
    {
      name: "addition and ordering",
      changes: [additional, discovered],
      expected: [additional, promoted],
    },
  ])("preserves $name changes made during update", async ({ changes, expected }) => {
    const opts = saveOptions();
    opts.form = { ...opts.form, repositoryMode: "selected", repositorySelections: [discovered] };
    mocks.resolve.mockResolvedValue({
      selections: [promoted],
      repositories: [{ repository_id: promoted.id, base_branch: "main" }],
    });
    const response = {
      id: "automation",
      workspace_id: "workspace",
      mcp_selection: opts.form.mcpSelection,
      mcp_selection_revision: 2,
    } as never;
    let release!: () => void;
    let started!: () => void;
    const updating = new Promise<void>((resolve) => {
      started = resolve;
    });
    vi.mocked(opts.update).mockImplementation(
      () =>
        new Promise((resolve) => {
          release = () => resolve(response);
          started();
        }),
    );
    vi.mocked(getAutomation).mockResolvedValue({ triggers: [] } as never);
    const { result } = renderEditorSave(opts);
    let saving!: Promise<void>;
    await act(async () => {
      saving = result.current.save();
      await updating;
    });
    act(() => result.current.setForm((prev) => ({ ...prev, repositorySelections: changes })));
    await act(async () => {
      release();
      await saving;
    });
    expect(result.current.form.repositorySelections).toEqual(expected);
    expect(result.current.savedForm.repositorySelections).toEqual([promoted]);
  });
});
