import { describe, expect, it } from "vitest";
import { canBuildDockerImage, containerTaskLabel } from "./docker-sections";
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

  // The untracked fallback is copy, not container data, so it is resolved
  // through `t()` at the render site rather than returned from here.
  it("returns null when the container carries no task labels", () => {
    expect(containerTaskLabel(container())).toBeNull();
    expect(containerTaskLabel(container({ "kandev.task_title": "  " }))).toBeNull();
  });
});

describe("canBuildDockerImage", () => {
  // POST /api/v1/docker/build is admin-gated on the backend, so a member must
  // not be shown an enabled control that can only end in a 403.
  it("denies members", () => {
    expect(canBuildDockerImage("enabled", "member")).toBe(false);
  });

  it("allows admins", () => {
    expect(canBuildDockerImage("enabled", "admin")).toBe(true);
  });

  // Authentication-disabled mode represents the synthetic single-user admin,
  // which must behave exactly as before.
  it("allows the single user when authentication is disabled", () => {
    expect(canBuildDockerImage("disabled", undefined)).toBe(true);
    expect(canBuildDockerImage(undefined, undefined)).toBe(true);
  });

  it("denies a cleared session while authentication is enabled", () => {
    expect(canBuildDockerImage("enabled", undefined)).toBe(false);
  });

  it("denies an unrecognized role rather than defaulting open", () => {
    expect(canBuildDockerImage("enabled", "")).toBe(false);
    expect(canBuildDockerImage("enabled", "Admin")).toBe(false);
  });
});
