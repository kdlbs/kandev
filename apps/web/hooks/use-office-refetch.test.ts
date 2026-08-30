import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useOfficeRefetch } from "./use-office-refetch";

afterEach(() => cleanup());

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, { children });
}

/**
 * office.task.updated fires `task:<id>` and `dashboard` synchronously in the
 * same handler. Both a task-detail listener and a dashboard listener must
 * observe their own type from that one WS event — regression coverage for
 * the same-tick batching in setOfficeRefetchTrigger (AC-27).
 */
describe("useOfficeRefetch", () => {
  it("notifies every listener whose type was fired in the same tick", async () => {
    const taskCallback = vi.fn();
    const dashboardCallback = vi.fn();

    const { result } = renderHook(
      () => {
        useOfficeRefetch("task:t-1", taskCallback);
        useOfficeRefetch("dashboard", dashboardCallback);
        return useAppStoreApi();
      },
      { wrapper },
    );

    act(() => {
      result.current.getState().setOfficeRefetchTrigger("task:t-1");
      result.current.getState().setOfficeRefetchTrigger("dashboard");
    });

    await waitFor(() => expect(taskCallback).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(dashboardCallback).toHaveBeenCalledTimes(1));
  });

  it("prefix-matches a task channel without notifying an unrelated task", async () => {
    const matchingCallback = vi.fn();
    const otherCallback = vi.fn();

    const { result } = renderHook(
      () => {
        useOfficeRefetch("task:t-1", matchingCallback);
        useOfficeRefetch("task:t-2", otherCallback);
        return useAppStoreApi();
      },
      { wrapper },
    );

    act(() => {
      result.current.getState().setOfficeRefetchTrigger("task:t-1");
    });

    await waitFor(() => expect(matchingCallback).toHaveBeenCalledTimes(1));
    expect(otherCallback).not.toHaveBeenCalled();
  });
});
