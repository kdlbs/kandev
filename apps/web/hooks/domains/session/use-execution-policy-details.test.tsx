import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { useExecutionPolicyDetails } from "./use-execution-policy-details";
import type { ExecutionSnapshot } from "@/lib/types/execution";

const mocks = vi.hoisted(() => ({
  access: { enabled: true, identity: "operator", canRead: true, canEdit: true },
  fetchTask: vi.fn(),
  fetchSandboxRevision: vi.fn(),
  previewSandboxSelection: vi.fn(),
}));
vi.mock("@/hooks/domains/settings/use-sandbox-access", () => ({
  useSandboxAccess: () => mocks.access,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: mocks.fetchTask }));
vi.mock("@/hooks/use-task", () => ({ useTask: () => null }));
vi.mock("@/lib/api/domains/sandbox-api", () => mocks);
const snapshot = {
  task_id: "task",
  executor_id: "executor",
  agent_profile_id: "profile",
  sandbox: { revision_id: "captured", policy_digest: "digest" },
} as ExecutionSnapshot;
function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.fetchTask.mockResolvedValue({
    id: "task",
    workspace_id: "workspace",
    workflow_step_id: "step",
  });
  mocks.access = { enabled: true, identity: "operator", canRead: true, canEdit: true };
  mocks.fetchSandboxRevision.mockResolvedValue({ id: "captured", digest: "digest" });
  mocks.previewSandboxSelection.mockResolvedValue({ resolved: null, eligible: [] });
});
it("reads the captured revision separately from a complete current task context", async () => {
  const { result } = renderHook(() => useExecutionPolicyDetails(snapshot), { wrapper });
  await waitFor(() => expect(result.current.preview.data).toBeDefined());
  expect(mocks.fetchSandboxRevision.mock.calls[0][0]).toBe("captured");
  expect(mocks.previewSandboxSelection.mock.calls[0][0]).toEqual({
    workspace_id: "workspace",
    task_id: "task",
    workflow_step_id: "step",
    executor_id: "executor",
    agent_profile_id: "profile",
  });
  expect(result.current.captured).toMatchObject({ id: "captured" });
});
it("does not expose policy reads after operator identity changes", async () => {
  let finish!: (value: unknown) => void;
  mocks.previewSandboxSelection.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const { result, rerender } = renderHook(() => useExecutionPolicyDetails(snapshot), { wrapper });
  await waitFor(() => expect(mocks.previewSandboxSelection).toHaveBeenCalledOnce());
  mocks.access = { enabled: true, identity: "signed-out", canRead: false, canEdit: false };
  rerender();
  await act(async () => finish({ resolved: { revision: { id: "private" } } }));
  expect(result.current.preview.data).toBeUndefined();
  expect(result.current.captured).toBeUndefined();
});
it("keeps member reads separate from operator preview and rejects a mismatched revision", async () => {
  mocks.access.canEdit = false;
  mocks.fetchSandboxRevision.mockResolvedValue({ id: "captured", digest: "foreign" });
  const { result } = renderHook(() => useExecutionPolicyDetails(snapshot), { wrapper });
  await waitFor(() => expect(result.current.revision.isSuccess).toBe(true));
  expect(result.current.captured).toBeUndefined();
  expect(mocks.previewSandboxSelection).not.toHaveBeenCalled();
});

it("reads the current saved task when the board cache does not contain it", async () => {
  const { result } = renderHook(() => useExecutionPolicyDetails(snapshot), { wrapper });
  await waitFor(() => expect(mocks.fetchTask).toHaveBeenCalled());
  await waitFor(() => expect(result.current.preview.data).toBeDefined());
  expect(mocks.previewSandboxSelection.mock.calls[0][0]).toMatchObject({
    task_id: "task",
    workspace_id: "workspace",
    workflow_step_id: "step",
  });
});
