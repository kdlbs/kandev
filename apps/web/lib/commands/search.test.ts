import { describe, expect, it } from "vitest";
import type { CommandItem } from "./types";
import {
  findFirstMatchingCommand,
  getCommandSearchTerms,
  scoreCommandSearch,
  sortCommandsForSearch,
} from "./search";

function command(
  id: string,
  label: string,
  keywords: string[] = [],
  priority?: number,
): CommandItem {
  return {
    id,
    label,
    group: "Test",
    keywords,
    priority,
    action: () => undefined,
  };
}

function createPRCommand(extraKeywords: string[] = []): CommandItem {
  return command("git-create-pr", "Create PR", ["pull request", "pr", ...extraKeywords]);
}

describe("command search", () => {
  it("searches labels and aliases without matching internal IDs", () => {
    const theme = command("pref-theme", "Switch to Dark Mode", ["theme", "appearance"]);

    expect(getCommandSearchTerms(theme)).toEqual(["Switch to Dark Mode", "theme", "appearance"]);
    expect(scoreCommandSearch("pr", "pr", [])).toBe(0);
    expect(scoreCommandSearch(theme.id, "pr", getCommandSearchTerms(theme))).toBeLessThan(1);
  });

  it("ranks an exact alias above partial fuzzy matches", () => {
    const push = command("git-push", "Push", ["push to remote"]);
    const createPR = createPRCommand();

    expect(sortCommandsForSearch([push, createPR], "pr")).toEqual([createPR, push]);
    expect(findFirstMatchingCommand([push, createPR], "pr")).toBe(createPR);
  });

  it("ignores surrounding whitespace and casing in aliases", () => {
    const createPR = createPRCommand();

    expect(findFirstMatchingCommand([createPR], "  PULL REQUEST  ")).toBe(createPR);
  });

  it("matches compound queries across separate search terms", () => {
    const push = command("git-push", "Push", ["push", "git", "push to remote"]);
    const createPR = createPRCommand(["git"]);
    const rebase = command("git-rebase", "Rebase", ["rebase", "git", "branch"]);
    const allTasks = command("nav-tasks", "Go to All Tasks", ["tasks", "list", "all"]);
    const theme = command("pref-theme", "Switch to Dark Mode", ["theme", "appearance", "dark"]);
    const commands = [push, createPR, rebase, allTasks, theme];

    expect(findFirstMatchingCommand(commands, "git push")).toBe(push);
    expect(findFirstMatchingCommand(commands, "git pull request")).toBe(createPR);
    expect(findFirstMatchingCommand(commands, "git rebase")).toBe(rebase);
    expect(findFirstMatchingCommand(commands, "task list")).toBe(allTasks);
    expect(findFirstMatchingCommand(commands, "dark theme")).toBe(theme);
  });

  it("does not expand short aliases into longer unrelated query tokens", () => {
    const createPR = createPRCommand(["git"]);

    expect(findFirstMatchingCommand([createPR], "git preview")).toBeUndefined();
  });

  it("uses command priority to break equal-score ties", () => {
    const later = command("later", "Shared term", [], 10);
    const earlier = command("earlier", "Shared term", [], 0);

    expect(sortCommandsForSearch([later, earlier], "shared")).toEqual([earlier, later]);
  });

  it("returns no command when nothing matches", () => {
    expect(findFirstMatchingCommand([command("home", "Go Home")], "terminal")).toBeUndefined();
  });
});
