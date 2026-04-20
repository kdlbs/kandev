import { describe, it, expect, vi, beforeEach } from "vitest";
import { performSessionSwitch, type SessionSwitchParams } from "./dockview-session-switch";

// Mock dependencies
vi.mock("@/lib/local-storage", () => ({
  getSessionLayout: vi.fn(() => null),
}));

vi.mock("./dockview-layout-builders", () => ({
  applyLayoutFixups: vi.fn(() => ({
    sidebarGroupId: "g1",
    centerGroupId: "g2",
    rightTopGroupId: "g3",
    rightBottomGroupId: "g4",
  })),
}));

vi.mock("./layout-manager", () => ({
  fromDockviewApi: vi.fn(() => ({ columns: [] })),
  savedLayoutMatchesLive: vi.fn(() => false),
  layoutStructuresMatch: vi.fn(() => false),
}));

import { getSessionLayout } from "@/lib/local-storage";
import { layoutStructuresMatch, savedLayoutMatchesLive } from "./layout-manager";

function makeMockApi() {
  return {
    panels: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
  } as unknown as SessionSwitchParams["api"];
}

function makeParams(overrides?: Partial<SessionSwitchParams>): SessionSwitchParams {
  return {
    api: makeMockApi(),
    oldSessionId: "old-session",
    newSessionId: "new-session",
    safeWidth: 800,
    safeHeight: 600,
    buildDefault: vi.fn(),
    getDefaultLayout: vi.fn(() => ({ columns: [] })),
    ...overrides,
  };
}

describe("performSessionSwitch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls api.layout on the fast path when structures match", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
  });

  it("calls api.layout on the fast path when saved layout matches", () => {
    vi.mocked(getSessionLayout).mockReturnValueOnce({ grid: {} } as unknown as ReturnType<
      typeof getSessionLayout
    >);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.api.fromJSON).not.toHaveBeenCalled();
  });

  it("creates session panel inline on the fast path when it does not exist", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "session:new-session",
        component: "chat",
        params: { sessionId: "new-session" },
      }),
    );
  });

  it("skips addPanel on the fast path when the session panel already exists", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValueOnce(true);
    const panel = { id: "session:new-session", api: { component: "chat" }, group: { id: "g1" } };
    const params = makeParams({
      api: {
        ...makeMockApi(),
        getPanel: vi.fn((id: string) => (id === "session:new-session" ? panel : null)),
      } as unknown as SessionSwitchParams["api"],
    });

    performSessionSwitch(params);

    expect(params.api.addPanel).not.toHaveBeenCalled();
  });

  it("skips fast path when saved layout has ephemeral panels (file-editor)", () => {
    const savedLayout = {
      grid: {},
      panels: { "preview:file-editor": { contentComponent: "file-editor" } },
    } as unknown as ReturnType<typeof getSessionLayout>;
    vi.mocked(getSessionLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
    const params = makeParams();

    performSessionSwitch(params);

    // Should use fromJSON (slow path) instead of fast path
    expect(params.api.fromJSON).toHaveBeenCalled();
  });

  it("skips fast path when saved layout has ephemeral panels (diff-viewer)", () => {
    const savedLayout = {
      grid: {},
      panels: { "preview:file-diff": { contentComponent: "diff-viewer" } },
    } as unknown as ReturnType<typeof getSessionLayout>;
    vi.mocked(getSessionLayout).mockReturnValueOnce(savedLayout).mockReturnValueOnce(savedLayout);
    vi.mocked(savedLayoutMatchesLive).mockReturnValueOnce(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.fromJSON).toHaveBeenCalled();
  });

  it("calls api.layout on the slow path (buildDefault fallback)", () => {
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.buildDefault).toHaveBeenCalledWith(params.api);
  });
});
