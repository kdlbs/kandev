import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import { ConversationForkPreview } from "./conversation-fork-preview";
import { ConversationForkChip } from "./conversation-fork-chip";

const onApplySelection = vi.fn().mockResolvedValue(true);
const onRangeStartChange = vi.fn();
const onRemove = vi.fn();
const onModelChange = vi.fn();
const onConsumed = vi.fn();

function createFork(): ConversationForkFormContext {
  return {
    snapshot: {
      descriptor: {
        id: "fork-1",
        source_task_id: "task-1",
        source_session_id: "session-1",
        source_message_id: "message-2",
        source_revision: 2,
        compiler_version: "conversation-fork-v1",
        content_hash: "hash-1",
        message_count: 2,
        text_bytes: 36,
        omissions: { tool_protocol: 3 },
        attachments: [],
        estimate: {
          estimated_tokens: 24,
          method: "o200k_base:conversation-fork-v1",
          attachments_unmeasured: true,
        },
        created_at: "2026-09-23T10:00:00Z",
        expires_at: "2026-09-24T10:00:00Z",
        state: "draft",
      },
      content: {
        content: "Historical user text\nHistorical assistant answer",
        content_hash: "hash-1",
        compiler_version: "conversation-fork-v1",
      },
    },
    snapshotError: null,
    source: {
      taskId: "task-1",
      sessionId: "session-1",
      title: "Source task",
      revision: 2,
      cutoffMessageId: "message-2",
      cutoffTurnComplete: true,
      boundaries: [
        {
          finalized: true,
          message: {
            id: "message-1",
            session_id: "session-1" as never,
            task_id: "task-1" as never,
            author_type: "user",
            type: "message",
            content: "Question",
            created_at: "2026-09-23T09:00:00Z",
          },
        },
        {
          finalized: true,
          message: {
            id: "message-2",
            session_id: "session-1" as never,
            task_id: "task-1" as never,
            author_type: "agent",
            type: "message",
            content: "Answer",
            created_at: "2026-09-23T10:00:00Z",
          },
        },
      ],
      attachments: [
        { source_id: "attachment-1", name: "source.txt", size: 10, available: true },
        { source_id: "attachment-2", name: "missing.txt", available: false },
      ],
    },
    selection: { includeToolEvidence: false, attachmentIds: [] },
    creationRequestId: "create-1",
    attachmentsLoading: false,
    onPreview: vi.fn(),
    onRemove,
    onApplySelection,
    onRangeStartChange,
    onModelChange,
    onConsumed,
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  onApplySelection.mockResolvedValue(true);
});

describe("ConversationForkPreview", () => {
  it("shows the frozen content and informational estimate uncertainty", () => {
    render(<ConversationForkPreview fork={createFork()} onBack={vi.fn()} />);

    expect(screen.getByTestId("conversation-fork-content").textContent).toContain(
      "Historical user text",
    );
    expect(screen.getByText("Tokenizer: o200k_base:conversation-fork-v1")).toBeTruthy();
    expect(screen.getByText("Attachment token cost is not measured.")).toBeTruthy();
    expect(
      screen.getByText("Fork estimate only. New instructions and runtime overhead are excluded."),
    ).toBeTruthy();
    expect(screen.queryByText(/known model context/)).toBeNull();
  });

  it("applies a changed range, selected evidence, and explicit attachment choices", async () => {
    const onBack = vi.fn();
    render(<ConversationForkPreview fork={createFork()} onBack={onBack} />);

    fireEvent.change(screen.getByRole("combobox", { name: "Start from" }), {
      target: { value: "message-1" },
    });
    fireEvent.click(screen.getByRole("checkbox", { name: "Include selected tool evidence" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "source.txt" }));
    fireEvent.click(screen.getByRole("button", { name: "Apply selection" }));

    expect(onRangeStartChange).toHaveBeenCalledWith("message-1");
    expect(onApplySelection).toHaveBeenCalledWith({
      startMessageId: "message-1",
      includeToolEvidence: true,
      attachmentIds: ["attachment-1"],
    });
    await waitFor(() => expect(onBack).toHaveBeenCalled());
    expect(screen.getByText("1 unavailable file is not included.")).toBeTruthy();
  });

  it("shows an approximate percentage only when the model context limit is known", () => {
    const fork = createFork();
    fork.snapshot.descriptor.estimate.model_id = "model-1";
    fork.snapshot.descriptor.estimate.context_limit = 120;
    render(<ConversationForkPreview fork={fork} onBack={vi.fn()} />);

    expect(screen.getByText("Approximately 20% of known model context")).toBeTruthy();
  });

  it("keeps controls compact for fine pointers and enlarges them for coarse pointers", () => {
    render(<ConversationForkPreview fork={createFork()} onBack={vi.fn()} />);

    const range = screen.getByRole("combobox", { name: "Start from" });
    expect(range.className).toContain("h-7");
    expect(range.className).toContain("[@media(pointer:coarse)]:min-h-11");
    const apply = screen.getByRole("button", { name: "Apply selection" });
    expect(apply.className).toContain("h-7");
    expect(apply.className).toContain("[@media(pointer:coarse)]:min-h-11");
  });

  it("shows the frozen source range and a short historical preview in the creation chip", () => {
    const fork = createFork();
    fork.selection.startMessageId = "message-1";
    render(<ConversationForkChip fork={fork} />);

    expect(screen.getByTestId("conversation-fork-chip").textContent).toContain(
      "From User message 1",
    );
    expect(screen.getByTestId("conversation-fork-chip-history-preview").textContent).toContain(
      "Historical user text",
    );
    const preview = screen.getByRole("button", { name: "Preview" });
    expect(preview.className).toContain("h-7");
    expect(preview.className).toContain("[@media(pointer:coarse)]:min-h-11");
  });
});
