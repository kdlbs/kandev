import { describe, expect, it, vi } from "vitest";
import { resolveLayoutApplySessionIds } from "./layout-preset-selector-session-ids";

const ACTIVE_SESSION_ID = "active-session";
const SIBLING_SESSION_ID = "sibling-session";

describe("resolveLayoutApplySessionIds", () => {
  it("loads task sessions before reading ids when the cache is not loaded", async () => {
    let sessions = [{ id: ACTIVE_SESSION_ID }];
    const loadSessions = vi.fn(async () => {
      sessions = [{ id: ACTIVE_SESSION_ID }, { id: SIBLING_SESSION_ID }];
    });

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      activeSessionId: ACTIVE_SESSION_ID,
      sessionsLoaded: false,
      loadSessions,
      getSessionsForTask: () => sessions,
    });

    expect(loadSessions).toHaveBeenCalledWith(true);
    expect(result).toEqual([ACTIVE_SESSION_ID, SIBLING_SESSION_ID]);
  });

  it("keeps the active session when it is not in the loaded task-session list", async () => {
    const loadSessions = vi.fn();

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      activeSessionId: ACTIVE_SESSION_ID,
      sessionsLoaded: true,
      loadSessions,
      getSessionsForTask: () => [{ id: SIBLING_SESSION_ID }],
    });

    expect(loadSessions).not.toHaveBeenCalled();
    expect(result).toEqual([ACTIVE_SESSION_ID, SIBLING_SESSION_ID]);
  });

  it("does not duplicate the active session when it is already loaded", async () => {
    const loadSessions = vi.fn();

    const result = await resolveLayoutApplySessionIds({
      activeTaskId: "task-1",
      activeSessionId: ACTIVE_SESSION_ID,
      sessionsLoaded: true,
      loadSessions,
      getSessionsForTask: () => [{ id: ACTIVE_SESSION_ID }, { id: SIBLING_SESSION_ID }],
    });

    expect(loadSessions).not.toHaveBeenCalled();
    expect(result).toEqual([ACTIVE_SESSION_ID, SIBLING_SESSION_ID]);
  });
});
