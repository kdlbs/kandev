import { useSyncExternalStore } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const harness = vi.hoisted(() => {
  const state = {
    tasks: { activeSessionId: "session-a" },
    taskSessions: { items: {} },
    prompts: { items: [] as Array<Record<string, unknown>> },
  };
  const listeners = new Set<() => void>();
  return {
    state,
    subscribe(listener: () => void) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    updatePrompts(items: Array<Record<string, unknown>>) {
      state.prompts.items = items;
      for (const listener of listeners) listener();
    },
  };
});
const MENTION_TEST_ID = "custom-prompt-mention";

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: typeof harness.state) => T) =>
    useSyncExternalStore(
      harness.subscribe,
      () => selector(harness.state),
      () => selector(harness.state),
    ),
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({
    prompts: useSyncExternalStore(
      harness.subscribe,
      () => harness.state.prompts.items,
      () => harness.state.prompts.items,
    ),
    loaded: true,
    loading: false,
  }),
}));

vi.mock("@/hooks/domains/session/use-session-prompts", () => ({
  useSessionPrompts: () => ({
    prompts: [
      {
        id: "message-1",
        session_id: "session-a",
        task_id: "task-1",
        author_type: "user",
        content: "Review @daily",
        type: "message",
        created_at: "2026-01-01T12:00:00.000Z",
      },
    ],
    isLoading: false,
    fetchFailed: false,
    retryPrompts: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-lazy-load-prompts", () => ({
  useLazyLoadPrompts: () => ({
    loadMore: vi.fn(),
    hasMore: false,
    isLoadingMore: false,
  }),
}));

vi.mock("@/hooks/domains/session/use-session-turns", () => ({
  useSessionTurnsState: () => ({ turns: [], isHydrated: true }),
}));

vi.mock("@/hooks/domains/session/use-message-favorite", () => ({
  useMessageFavorite: () => ({ isFavorite: false }),
}));

import { PromptHistoryPanelContent } from "./prompt-history-panel-content";
import { PromptMentionText } from "./chat/messages/prompt-mention-components";

afterEach(() => {
  cleanup();
});

describe("PromptHistoryPanelContent prompt mention reactivity", () => {
  it("updates recognition and preview content from mounted store changes", () => {
    render(<PromptHistoryPanelContent />);
    expect(screen.queryByTestId(MENTION_TEST_ID)).toBeNull();

    act(() => {
      harness.updatePrompts([
        {
          id: "daily",
          name: "daily",
          content: "Initial prompt content",
          builtin: false,
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        },
      ]);
    });

    const mention = screen.getByTestId(MENTION_TEST_ID);
    expect(mention.getAttribute("data-prompt-name")).toBe("daily");
    fireEvent.click(mention);
    expect(screen.getByText("Initial prompt content")).toBeTruthy();

    act(() => {
      harness.updatePrompts([
        {
          id: "daily",
          name: "daily",
          content: "Updated prompt content",
          builtin: false,
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:01:00.000Z",
        },
      ]);
    });

    expect(screen.getByText("Updated prompt content")).toBeTruthy();
    expect(screen.queryByText("Initial prompt content")).toBeNull();
  });
  it("resets preview state when the mention identity changes", () => {
    const promptNames = ["daily", "weekly"];
    harness.updatePrompts([
      { name: "daily", content: "Daily prompt" },
      { name: "weekly", content: "Weekly prompt" },
    ]);
    const { rerender } = render(<PromptMentionText text="@daily" promptNames={promptNames} />);

    fireEvent.click(screen.getByTestId(MENTION_TEST_ID));
    expect(screen.getByText("Daily prompt")).toBeTruthy();

    rerender(<PromptMentionText text="@weekly" promptNames={promptNames} />);
    expect(screen.getByTestId(MENTION_TEST_ID).getAttribute("aria-expanded")).toBe("false");
  });
});
