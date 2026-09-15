import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskPreviewFeedbackSnapshot } from "@/lib/types/http";

const api = vi.hoisted(() => ({
  getTaskPreviewFeedback: vi.fn(),
  createTaskPreviewFeedback: vi.fn(),
  updateTaskPreviewFeedback: vi.fn(),
  deleteTaskPreviewFeedback: vi.fn(),
  clearTaskPreviewFeedback: vi.fn(),
}));
vi.mock("@/lib/api/domains/preview-feedback-api", () => api);

import { usePreviewFeedback } from "./use-preview-feedback";

const snapshot = (revision: number): TaskPreviewFeedbackSnapshot => ({
  task_id: "task-1",
  revision,
  items: [
    {
      id: `feedback-${revision}`,
      task_id: "task-1",
      kind: "text",
      comment: `comment ${revision}`,
      source_kind: "browser",
      source_label: "Local app",
      page_route: "/products",
      page_title: "Products",
      selected_text: "Choose a plan",
      text_anchor: {
        start: { node_path: [0], offset: 0 },
        end: { node_path: [0], offset: 13 },
      },
      version: 1,
      created_at: "2026-09-15T00:00:00Z",
      updated_at: "2026-09-15T00:00:00Z",
    },
  ],
});

function wrapper({ children }: { children: React.ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function useTwoConsumers() {
  const store = useAppStoreApi();
  return { store, first: usePreviewFeedback("task-1"), second: usePreviewFeedback("task-1") };
}

afterEach(cleanup);

describe("usePreviewFeedback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getTaskPreviewFeedback.mockResolvedValue(snapshot(1));
  });

  it("deduplicates loading and shares one task snapshot", async () => {
    const { result } = renderHook(useTwoConsumers, { wrapper });
    act(() => result.current.store.getState().setConnectionStatus("connected"));

    await waitFor(() => expect(result.current.first.items).toHaveLength(1));
    expect(api.getTaskPreviewFeedback).toHaveBeenCalledTimes(1);
    expect(result.current.second.items).toEqual(result.current.first.items);
  });

  it("refreshes the collection after reconnect", async () => {
    api.getTaskPreviewFeedback.mockResolvedValue(snapshot(2));
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return { store, feedback: usePreviewFeedback("task-1") };
      },
      { wrapper },
    );
    act(() => {
      result.current.store.getState().setTaskPreviewFeedback("task-1", snapshot(1));
      result.current.store.getState().setConnectionStatus("connected");
    });

    await waitFor(() => expect(result.current.feedback.snapshot?.revision).toBe(2));
    expect(api.getTaskPreviewFeedback).toHaveBeenCalledWith("task-1");
  });
});
