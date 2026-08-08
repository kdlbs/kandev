import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import { t as translate } from "@/lib/i18n";

import { buildTaskCommands } from "./session-commands";

const t = translate as unknown as TFunction;
const ARCHIVE_COMMAND_ID = "task-archive";

function build(overrides: Partial<Parameters<typeof buildTaskCommands>[0]> = {}) {
  return buildTaskCommands({
    activeTaskId: "task-A",
    isTaskArchived: false,
    t,
    openNewAgent: vi.fn(),
    openSubtask: vi.fn(),
    requestArchive: vi.fn(),
    ...overrides,
  });
}

describe("buildTaskCommands", () => {
  it("offers an archive command that matches typing 'archive'", () => {
    const archive = build().find((cmd) => cmd.id === ARCHIVE_COMMAND_ID);

    expect(archive).toBeDefined();
    expect(archive?.label).toBe("Archive task");
    expect(archive?.group).toBe("Tasks");
    expect(archive?.keywords).toContain("archive");
  });

  it("groups the archive command with the other task commands", () => {
    const commands = build();
    const subtask = commands.find((cmd) => cmd.id === "subtask-create");
    const archive = commands.find((cmd) => cmd.id === ARCHIVE_COMMAND_ID);

    expect(archive?.group).toBe(subtask?.group);
  });

  it("runs the archive request rather than archiving directly", () => {
    const requestArchive = vi.fn();

    build({ requestArchive })
      .find((cmd) => cmd.id === ARCHIVE_COMMAND_ID)
      ?.action?.();

    expect(requestArchive).toHaveBeenCalledTimes(1);
  });

  it("hides the archive command for an archived task", () => {
    const commands = build({ isTaskArchived: true });

    expect(commands.find((cmd) => cmd.id === ARCHIVE_COMMAND_ID)).toBeUndefined();
    expect(commands.find((cmd) => cmd.id === "subtask-create")).toBeDefined();
  });

  it("registers no task commands without an active task", () => {
    expect(build({ activeTaskId: null })).toEqual([]);
  });
});
