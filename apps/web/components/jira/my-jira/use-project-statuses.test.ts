import { describe, expect, it } from "vitest";
import type { JiraStatus } from "@/lib/types/jira";
import { reconcileStatuses } from "./use-project-statuses";

function status(id: string, name: string): JiraStatus {
  return { id, name, statusCategory: "indeterminate" };
}

const IN_DEV = "In Development";

describe("reconcileStatuses", () => {
  it("returns the same reference when nothing is selected", () => {
    const selected: string[] = [];
    expect(reconcileStatuses(selected, [status("1", "Open")])).toBe(selected);
  });

  it("keeps selected statuses that are still available", () => {
    const selected = [IN_DEV, "Done"];
    const available = [status("1", IN_DEV), status("2", "Done"), status("3", "To Do")];
    expect(reconcileStatuses(selected, available)).toEqual([IN_DEV, "Done"]);
  });

  it("drops selected statuses no longer present in the union", () => {
    const selected = [IN_DEV, "Ready for review"];
    const available = [status("1", IN_DEV)];
    expect(reconcileStatuses(selected, available)).toEqual([IN_DEV]);
  });

  it("drops all when none remain (e.g. project deselected)", () => {
    expect(reconcileStatuses([IN_DEV], [])).toEqual([]);
  });

  it("returns the same reference when every selection is still valid", () => {
    const selected = ["Open"];
    const result = reconcileStatuses(selected, [status("1", "Open")]);
    expect(result).toBe(selected);
  });
});
