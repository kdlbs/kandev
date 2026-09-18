import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { AssistantBinding, AssistantMemory } from "@/lib/api/domains/assistant-api";
import { MemoryPanel } from "./memory-panel";
const fixture = vi.hoisted(() => ({
  page: vi.fn(),
  edit: vi.fn(),
  forget: vi.fn(),
  source: vi.fn(),
  comments: vi.fn(),
}));
vi.mock("@/hooks/domains/orchestration/use-assistant", () => ({ useAssistantPage: fixture.page }));
vi.mock("@/lib/api/domains/assistant-api", () => ({
  editAssistantMemory: fixture.edit,
  forgetAssistantMemory: fixture.forget,
  getAssistantMemorySource: fixture.source,
}));
vi.mock("@/lib/api/domains/orchestration-conversation-api", () => ({
  getConversationCommentPage: fixture.comments,
}));
const binding = {
  id: "b",
  owner_user_id: "owner",
  conversation_id: "conversation",
  home_workspace_id: "ws",
  version: 1,
} as AssistantBinding;
const memory: AssistantMemory = {
  id: "memory",
  key: "Example preference",
  content: "Use short headings.",
  scope: "workspace",
  scope_id: "ws",
  confirmed: true,
  revision: 2,
  owner_user_id: "owner",
  source_comment_id: "source",
  priority: 0,
  updated_at: "2026-09-18T12:00:00Z",
};
const refresh = vi.fn();
function page(revision = 2) {
  return {
    entries: [{ ...memory, revision }],
    loading: false,
    loaded: true,
    nextCursor: "",
    refresh,
    loadMore: vi.fn(),
  };
}
beforeEach(() => {
  fixture.page.mockReset().mockReturnValue(page());
  fixture.edit.mockReset().mockResolvedValue({});
  fixture.forget.mockReset().mockResolvedValue({});
  fixture.source.mockReset().mockResolvedValue({
    body: "Please use short headings for examples.",
    created_at: memory.updated_at,
  });
  fixture.comments.mockReset().mockResolvedValue({
    comments: [
      {
        id: "source",
        authorType: "user",
        authorId: "owner",
        content: "Please use short headings for examples.",
      },
    ],
  });
  refresh.mockReset();
});
afterEach(cleanup);
it("keeps the opened edit revision despite refresh, shows provenance, and forgets the current revision", async () => {
  const view = render(<MemoryPanel binding={binding} revision={1} />);
  fireEvent.click(screen.getByRole("button", { name: "Original instruction" }));
  expect(await screen.findByText("Please use short headings for examples.")).toBeTruthy();
  expect(fixture.source).toHaveBeenCalledWith("memory");
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  fireEvent.change(screen.getByLabelText("What should be remembered"), {
    target: { value: "Use concise example headings." },
  });
  fixture.page.mockReturnValue(page(3));
  view.rerender(<MemoryPanel binding={binding} revision={2} />);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
  });
  expect(fixture.edit).toHaveBeenCalledWith(
    "memory",
    expect.objectContaining({
      expected_revision: 2,
      content: "Use concise example headings.",
      source_comment_id: "source",
      scope_id: "ws",
    }),
  );
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Forget" }));
  });
  expect(fixture.forget).toHaveBeenCalledWith("memory", 3);
  expect(refresh).toHaveBeenCalledTimes(2);
});
