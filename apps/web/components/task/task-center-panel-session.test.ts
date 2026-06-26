import { describe, expect, it } from "vitest";
import { resolveCenterPanelSessionId } from "./task-center-panel-session";

const ACTIVE_SESSION_ID = "active-session";
const PANEL_SESSION_ID = "panel-session";

describe("resolveCenterPanelSessionId", () => {
  it("uses the active session when no explicit panel session is provided", () => {
    expect(resolveCenterPanelSessionId(null, ACTIVE_SESSION_ID)).toBe(ACTIVE_SESSION_ID);
    expect(resolveCenterPanelSessionId(undefined, ACTIVE_SESSION_ID)).toBe(ACTIVE_SESSION_ID);
  });

  it("preserves an explicit panel session", () => {
    expect(resolveCenterPanelSessionId(PANEL_SESSION_ID, ACTIVE_SESSION_ID)).toBe(PANEL_SESSION_ID);
  });

  it("returns null when neither source is available", () => {
    expect(resolveCenterPanelSessionId(null, null)).toBeNull();
  });
});
