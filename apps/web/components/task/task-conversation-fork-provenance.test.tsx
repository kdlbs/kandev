import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  conversationForkIdFromTaskMetadata,
  TaskConversationForkProvenance,
} from "./task-conversation-fork-provenance";

vi.mock("./conversation-fork-provenance", () => ({
  ConversationForkProvenance: ({ forkId }: { forkId: string }) => (
    <div data-testid="task-fork-provenance" data-fork-id={forkId} />
  ),
}));

afterEach(cleanup);

describe("task conversation fork provenance", () => {
  it("reads only a non-empty server-authored fork reference", () => {
    expect(conversationForkIdFromTaskMetadata({ conversation_fork_id: "fork-1" })).toBe("fork-1");
    expect(conversationForkIdFromTaskMetadata({ conversation_fork_id: "" })).toBeNull();
    expect(conversationForkIdFromTaskMetadata({ conversation_fork_id: 42 })).toBeNull();
    expect(conversationForkIdFromTaskMetadata(null)).toBeNull();
  });

  it("renders provenance without depending on a task session or messages", () => {
    render(<TaskConversationForkProvenance metadata={{ conversation_fork_id: "fork-1" }} />);

    expect(screen.getByTestId("task-fork-provenance").getAttribute("data-fork-id")).toBe("fork-1");
  });
});
