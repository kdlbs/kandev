import { describe, expect, it } from "vitest";
import { getNewArrivalIds, type ArrivalSnapshot } from "./chat-motion-state";

// @covers AC-UI-CHAT-MOTION-001.2, AC-UI-CHAT-MOTION-001.3
describe("chat arrival eligibility", () => {
  const baseline: ArrivalSnapshot = { sessionId: "session", ids: ["user", "tool"], live: true };
  it("finds new child messages even when the turn group stays the same", () => {
    expect([
      ...getNewArrivalIds(baseline, { ...baseline, ids: ["user", "tool", "tool2"] }),
    ]).toEqual(["tool2"]);
  });
  it("does not animate initial history, prepends, session switches or refresh", () => {
    expect(getNewArrivalIds(null, baseline).size).toBe(0);
    expect(getNewArrivalIds(baseline, { ...baseline, ids: ["older", "user", "tool"] }).size).toBe(
      0,
    );
    expect(getNewArrivalIds(baseline, { ...baseline, sessionId: "other", ids: ["new"] }).size).toBe(
      0,
    );
    expect(
      getNewArrivalIds(baseline, { ...baseline, live: false, ids: [...baseline.ids, "new"] }).size,
    ).toBe(0);
  });
  it("does not replay hidden deliveries when activated", () => {
    expect(
      getNewArrivalIds({ ...baseline, live: false }, { ...baseline, ids: [...baseline.ids, "new"] })
        .size,
    ).toBe(0);
  });
});

import { cleanup, render } from "@testing-library/react";
import { afterEach, vi } from "vitest";
import { ChatMotionItem, ChatMotionProvider } from "./chat-motion";
vi.mock("@/hooks/use-chat-motion", () => ({ useChatMotion: () => true }));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("animates a new row once and cancels it when the transcript hides", () => {
  const cancel = vi.fn();
  const animate = vi.fn(() => ({ cancel }));
  Object.defineProperty(Element.prototype, "animate", { configurable: true, value: animate });
  const tree = (ids: string[], live = true) => (
    <ChatMotionProvider sessionId="session" messages={ids.map((id) => ({ id }))} live={live}>
      {ids.map((id) => (
        <ChatMotionItem key={id} messageId={id}>
          {id}
        </ChatMotionItem>
      ))}
    </ChatMotionProvider>
  );
  const view = render(tree(["old"]));
  expect(animate).not.toHaveBeenCalled();
  view.rerender(tree(["old", "new"]));
  expect(animate).toHaveBeenCalledTimes(1);
  view.rerender(tree(["old", "new"]));
  expect(animate).toHaveBeenCalledTimes(1);
  view.rerender(tree(["old", "new"], false));
  expect(cancel).toHaveBeenCalledTimes(1);
  view.rerender(tree(["old", "new"]));
  expect(animate).toHaveBeenCalledTimes(1);
});
