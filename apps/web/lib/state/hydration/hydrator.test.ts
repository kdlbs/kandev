import { beforeEach, describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateUI } from "./hydrator";
import { defaultUIState } from "@/lib/state/slices/ui/ui-slice";
import type { AppState } from "@/lib/state/store";

function makeDraft(): AppState {
  // hydrateUI only touches UI-slice fields; an empty object cast satisfies
  // the rest without dragging the full AppState shape into this test.
  return { ...defaultUIState } as unknown as AppState;
}

describe("hydrateUI — quick chat name overlay", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("overlays a locally-renamed name onto the SSR-provided session name", () => {
    window.localStorage.setItem(
      "kandev.quickChat.names",
      JSON.stringify({ "sess-1": "My custom name" }),
    );

    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [{ sessionId: "sess-1", workspaceId: "ws-1", name: "Agent A - Chat 1" }],
        },
      });
    });

    expect(result.quickChat.sessions[0].name).toBe("My custom name");
  });

  it("keeps the SSR-provided name when no local rename exists", () => {
    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [{ sessionId: "sess-2", workspaceId: "ws-1", name: "Agent A - Chat 1" }],
        },
      });
    });

    expect(result.quickChat.sessions[0].name).toBe("Agent A - Chat 1");
  });

  it("only overlays sessions that have a stored rename, leaving siblings untouched", () => {
    window.localStorage.setItem(
      "kandev.quickChat.names",
      JSON.stringify({ "sess-a": "Renamed A" }),
    );

    const result = produce(makeDraft(), (draft: Draft<AppState>) => {
      hydrateUI(draft, {
        quickChat: {
          isOpen: false,
          activeSessionId: null,
          sessions: [
            { sessionId: "sess-a", workspaceId: "ws-1", name: "Original A" },
            { sessionId: "sess-b", workspaceId: "ws-1", name: "Original B" },
          ],
        },
      });
    });

    expect(result.quickChat.sessions.map((s) => s.name)).toEqual(["Renamed A", "Original B"]);
  });
});
