import { afterEach, describe, expect, it } from "vitest";
import {
  getEnvHiddenSessionRecords,
  getEnvHiddenSessions,
  setEnvHiddenSessionOwner,
  setEnvHiddenSessions,
} from "./env-hidden-sessions";

const ENV_ID = "env-hidden-sessions-test";
const STORAGE_KEY = `kandev.dockview.env-hidden-sessions-v1.${ENV_ID}`;

afterEach(() => window.sessionStorage.removeItem(STORAGE_KEY));

describe("env hidden sessions", () => {
  it("returns no sessions for an environment without persisted state", () => {
    expect(getEnvHiddenSessions(ENV_ID)).toEqual([]);
    expect(getEnvHiddenSessionRecords(ENV_ID)).toEqual([]);
  });

  it("round-trips records and treats legacy session-only values as ownerless", () => {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(["legacy", "session@task@owner"]));

    expect(getEnvHiddenSessionRecords(ENV_ID)).toEqual([
      { sessionId: "legacy", taskId: "" },
      { sessionId: "session@task", taskId: "owner" },
    ]);
  });

  it("upserts the owner for a hidden session", () => {
    setEnvHiddenSessionOwner(ENV_ID, "session-a", "task-a");
    setEnvHiddenSessionOwner(ENV_ID, "session-a", "task-b");

    expect(getEnvHiddenSessionRecords(ENV_ID)).toEqual([
      { sessionId: "session-a", taskId: "task-b" },
    ]);
  });

  it("retains owners for kept sessions and drops owners for removed sessions", () => {
    setEnvHiddenSessionOwner(ENV_ID, "session-a", "task-a");
    setEnvHiddenSessionOwner(ENV_ID, "session-b", "task-b");

    setEnvHiddenSessions(ENV_ID, ["session-b", "session-c"]);

    expect(getEnvHiddenSessionRecords(ENV_ID)).toEqual([
      { sessionId: "session-b", taskId: "task-b" },
      { sessionId: "session-c", taskId: "" },
    ]);
  });
});
