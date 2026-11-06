import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSaveHandler, type SaveHandlerOpts } from "./automation-editor-save";
import type { FormState } from "./automation-payload";
const mocks = vi.hoisted(() => ({ resolve: vi.fn() }));
vi.mock("./automation-payload", async (original) => ({
  ...(await original<typeof import("./automation-payload")>()),
  resolveRepositoryIdsForMode: mocks.resolve,
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
