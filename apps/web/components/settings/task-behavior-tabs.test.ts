import { describe, expect, it } from "vitest";
import {
  taskBehaviorTab,
  firstNewAttentionTab,
  TASK_BEHAVIOR_TARGET_TABS,
} from "./task-behavior-tabs";

describe("Task behavior tab ownership", () => {
  it("maps every existing discovery target to its owning tab", () => {
    expect(taskBehaviorTab("general-creation-auto-focus")).toBe("tasks");
    expect(taskBehaviorTab("general-transcript-navigation")).toBe("conversation");
    expect(taskBehaviorTab("general-todo-list-panel")).toBe("conversation");
    expect(taskBehaviorTab("system-message-queue")).toBe("runtime");
    expect(taskBehaviorTab("general-task-sleep-inhibition")).toBe("runtime");
    expect(taskBehaviorTab("unrelated")).toBeUndefined();
  });
  it("reveals new attention once in tab order without trapping navigation", () => {
    expect(firstNewAttentionTab(["runtime", "tasks"], [])).toBe("tasks");
    expect(firstNewAttentionTab(["runtime"], ["runtime"])).toBeUndefined();
    expect(firstNewAttentionTab(["runtime", "conversation"], ["runtime"])).toBe("conversation");
    expect(firstNewAttentionTab([], ["tasks"])).toBeUndefined();
  });
});

it("preserves all nine existing discovery fragments", () => {
  expect(TASK_BEHAVIOR_TARGET_TABS).toEqual({
    "setting-creation-auto-focus": "tasks",
    "setting-agent-generated-task-titles": "tasks",
    "setting-agent-task-profile": "tasks",
    "setting-prevent-auto-start-on-open": "tasks",
    "setting-archive-confirmation": "tasks",
    "setting-unread-messages": "conversation",
    "setting-transcript-navigation": "conversation",
    "setting-session-capacity": "runtime",
    "setting-message-queue": "runtime",
  });
});
