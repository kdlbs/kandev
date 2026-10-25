import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";

vi.mock("./inbox-history-row", () => ({
  InboxHistoryRow: () => null,
}));

import { InboxHistoryList } from "./inbox-history-list";

function bundle(): InboxHistoryBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "COMPLETED",
    task_title: "Task",
    created_at: "2026-09-03T04:54:02Z",
    context: "",
    messages: [],
    kind: "clarification",
    reason: "session_ended",
    asking_turn_id: "turn-1",
  };
}

afterEach(() => cleanup());

describe("InboxHistoryList pagination", () => {
  it("offers a load-more action for a truncated page", () => {
    const onLoadMore = vi.fn();
    render(
      <InboxHistoryList
        bundles={[bundle()]}
        hasMore
        isLoadingMore={false}
        loadMoreError={false}
        onLoadMore={onLoadMore}
      />,
    );

    fireEvent.click(screen.getByTestId("inbox-history-load-more"));

    expect(onLoadMore).toHaveBeenCalledOnce();
  });

  it("keeps the action disabled and reports a page error while loading more", () => {
    render(
      <InboxHistoryList
        bundles={[bundle()]}
        hasMore
        isLoadingMore
        loadMoreError
        onLoadMore={vi.fn()}
      />,
    );

    expect((screen.getByTestId("inbox-history-load-more") as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getByTestId("inbox-history-load-more-error").getAttribute("role")).toBe("alert");
  });
});
