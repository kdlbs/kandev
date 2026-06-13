import { afterEach, describe, expect, it } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { useUserMessageNavigation } from "./use-message-navigation";

afterEach(() => cleanup());

const SESSION_ID = "sess-1";

function makeMessage(id: string, authorType: Message["author_type"]): Message {
  return {
    id,
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId("task-1"),
    author_type: authorType,
    content: "",
    type: "message",
    created_at: "2026-06-12T00:00:00Z",
  };
}

// Render the hook alongside setMessages so a test can seed the store and read
// the resolved navigation within the same render tree.
function renderNav(currentMessageId: string, sid: string | null = SESSION_ID) {
  function wrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, null, children);
  }
  return renderHook(
    () => {
      const setMessages = useAppStore((s) => s.setMessages);
      const nav = useUserMessageNavigation(sid, currentMessageId);
      return { setMessages, nav };
    },
    { wrapper },
  );
}

const NONE = { hasPrevious: false, hasNext: false, previousId: null, nextId: null };

type Nav = ReturnType<typeof renderNav>;

function seed(result: Nav["result"], messages: Message[]) {
  act(() => result.current.setMessages(SESSION_ID, messages));
}

describe("useUserMessageNavigation", () => {
  it("returns no navigation when the session id is null", () => {
    const { result } = renderNav("u1", null);
    expect(result.current.nav).toEqual(NONE);
  });

  it("returns no navigation when the session has no messages", () => {
    const { result } = renderNav("u1");
    expect(result.current.nav).toEqual(NONE);
  });

  it("returns no navigation for a lone user message", () => {
    const { result } = renderNav("u1");
    seed(result, [makeMessage("u1", "user")]);
    expect(result.current.nav).toEqual(NONE);
  });

  it("resolves both neighbours for a middle user message", () => {
    const { result } = renderNav("u2");
    seed(
      result,
      ["u1", "u2", "u3"].map((id) => makeMessage(id, "user")),
    );
    expect(result.current.nav).toEqual({
      hasPrevious: true,
      hasNext: true,
      previousId: "u1",
      nextId: "u3",
    });
  });

  it("resolves only next for the first user message", () => {
    const { result } = renderNav("u1");
    seed(
      result,
      ["u1", "u2"].map((id) => makeMessage(id, "user")),
    );
    expect(result.current.nav).toMatchObject({
      hasPrevious: false,
      previousId: null,
      nextId: "u2",
    });
  });

  it("resolves only previous for the last user message", () => {
    const { result } = renderNav("u2");
    seed(
      result,
      ["u1", "u2"].map((id) => makeMessage(id, "user")),
    );
    expect(result.current.nav).toMatchObject({ hasNext: false, previousId: "u1", nextId: null });
  });

  it("ignores agent messages when computing neighbours", () => {
    const { result } = renderNav("u2");
    seed(result, [
      makeMessage("u1", "user"),
      makeMessage("a1", "agent"),
      makeMessage("u2", "user"),
      makeMessage("a2", "agent"),
      makeMessage("u3", "user"),
    ]);
    expect(result.current.nav).toMatchObject({ previousId: "u1", nextId: "u3" });
  });

  it("returns no navigation when the current message is not a user message", () => {
    const { result } = renderNav("a1");
    seed(result, [
      makeMessage("u1", "user"),
      makeMessage("a1", "agent"),
      makeMessage("u2", "user"),
    ]);
    expect(result.current.nav).toEqual(NONE);
  });
});
