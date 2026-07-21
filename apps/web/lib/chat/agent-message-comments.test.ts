import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  createMessageTextAnchor,
  getMessageCommentDecorations,
  getMessageSelection,
  isSelectableAgentMessage,
  resolveMessageTextAnchor,
} from "./agent-message-comments";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "reply-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "message",
    content: "A settled answer with useful detail.",
    created_at: "2026-07-20T00:00:00Z",
    ...overrides,
  };
}

describe("agent message comment anchors", () => {
  it("captures nearby text context and resolves unchanged content", () => {
    const content = "A settled answer with useful detail.";
    const start = content.indexOf("answer");
    const end = start + "answer".length;
    const anchor = createMessageTextAnchor("reply-1", content, start, end);

    expect(anchor).toEqual({
      messageId: "reply-1",
      start,
      end,
      selectedText: "answer",
      prefix: "A settled ",
      suffix: " with useful detail.",
    });
    expect(resolveMessageTextAnchor(anchor, content)).toEqual({ start, end });
  });

  it("falls back to nearby quote text when content shifted before selection", () => {
    const anchor = createMessageTextAnchor("reply-1", "before select after", 7, 13);

    expect(resolveMessageTextAnchor(anchor, "new before select after")).toEqual({
      start: 11,
      end: 17,
    });
  });

  it("uses prefix and suffix context when selected text repeats", () => {
    const original = "first answer; second answer; trailing";
    const start = original.lastIndexOf("answer");
    const anchor = createMessageTextAnchor("reply-1", original, start, start + 6);

    expect(resolveMessageTextAnchor(anchor, "first answer; revised answer; trailing")).toEqual({
      start: 22,
      end: 28,
    });
  });

  it("only permits settled ordinary agent prose", () => {
    expect(isSelectableAgentMessage(message(), false, false)).toBe(true);
    expect(isSelectableAgentMessage(message({ type: undefined }), false, false)).toBe(true);
    expect(isSelectableAgentMessage(message({ author_type: "user" }), false, false)).toBe(false);
    expect(isSelectableAgentMessage(message({ type: "thinking" }), false, false)).toBe(false);
    for (const type of ["tool_call", "status", "agent_plan"] as const) {
      expect(isSelectableAgentMessage(message({ type }), false, false)).toBe(false);
    }
    expect(isSelectableAgentMessage(message(), true, false)).toBe(false);
    expect(isSelectableAgentMessage(message(), false, true)).toBe(false);
    // Rich blocks render outside the prose annotation surface. Their presence
    // must not disable selection of an otherwise ordinary reply.
    expect(
      isSelectableAgentMessage(message({ metadata: { diff: "--- a/file" } }), false, false),
    ).toBe(true);
    expect(
      isSelectableAgentMessage(
        message({ metadata: { content_blocks: [{ type: "text" }] } }),
        false,
        false,
      ),
    ).toBe(true);
  });
});

describe("message-local DOM anchors", () => {
  it("rejects a cross-message selection and captures a local selection", () => {
    const root = document.createElement("div");
    root.innerHTML = "<p>First settled reply.</p><p>Second reply.</p>";
    document.body.append(root);
    const firstText = root.querySelector("p")?.firstChild;
    expect(firstText).toBeTruthy();
    const selection = window.getSelection();
    const range = document.createRange();
    range.setStart(firstText!, 6);
    range.setEnd(firstText!, 13);
    selection?.removeAllRanges();
    selection?.addRange(range);

    const captured = getMessageSelection(root, selection);
    expect(captured?.selectedText).toBe("settled");
    const secondText = root.querySelectorAll("p")[1].firstChild;
    const crossRange = document.createRange();
    crossRange.setStart(firstText!, 6);
    crossRange.setEnd(secondText!, 3);
    selection?.removeAllRanges();
    selection?.addRange(crossRange);
    expect(getMessageSelection(root.querySelector("p")!, selection)).toBeNull();
    selection?.removeAllRanges();
    root.remove();
  });

  it("resolves decoration ranges without rewriting React-owned nodes", () => {
    const root = document.createElement("div");
    root.innerHTML = "<p>A settled </p><p>answer with detail.</p>";
    const renderedHtml = root.innerHTML;
    const comment = {
      id: "comment-1",
      sessionId: "session-1",
      source: "agent-message" as const,
      messageId: "reply-1",
      selectedText: "answer",
      text: "Please expand this.",
      createdAt: "2026-07-20T00:00:00Z",
      status: "pending" as const,
      anchor: createMessageTextAnchor("reply-1", "A settled answer with detail.", 10, 16),
    };
    const decorations = getMessageCommentDecorations(root, [comment]);
    expect(decorations).toHaveLength(1);
    expect(decorations[0].range.toString()).toBe("answer");
    expect(root.innerHTML).toBe(renderedHtml);
    expect(root.textContent).toBe("A settled answer with detail.");

    // Recomputing for a virtualized row remount remains side-effect free.
    expect(getMessageCommentDecorations(root, [comment])).toHaveLength(1);
    expect(root.innerHTML).toBe(renderedHtml);
  });

  it("keeps highlight restoration bounded to the message body", () => {
    const words = Array.from({ length: 40 }, (_, index) => `reply-${index}`).join(" ");
    const root = document.createElement("div");
    root.textContent = words;
    const comments = Array.from({ length: 40 }, (_, index) => {
      const selectedText = `reply-${index}`;
      const start = words.indexOf(selectedText);
      return {
        id: `comment-${index}`,
        sessionId: "session-1",
        source: "agent-message" as const,
        messageId: "reply-1",
        selectedText,
        text: "feedback",
        createdAt: "2026-07-20T00:00:00Z",
        status: "pending" as const,
        anchor: createMessageTextAnchor("reply-1", words, start, start + selectedText.length),
      };
    });

    expect(getMessageCommentDecorations(root, comments)).toHaveLength(40);
    expect(root.querySelectorAll("mark[data-agent-message-comment-id]")).toHaveLength(0);
    expect(root.textContent).toBe(words);
  });
});
