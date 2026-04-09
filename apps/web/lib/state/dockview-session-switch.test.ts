import { describe, it, expect, vi, beforeEach } from "vitest";
import { performSessionSwitch, type SessionSwitchParams } from "./dockview-session-switch";

// Mock dependencies
vi.mock("@/lib/local-storage", () => ({
  getSessionLayout: vi.fn(() => null),
}));

vi.mock("./dockview-layout-builders", () => ({
  applyLayoutFixups: vi.fn(() => ({ sidebar: "g1", center: "g2" })),
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
    getPanel: vi.fn(),
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
    vi.restoreAllMocks();
    vi.mocked(getSessionLayout).mockReturnValue(null);
    vi.mocked(savedLayoutMatchesLive).mockReturnValue(false);
    vi.mocked(layoutStructuresMatch).mockReturnValue(false);
  });

  it("calls api.layout on the fast path when structures match", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValue(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
  });

  it("calls api.layout on the fast path when saved layout matches", () => {
    vi.mocked(getSessionLayout).mockReturnValue({ grid: {} } as any);
    vi.mocked(savedLayoutMatchesLive).mockReturnValue(true);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.api.fromJSON).not.toHaveBeenCalled();
  });

  it("calls api.layout on the slow path (buildDefault fallback)", () => {
    vi.mocked(layoutStructuresMatch).mockReturnValue(false);
    const params = makeParams();

    performSessionSwitch(params);

    expect(params.api.layout).toHaveBeenCalledWith(800, 600);
    expect(params.buildDefault).toHaveBeenCalledWith(params.api);
  });
});
