import { describe, expect, it } from "vitest";
import {
  TASK_TITLE_MAX_LENGTH,
  clampTaskTitleInput,
  truncateRemoteTaskTitle,
} from "@/lib/task-title";

describe("task title limits", () => {
  it("uses a 60-character maximum", () => {
    expect(TASK_TITLE_MAX_LENGTH).toBe(60);
  });

  it("clamps manual input to the maximum without adding decoration", () => {
    const value = clampTaskTitleInput("x".repeat(80));

    expect(value).toHaveLength(TASK_TITLE_MAX_LENGTH);
    expect(value).toBe("x".repeat(TASK_TITLE_MAX_LENGTH));
  });

  it("counts Unicode code points as characters", () => {
    expect(clampTaskTitleInput("🙂".repeat(80))).toBe("🙂".repeat(TASK_TITLE_MAX_LENGTH));
  });

  it("truncates remote prefills with an ellipsis", () => {
    const value = truncateRemoteTaskTitle("x".repeat(80));

    expect(value).toHaveLength(TASK_TITLE_MAX_LENGTH);
    expect(value).toBe(`${"x".repeat(TASK_TITLE_MAX_LENGTH - 1)}…`);
  });

  it("leaves titles at or below the limit unchanged", () => {
    expect(truncateRemoteTaskTitle("x".repeat(TASK_TITLE_MAX_LENGTH))).toBe(
      "x".repeat(TASK_TITLE_MAX_LENGTH),
    );
    expect(truncateRemoteTaskTitle("short title")).toBe("short title");
  });
});
