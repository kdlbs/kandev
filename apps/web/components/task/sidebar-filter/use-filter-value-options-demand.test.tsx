import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { useFilterValueOptions } from "./use-filter-value-options";

const reads = vi.hoisted(() => ({ steps: vi.fn() }));
vi.mock("@/lib/api/domains/workflow-api", () => ({ listWorkflowSteps: reads.steps }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("loads step filter choices from workspace metadata without requesting board tasks", async () => {
  reads.steps.mockImplementation(async (workflow: string) => ({
    steps: [{ id: `${workflow}-done`, name: "Done", position: 0, color: "blue" }],
  }));
  const state = {
    workspaces: { activeId: "workspace", items: [] },
    workflows: {
      activeId: "alpha",
      items: [
        { id: "alpha", name: "Alpha", workspaceId: "workspace" },
        { id: "beta", name: "Beta", workspaceId: "workspace" },
        { id: "foreign", name: "Foreign", workspaceId: "other" },
      ],
    },
    kanbanMulti: { snapshots: {} },
  } as unknown as Partial<AppState>;
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState: state, children });
  const { result, rerender } = renderHook(({ dimension }) => useFilterValueOptions(dimension), {
    initialProps: { dimension: "repository" as "repository" | "workflowStep" },
    wrapper,
  });
  expect(reads.steps).not.toHaveBeenCalled();
  act(() => rerender({ dimension: "workflowStep" }));
  await waitFor(() =>
    expect(result.current.map((option) => [option.value, option.group])).toEqual([
      ["alpha-done", "Alpha"],
      ["beta-done", "Beta"],
    ]),
  );
  expect(reads.steps.mock.calls.map(([workflow]) => workflow).sort()).toEqual(["alpha", "beta"]);
});

it("shares overlapping step option reads for the same workspace generation", async () => {
  let resolve!: (value: unknown) => void;
  reads.steps.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const state = {
    workspaces: { activeId: "workspace", items: [] },
    workflows: { items: [{ id: "alpha", name: "Alpha", workspaceId: "workspace" }] },
  } as unknown as Partial<AppState>;
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState: state, children });
  const { result } = renderHook(
    () => ({
      first: useFilterValueOptions("workflowStep"),
      second: useFilterValueOptions("workflowStep"),
    }),
    { wrapper },
  );
  const count = reads.steps.mock.calls.length;
  await act(async () =>
    resolve({ steps: [{ id: "done", name: "Done", position: 0, color: "blue" }] }),
  );
  await waitFor(() => expect(result.current.first.length + result.current.second.length).toBe(2));
  expect(count).toBe(1);
});
