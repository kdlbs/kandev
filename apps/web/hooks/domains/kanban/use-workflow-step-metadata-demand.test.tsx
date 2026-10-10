import { createElement, type ReactNode } from "react";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { useWorkflowStepsById } from "./use-workflow-steps-by-id";
const reads = vi.hoisted(() => ({ steps: vi.fn() }));
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps: reads.steps }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it.each([false, true])(
  "loads moved task metadata without board tasks (placeholder=%s)",
  async (placeholder) => {
    reads.steps.mockResolvedValue({
      steps: [
        {
          id: "analysis",
          name: "Analysis",
          color: "blue",
          position: 0,
          allow_manual_move: true,
          events: {},
        },
      ],
    });
    const state = {
      workspaces: { activeId: "workspace", items: [] },
      workflows: { items: [{ id: "destination", name: "Destination", workspaceId: "workspace" }] },
      kanban: { workflowId: "source", steps: [], tasks: [] },
      kanbanMulti: {
        snapshots: placeholder
          ? {
              destination: { workflowId: "destination", steps: [], tasks: [], isPlaceholder: true },
            }
          : {},
      },
    } as unknown as Partial<AppState>;
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(StateProvider, { initialState: state, children });
    const { result } = renderHook(() => useWorkflowStepsById("destination"), { wrapper });
    await waitFor(() =>
      expect(result.current).toEqual([
        expect.objectContaining({ id: "analysis", name: "Analysis", allow_manual_move: true }),
      ]),
    );
    expect(reads.steps).toHaveBeenCalledTimes(1);
  },
);

it("does not read step metadata outside the active workspace", () => {
  const state = {
    workspaces: { activeId: "workspace", items: [] },
    workflows: { items: [{ id: "foreign", name: "Foreign", workspaceId: "other" }] },
  } as unknown as Partial<AppState>;
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState: state, children });
  const { result } = renderHook(() => useWorkflowStepsById("foreign"), { wrapper });
  expect(result.current).toEqual([]);
  expect(reads.steps).not.toHaveBeenCalled();
});

it("discards an old destination response after a different workflow becomes active", async () => {
  let resolveOld!: (value: unknown) => void;
  reads.steps.mockImplementation((id: string) =>
    id === "old"
      ? new Promise((resolve) => {
          resolveOld = resolve;
        })
      : Promise.resolve({
          steps: [{ id: "new-step", name: "New step", position: 0, color: "blue" }],
        }),
  );
  const state = {
    workspaces: { activeId: "workspace", items: [] },
    workflows: { items: ["old", "new"].map((id) => ({ id, name: id, workspaceId: "workspace" })) },
  } as unknown as Partial<AppState>;
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState: state, children });
  const { result, rerender } = renderHook(({ workflow }) => useWorkflowStepsById(workflow), {
    initialProps: { workflow: "old" },
    wrapper,
  });
  rerender({ workflow: "new" });
  await waitFor(() => expect(result.current.map((step) => step.id)).toEqual(["new-step"]));
  resolveOld({ steps: [{ id: "old-step", name: "Old step", position: 0, color: "red" }] });
  await waitFor(() => expect(result.current.map((step) => step.id)).toEqual(["new-step"]));
});
