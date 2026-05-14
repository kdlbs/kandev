import { describe, expect, it } from "vitest";
import { containerTaskLabel } from "./docker-sections";
import type { DockerContainer } from "@/lib/api/domains/settings-api";

const taskID = "task-abc123";

function container(labels?: Record<string, string>): DockerContainer {
  return {
    id: "container-1",
    name: "kandev-agent-test",
    image: "kandev/multi-agent:latest",
    state: "running",
    status: "Up 5 seconds",
    labels,
  };
}

describe("containerTaskLabel", () => {
  it("prefers task title over task id", () => {
    expect(
      containerTaskLabel(
        container({
          "kandev.task_id": taskID,
          "kandev.task_title": "Fix Docker reuse",
        }),
      ),
    ).toBe("Fix Docker reuse");
  });

  it("falls back to task id for older containers", () => {
    expect(containerTaskLabel(container({ "kandev.task_id": taskID }))).toBe(taskID);
  });

  it("falls back to task id when the task title label is empty", () => {
    expect(
      containerTaskLabel(
        container({
          "kandev.task_id": taskID,
          "kandev.task_title": " ",
        }),
      ),
    ).toBe(taskID);
  });
});
