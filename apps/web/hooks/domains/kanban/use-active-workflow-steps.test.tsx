/**
 * Regression tests for `useActiveWorkflowSteps`.
 *
 * The hook resolves the *contextually active* workflow's steps from the
 * `qk.kanban.multi()` cache. After the Zustand→TQ migration deleted the old
 * `kanban.workflowId` field, a naive port read only the homepage filter
 * (`workflows.activeId`). That broke the task page: a task opened in a workflow
 * that is NOT the current homepage filter rendered an empty stepper.
 *
 * These tests pin the corrected resolution: prefer the workflow that owns the
 * active task, falling back to the homepage filter only when there is no
 * active-task context.
 */
import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";
import { useActiveWorkflowSteps } from "./use-kanban-tasks";

const step = (id: string, title: string, position: number) => ({
  id,
  title,
  color: "bg-neutral-400",
  position,
});

const task = (id: string, workflowStepId: string) => ({
  id,
  workflowStepId,
  title: id,
  position: 0,
});

const MULTI: KanbanMultiData = {
  snapshots: {
    "wf-task": {
      workflowId: "wf-task",
      workflowName: "Task Workflow",
      steps: [step("s-backlog", "Backlog", 0), step("s-progress", "In Progress", 1)],
      tasks: [task("task-1", "s-backlog")],
    },
    "wf-filter": {
      workflowId: "wf-filter",
      workflowName: "Filter Workflow",
      steps: [step("f-todo", "Todo", 0)],
      tasks: [],
    },
  },
};

function makeWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(
      QueryClientProvider,
      { client: qc },
      React.createElement(StateProvider, null, children),
    );
  };
}

/**
 * Renders both the hook under test and the store API from the same tree, so the
 * test can drive client state (active workspace / workflow / task) and read the
 * resolved steps without writing refs during render.
 */
function useHarness() {
  const api = useAppStoreApi();
  const steps = useActiveWorkflowSteps();
  return { api, steps };
}

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(qk.kanban.multi(), MULTI);
  const rendered = renderHook(() => useHarness(), { wrapper: makeWrapper(qc) });
  // The active workspace gates the query; set it before asserting.
  act(() => {
    rendered.result.current.api.getState().setActiveWorkspace("ws-1");
  });
  return rendered;
}

describe("useActiveWorkflowSteps", () => {
  it("returns the active task's workflow steps even when no homepage filter matches", () => {
    const { result } = setup();
    // Homepage filter points at a DIFFERENT workflow; the task page must still
    // resolve the task's own workflow.
    act(() => {
      result.current.api.getState().setActiveWorkflow("wf-filter");
      result.current.api.getState().setActiveTask("task-1");
    });
    expect(result.current.steps.map((s) => s.title)).toEqual(["Backlog", "In Progress"]);
  });

  it("falls back to the homepage filter workflow when there is no active task", () => {
    const { result } = setup();
    act(() => {
      result.current.api.getState().setActiveWorkflow("wf-filter");
    });
    expect(result.current.steps.map((s) => s.title)).toEqual(["Todo"]);
  });

  it("returns [] when neither an active task nor a filter is set", () => {
    const { result } = setup();
    expect(result.current.steps).toEqual([]);
  });
});
