import { describe, expect, it } from "vitest";

import { DEV_SERVER_PANEL_ID } from "./constants";
import { filterEphemeral } from "./serializer";
import type { LayoutState } from "./types";

/**
 * The dev-server output panel reuses the `terminal` component but owns no PTY.
 * The terminal branch of `normalizePanel` rewrites ids, params and titles so a
 * restored shell reconnects to a fresh PTY — applying that here would turn the
 * output view into an empty terminal named "Terminal".
 */
function layoutWithDevServer(): LayoutState {
  return {
    columns: [
      {
        id: "col",
        groups: [
          {
            id: "group",
            panels: [
              {
                id: DEV_SERVER_PANEL_ID,
                component: "terminal",
                title: "Dev Server",
                params: { type: DEV_SERVER_PANEL_ID },
              },
            ],
          },
        ],
      },
    ],
  } as unknown as LayoutState;
}

describe("dev-server panel persistence", () => {
  it("survives ephemeral filtering with its id, params and title intact", () => {
    const kept = filterEphemeral(layoutWithDevServer());

    const panels = kept.columns[0].groups[0].panels;
    expect(panels).toHaveLength(1);
    expect(panels[0].id).toBe(DEV_SERVER_PANEL_ID);
    expect(panels[0].title).toBe("Dev Server");
    expect(panels[0].params).toEqual({ type: DEV_SERVER_PANEL_ID });
    expect(panels[0].tabComponent).toBeUndefined();
  });

  it("does not gain a terminalId, which would make the close handler kill a shell", () => {
    const kept = filterEphemeral(layoutWithDevServer());

    expect(kept.columns[0].groups[0].panels[0].params?.terminalId).toBeUndefined();
  });
});
