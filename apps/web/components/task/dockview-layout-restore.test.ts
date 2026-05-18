import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  collectPhantomSessionIdsForEnv,
  sanitizeLayout,
  tryRestoreLayout,
} from "./dockview-layout-restore";
import * as localStorage from "@/lib/local-storage";

const VALID_COMPONENTS = new Set<string>(["chat", "files", "shell", "git", "terminal"]);
const PHANTOM_PANEL_ID = "session:phantom";
const ALIVE_PANEL_ID = "session:alive-1";

/**
 * Build a minimal valid SerializedDockview-shaped object — matches what
 * dockview's api.toJSON() produces for a 3-column layout (sidebar | center | right).
 */
function buildLayout(opts?: { centerSize?: number; sidebarSize?: number; rightSize?: number }) {
  return {
    grid: {
      root: {
        type: "branch" as const,
        size: 600,
        data: [
          {
            type: "leaf" as const,
            size: opts?.sidebarSize ?? 350,
            data: { id: "g-sidebar", views: ["files"], activeView: "files" },
          },
          {
            type: "leaf" as const,
            size: opts?.centerSize ?? 800,
            data: { id: "g-center", views: ["chat"], activeView: "chat" },
          },
          {
            type: "leaf" as const,
            size: opts?.rightSize ?? 450,
            data: { id: "g-right", views: ["git", "shell"], activeView: "git" },
          },
        ],
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      files: { id: "files", contentComponent: "files" },
      chat: { id: "chat", contentComponent: "chat" },
      git: { id: "git", contentComponent: "git" },
      shell: { id: "shell", contentComponent: "shell" },
    },
    activeGroup: "g-center",
  };
}

describe("sanitizeLayout - size validation", () => {
  it("returns the layout unchanged when all sizes are positive", () => {
    const layout = buildLayout();
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).not.toBeNull();
    expect(result.grid.root.data).toHaveLength(3);
  });

  it("returns null when a leaf node has size 0", () => {
    const layout = buildLayout({ centerSize: 0 });
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when a leaf node has negative size", () => {
    const layout = buildLayout({ sidebarSize: -50 });
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when a branch node has size 0", () => {
    const layout = buildLayout();
    layout.grid.root.size = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when nested branch has invalid size", () => {
    const layout = {
      grid: {
        root: {
          type: "branch" as const,
          size: 800,
          data: [
            {
              type: "leaf" as const,
              size: 350,
              data: { id: "g-sidebar", views: ["files"], activeView: "files" },
            },
            {
              type: "branch" as const,
              size: 0,
              data: [
                {
                  type: "leaf" as const,
                  size: 800,
                  data: { id: "g-center", views: ["chat"], activeView: "chat" },
                },
              ],
            },
          ],
        },
        height: 600,
        width: 1600,
        orientation: "HORIZONTAL" as const,
      },
      panels: {
        files: { id: "files", contentComponent: "files" },
        chat: { id: "chat", contentComponent: "chat" },
      },
      activeGroup: "g-center",
    };
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when grid.width is 0", () => {
    const layout = buildLayout();
    layout.grid.width = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("returns null when grid.height is 0", () => {
    const layout = buildLayout();
    layout.grid.height = 0;
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).toBeNull();
  });

  it("preserves existing component-validation behavior alongside size checks", () => {
    const layout = buildLayout();
    // Add an unknown panel — sanitizer should remove it but keep the rest.
    layout.panels = {
      ...layout.panels,
      // @ts-expect-error - injecting an extra unknown panel
      unknown: { id: "unknown", contentComponent: "unknown-component" },
    };
    const result = sanitizeLayout(layout, VALID_COMPONENTS);
    expect(result).not.toBeNull();
    expect(Object.keys(result.panels)).not.toContain("unknown");
  });
});

describe("sanitizeLayout - session panel handling", () => {
  const SESSION_PANEL_ID = "session:abc-123";

  function buildLayoutWithSession() {
    const layout = buildLayout();
    layout.grid.root.data[1].data.views = ["chat", SESSION_PANEL_ID];
    layout.panels = {
      ...layout.panels,
      // @ts-expect-error - injecting a session panel without a matching valid component
      [SESSION_PANEL_ID]: { id: SESSION_PANEL_ID, contentComponent: "session-panel" },
    };
    return layout;
  }

  it("keeps session:* panels by default (per-env restore)", () => {
    const result = sanitizeLayout(buildLayoutWithSession(), VALID_COMPONENTS);
    expect(result).not.toBeNull();
    expect(Object.keys(result.panels)).toContain(SESSION_PANEL_ID);
  });

  it("strips session:* panels when stripSessionPanels=true (global fallback)", () => {
    const result = sanitizeLayout(buildLayoutWithSession(), VALID_COMPONENTS, {
      stripSessionPanels: true,
    });
    expect(result).not.toBeNull();
    expect(Object.keys(result.panels)).not.toContain(SESSION_PANEL_ID);
    // The non-session panels stay so the rest of the layout is still usable.
    expect(Object.keys(result.panels)).toContain("chat");
    expect(Object.keys(result.panels)).toContain("files");
  });

  it("strips session:* panels even when contentComponent is a valid component (e.g. 'chat')", () => {
    // dockview serializes panels added via api.addPanel({ component: "chat" })
    // with contentComponent: "chat", which is in VALID_COMPONENTS. The strip
    // logic must key off the panel id, not the component name.
    const layout = buildLayout();
    layout.grid.root.data[1].data.views = ["chat", SESSION_PANEL_ID];
    layout.panels = {
      ...layout.panels,
      // @ts-expect-error - injecting a session panel with the chat component
      [SESSION_PANEL_ID]: { id: SESSION_PANEL_ID, contentComponent: "chat" },
    };
    const result = sanitizeLayout(layout, VALID_COMPONENTS, { stripSessionPanels: true });
    expect(result).not.toBeNull();
    expect(Object.keys(result.panels)).not.toContain(SESSION_PANEL_ID);
    expect(Object.keys(result.panels)).toContain("chat");
  });

  describe("excludeSessionIds (per-env restore filter)", () => {
    function buildLayoutWithTwoSessions(sidA: string, sidB: string) {
      const layout = buildLayout();
      layout.grid.root.data[1].data.views = [`session:${sidA}`, `session:${sidB}`];
      layout.grid.root.data[1].data.activeView = `session:${sidA}`;
      layout.panels = {
        files: { id: "files", contentComponent: "files" },
        chat: { id: "chat", contentComponent: "chat" },
        git: { id: "git", contentComponent: "git" },
        shell: { id: "shell", contentComponent: "shell" },
        [`session:${sidA}`]: { id: `session:${sidA}`, contentComponent: "chat" },
        [`session:${sidB}`]: { id: `session:${sidB}`, contentComponent: "chat" },
      };
      return layout;
    }

    it("strips session panels whose ids are in excludeSessionIds (phantom panels from deleted tasks)", () => {
      // Regression for "phantom panels from local/session storage": a stale
      // session id from a previously-deleted task can end up serialized in an
      // env-layout (e.g. via a debounced save firing during a task switch).
      // The env restore must drop session panels we know belong to another env.
      const aliveId = "alive-1";
      const phantomId = "phantom-deleted";
      const layout = buildLayoutWithTwoSessions(aliveId, phantomId);
      const result = sanitizeLayout(layout, VALID_COMPONENTS, {
        excludeSessionIds: new Set([phantomId]),
      });
      expect(result).not.toBeNull();
      const alivePanel = `session:${aliveId}`;
      expect(Object.keys(result.panels)).toContain(alivePanel);
      expect(Object.keys(result.panels)).not.toContain(`session:${phantomId}`);
      // The leaf's views list also drops the phantom.
      const centerLeaf = result.grid.root.data[1];
      expect(centerLeaf.data.views).toEqual([alivePanel]);
      expect(centerLeaf.data.activeView).toBe(alivePanel);
    });

    it("keeps unmapped session ids (preserves still-loading sessions across restore)", () => {
      // A session id present in the saved layout that the store has not yet
      // mapped (WS hasn't arrived) MUST be kept — otherwise the user loses
      // their valid chat tab on load. Reconcile cleans it up later if stale.
      const layout = buildLayoutWithTwoSessions("a", "b");
      const result = sanitizeLayout(layout, VALID_COMPONENTS, {
        excludeSessionIds: new Set<string>(),
      });
      expect(result).not.toBeNull();
      expect(Object.keys(result.panels)).toContain("session:a");
      expect(Object.keys(result.panels)).toContain("session:b");
    });

    it("ignores excludeSessionIds when undefined (preserves default per-env behavior)", () => {
      // Without the option, behavior is unchanged: session panels are kept.
      const layout = buildLayoutWithTwoSessions("a", "b");
      const result = sanitizeLayout(layout, VALID_COMPONENTS);
      expect(result).not.toBeNull();
      expect(Object.keys(result.panels)).toContain("session:a");
      expect(Object.keys(result.panels)).toContain("session:b");
    });

    it("rejects mixing stripSessionPanels with excludeSessionIds at the type level", () => {
      // The two options are intent-mutually-exclusive; the type union enforces
      // it so a caller can't silently get strip-wins behavior.
      const layout = buildLayoutWithTwoSessions("a", "b");
      const result = sanitizeLayout(layout, VALID_COMPONENTS, {
        stripSessionPanels: true,
        // @ts-expect-error - the discriminated union forbids passing both
        excludeSessionIds: new Set(["a"]),
      });
      // Compile-time guard is the contract; this just asserts the call still
      // returns a usable value so the test isn't a no-op at runtime.
      expect(result).not.toBeNull();
    });
  });
});

describe("tryRestoreLayout - phantom session panel filtering", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  function makeFakeApi() {
    return {
      fromJSON: vi.fn(),
      layout: vi.fn(),
      width: 1600,
      height: 600,
      groups: [],
      panels: [],
      activeGroup: { id: "g-center" },
      getPanel: vi.fn(() => null),
      onDidActiveGroupChange: vi.fn(() => ({ dispose: vi.fn() })),
      onDidLayoutChange: vi.fn(() => ({ dispose: vi.fn() })),
    } as unknown as Parameters<typeof tryRestoreLayout>[0];
  }

  it("restores the (stripped) layout when every session in the saved env-layout is a phantom", () => {
    // Saved layout had a single session panel — a phantom from a previously-
    // deleted task. After stripping, no session panels remain. We still
    // restore the (truncated) layout; useAutoSessionTab will add the active
    // session's chat panel afterwards.
    const layout = buildLayout();
    layout.grid.root.data[1].data.views = [PHANTOM_PANEL_ID];
    layout.grid.root.data[1].data.activeView = PHANTOM_PANEL_ID;
    Object.assign(layout.panels, {
      [PHANTOM_PANEL_ID]: { id: PHANTOM_PANEL_ID, contentComponent: "chat" },
    });
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(layout);
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue(null);

    const api = makeFakeApi();
    const restored = tryRestoreLayout(api, "env-new", VALID_COMPONENTS, new Set(["phantom"]));
    expect(restored).toBe(true);
    expect(api.fromJSON).toHaveBeenCalledOnce();
    // The phantom was stripped from the layout passed to fromJSON.
    const restoredLayout = (api.fromJSON as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(Object.keys(restoredLayout.panels)).not.toContain(PHANTOM_PANEL_ID);
  });

  it("restores normally when at least one session panel survives the filter", () => {
    const layout = buildLayout();
    layout.grid.root.data[1].data.views = [ALIVE_PANEL_ID, PHANTOM_PANEL_ID];
    layout.grid.root.data[1].data.activeView = ALIVE_PANEL_ID;
    Object.assign(layout.panels, {
      [ALIVE_PANEL_ID]: { id: ALIVE_PANEL_ID, contentComponent: "chat" },
      [PHANTOM_PANEL_ID]: { id: PHANTOM_PANEL_ID, contentComponent: "chat" },
    });
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(layout);
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue(null);

    const api = makeFakeApi();
    const restored = tryRestoreLayout(api, "env-X", VALID_COMPONENTS, new Set(["phantom"]));
    expect(restored).toBe(true);
    expect(api.fromJSON).toHaveBeenCalledOnce();
  });

  it("restores normally when the saved layout has no session panels at all (legit non-session env layout)", () => {
    // An env may persist a layout with only files/terminal/etc. — no session
    // panels to begin with. The fallback guard must NOT discard this layout.
    const layout = buildLayout(); // center has "chat" only, no session:* ids
    vi.spyOn(localStorage, "getEnvLayout").mockReturnValue(layout);
    vi.spyOn(localStorage, "getEnvMaximizeState").mockReturnValue(null);

    const api = makeFakeApi();
    const restored = tryRestoreLayout(api, "env-X", VALID_COMPONENTS, new Set());
    expect(restored).toBe(true);
    expect(api.fromJSON).toHaveBeenCalledOnce();
  });
});

/**
 * Regression: an env-layout could be restored with a session panel whose id
 * referred to a previously-deleted task's session. The fix strips session
 * panels we KNOW belong to a different env; sessions not yet mapped in the
 * store are preserved (they may still be loading via WS).
 */
describe("collectPhantomSessionIdsForEnv", () => {
  it("returns session ids whose mapping is a different env", () => {
    const state = {
      environmentIdBySessionId: {
        "sess-1": "env-A",
        "sess-2": "env-A",
        "sess-3": "env-B",
      },
    };
    expect(collectPhantomSessionIdsForEnv(state, "env-A")).toEqual(new Set(["sess-3"]));
    expect(collectPhantomSessionIdsForEnv(state, "env-B")).toEqual(new Set(["sess-1", "sess-2"]));
  });

  it("returns every mapped session as phantom when the env has no own sessions yet", () => {
    const state = { environmentIdBySessionId: { "sess-1": "env-A" } };
    expect(collectPhantomSessionIdsForEnv(state, "env-new")).toEqual(new Set(["sess-1"]));
  });

  it("does NOT classify a session as a phantom when its mapping is absent (still loading via WS)", () => {
    // A session id present in a saved layout but not in environmentIdBySessionId
    // could be a not-yet-arrived session for this very env. Keep it; reconcile
    // will clean it up later if it really is stale.
    const state = { environmentIdBySessionId: {} };
    expect(collectPhantomSessionIdsForEnv(state, "env-A")).toEqual(new Set());
  });
});
