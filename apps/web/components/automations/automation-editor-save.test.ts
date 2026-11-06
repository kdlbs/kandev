import { useState } from "react";
import { addTrigger, getAutomation } from "@/lib/api/domains/automation-api";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSaveHandler, type SaveHandlerOpts } from "./automation-editor-save";
import type { FormState } from "./automation-payload";
import type { AutomationMCPSelection } from "@/lib/types/automation";
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

describe("partial automation saves", () => {
  it("resets to persisted MCP grants and uses their revision after a trigger save fails", async () => {
    const opts = saveOptions();
    vi.mocked(addTrigger).mockRejectedValue(new Error("trigger write failed"));
    vi.mocked(getAutomation).mockResolvedValue({ triggers: [] } as never);
    const { result } = renderHook(() => {
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
    await act(() => result.current.triggerActions.handleAdd("webhook", {}));
    await act(async () => {
      await expect(result.current.save()).rejects.toThrow("trigger write failed");
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
