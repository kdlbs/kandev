import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import { t as translate } from "@/lib/i18n";
import type { CommandItem } from "@/lib/commands/types";

import {
  buildGitCommands,
  buildPanelCommands,
  buildSessionCommands,
  buildTaskCommands,
  buildWorkspaceCommands,
} from "./session-commands";

const t = translate as unknown as TFunction;
const ARCHIVE_COMMAND_ID = "task-archive";

/**
 * A `t` that echoes the key it was asked for. Any label or group that did NOT
 * go through the catalog comes back as plain English instead, which is what the
 * "every … resolves through the catalog" tests below detect. A palette entry
 * whose copy is inlined renders correct English and passes lint, so an assertion
 * on the English itself cannot tell the two apart.
 */
const markerT = ((key: string) => `«${key}»`) as unknown as TFunction;

const unresolved = (commands: CommandItem[]) =>
  commands.flatMap((cmd) =>
    [
      ["label", cmd.label],
      ["group", cmd.group],
      ["inputPlaceholder", cmd.inputPlaceholder],
    ]
      .filter(([, value]) => typeof value === "string" && !value.startsWith("«"))
      .map(([field, value]) => `${cmd.id}.${field} = ${value}`),
  );

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

  it("registers the archive confirmation dismiss callback", () => {
    const onDismiss = vi.fn();
    const archive = build({
      onArchiveConfirmationDismiss: onDismiss,
    } as Partial<Parameters<typeof buildTaskCommands>[0]>).find(
      (cmd) => cmd.id === ARCHIVE_COMMAND_ID,
    ) as CommandItem & { onConfirmationDismiss?: () => void };

    archive.onConfirmationDismiss?.();

    expect(onDismiss).toHaveBeenCalledOnce();
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

describe("session palette copy", () => {
  const gitOptions = {
    git: { push: vi.fn(), pull: vi.fn(), rebase: vi.fn(), merge: vi.fn() },
    baseBranch: "origin/main",
    openCommitDialog: vi.fn(),
    openPRDialog: vi.fn(),
    runGitWithFeedback: vi.fn(),
  };

  it("resolves every session, git, workspace and panel entry through the catalog", () => {
    const commands = [
      ...buildSessionCommands(true, vi.fn(), markerT),
      ...buildGitCommands({
        ...gitOptions,
        git: gitOptions.git as unknown as Parameters<typeof buildGitCommands>[0]["git"],
        t: markerT,
      }),
      ...buildWorkspaceCommands("session-A", markerT),
      ...buildPanelCommands(
        {
          addBrowser: vi.fn(),
          addTerminal: vi.fn(),
          addPlan: vi.fn(),
          addChanges: vi.fn(),
        } as unknown as Parameters<typeof buildPanelCommands>[0],
        false,
        markerT,
      ),
      ...buildTaskCommands({
        activeTaskId: "task-A",
        isTaskArchived: false,
        t: markerT,
        openNewAgent: vi.fn(),
        openSubtask: vi.fn(),
        requestArchive: vi.fn(),
      }),
    ];

    expect(unresolved(commands)).toEqual([]);
  });

  it("labels the git operation toast with the same catalog entry as the palette row", () => {
    const runGitWithFeedback = vi.fn();
    const push = buildGitCommands({
      ...gitOptions,
      git: gitOptions.git as unknown as Parameters<typeof buildGitCommands>[0]["git"],
      runGitWithFeedback,
      t: markerT,
    }).find((cmd) => cmd.id === "git-push");

    push?.action?.();

    // The second argument is the operation name `useGitWithFeedback` composes
    // its toast titles around; it used to be a hardcoded "Push".
    expect(runGitWithFeedback).toHaveBeenCalledWith(expect.any(Function), push?.label);
  });

  it("keeps every group of a builder identical so the palette does not split it", () => {
    const groups = new Set(
      buildGitCommands({
        ...gitOptions,
        git: gitOptions.git as unknown as Parameters<typeof buildGitCommands>[0]["git"],
        t: markerT,
      }).map((cmd) => cmd.group),
    );

    expect(groups.size).toBe(1);
  });
});
