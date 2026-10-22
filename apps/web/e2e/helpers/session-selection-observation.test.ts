import { EventEmitter } from "node:events";
import type { Page } from "@playwright/test";
import { describe, expect, it } from "vitest";
import {
  isBackgroundUpdateNotice,
  observeSessionSelection,
  unexpectedSelectionRequests,
} from "./session-selection-observation";

const focus = {
  type: "request",
  action: "session.launch",
  payload: {
    task_id: "task-1",
    session_id: "live-1",
    intent: "resume",
    activation_source: "session_focus",
  },
};

function captured(messages: unknown[]) {
  const page = new EventEmitter();
  const socket = Object.assign(new EventEmitter(), { url: () => "ws://localhost:8080/ws" });
  const requests = observeSessionSelection(page as unknown as Page);
  page.emit("websocket", socket);
  for (const message of messages) {
    socket.emit("framesent", { payload: Buffer.from(JSON.stringify(message)) });
  }
  return requests;
}

describe("native session selection observation", () => {
  it("permits only the full automatic focus payload for the known live session", () => {
    const requests = captured([focus]);
    expect(requests).toEqual([focus]);
    expect(unexpectedSelectionRequests(requests, "task-1", "live-1")).toEqual([]);
  });

  it.each([
    { activation_source: undefined },
    { activation_source: "session_open" },
    { intent: "restart" },
    { intent: undefined },
    { task_id: "another-task" },
    { session_id: "seeded-no-runtime" },
    { prompt: "unexpected prompt" },
    { attachments: [] },
  ])("rejects unclassified launch payload %j", (changes) => {
    const requests = captured([{ ...focus, payload: { ...focus.payload, ...changes } }]);
    expect(unexpectedSelectionRequests(requests, "task-1", "live-1")).toEqual(requests);
    expect(requests).toHaveLength(1);
  });

  it.each(["stop", "resume", "restart", "delete", "set_primary"])(
    "rejects explicit session.%s",
    (action) => {
      const requests = captured([{ ...focus, action: `session.${action}` }]);
      expect(unexpectedSelectionRequests(requests, "task-1", "live-1")).toEqual(requests);
      expect(requests).toHaveLength(1);
    },
  );

  it("retains both requests from the separately diagnosed menu restart", () => {
    const requests = captured([
      { ...focus, action: "session.stop" },
      { ...focus, payload: { task_id: "task-1", session_id: "live-1" } },
    ]);
    expect(unexpectedSelectionRequests(requests, "task-1", "live-1")).toEqual(requests);
    expect(requests).toHaveLength(2);
  });
});

const update = {
  title: "Kandev update available",
  description: "Kandev v1.2.3 is available. Open Settings > System > Updates to review it.",
  icons: ["tabler-icon tabler-icon-info-circle h-5 w-5"],
};

it("recognizes the source-supported informational background update", () => {
  expect(isBackgroundUpdateNotice(update)).toBe(true);
});

it.each([
  { title: "Session restart failed" },
  { description: "Failed to launch session: session is not resumable" },
  { icons: ["tabler-icon tabler-icon-x h-5 w-5"] },
  { icons: ["tabler-icon tabler-icon-check h-5 w-5"] },
  { icons: [] },
])("rejects error or unclassified feedback %j", (changes) => {
  expect(isBackgroundUpdateNotice({ ...update, ...changes })).toBe(false);
});
