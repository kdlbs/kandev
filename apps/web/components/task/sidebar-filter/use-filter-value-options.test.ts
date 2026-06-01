import { describe, it, expect } from "vitest";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Repository } from "@/lib/types/http";
import { repositorySlug } from "@/lib/repository-slug";
import { repositoryOptions, workflowStepOptions } from "./use-filter-value-options";

function repo(overrides: Partial<Repository>): Repository {
  return {
    id: "r1",
    workspace_id: "w1",
    name: "",
    source_type: "local",
    local_path: "",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    ...overrides,
  } as Repository;
}

function snapshot(
  id: string,
  name: string,
  steps: Array<{ id: string; title: string; position: number; color?: string }>,
): WorkflowSnapshotData {
  return {
    workflowId: id,
    workflowName: name,
    steps: steps.map((s) => ({
      id: s.id,
      title: s.title,
      color: s.color ?? "bg-neutral-400",
      position: s.position,
    })),
    tasks: [],
  };
}

describe("repositoryOptions (#1213)", () => {
  // The option value becomes the saved filter clause value. It MUST equal what
  // the task board sets as each task's repositoryPath (repositorySlug) — for a
  // local repo that is the repo name, NOT the full local_path. Using local_path
  // here was the bug: the clause matched no task and the board went empty.
  it("uses repositorySlug as the option value (local repo → name, not local_path)", () => {
    const local = repo({ name: "kandev", local_path: "/home/carlos/Projects/kandev" });
    const provider = repo({ provider_owner: "kdlbs", provider_name: "kandev-web" });
    const options = repositoryOptions({ w1: [local, provider] });
    expect(options).toEqual([
      { value: repositorySlug(local), label: repositorySlug(local) },
      { value: repositorySlug(provider), label: repositorySlug(provider) },
    ]);
    expect(options[0].value).toBe("kandev");
    expect(options[0].value).not.toBe(local.local_path);
    expect(options[1].value).toBe("kdlbs/kandev-web");
  });
});

describe("workflowStepOptions", () => {
  it("tags each step with its workflow name as the group", () => {
    const snapshots = {
      wf1: snapshot("wf1", "Alpha", [
        { id: "s1", title: "Backlog", position: 0 },
        { id: "s2", title: "Review", position: 1 },
      ]),
      wf2: snapshot("wf2", "Beta", [{ id: "s3", title: "Done", position: 0 }]),
    };

    const options = workflowStepOptions(snapshots);

    expect(options.map((o) => [o.label, o.group])).toEqual([
      ["Backlog", "Alpha"],
      ["Review", "Alpha"],
      ["Done", "Beta"],
    ]);
  });

  it("orders workflows alphabetically and steps by position within each workflow", () => {
    const snapshots = {
      b: snapshot("b", "Beta", [
        { id: "b2", title: "Second", position: 2 },
        { id: "b1", title: "First", position: 1 },
      ]),
      a: snapshot("a", "Alpha", [{ id: "a1", title: "Only", position: 0 }]),
    };

    const options = workflowStepOptions(snapshots);

    expect(options.map((o) => o.group)).toEqual(["Alpha", "Beta", "Beta"]);
    expect(options.map((o) => o.label)).toEqual(["Only", "First", "Second"]);
  });

  it("falls back to workflow id when name is empty", () => {
    const snapshots = {
      wfX: snapshot("wfX", "", [{ id: "s1", title: "Step", position: 0 }]),
    };

    expect(workflowStepOptions(snapshots)[0]?.group).toBe("wfX");
  });

  it("dedupes steps that appear under multiple workflows, keeping the first occurrence", () => {
    const snapshots = {
      a: snapshot("a", "Alpha", [{ id: "shared", title: "Shared", position: 0 }]),
      b: snapshot("b", "Beta", [{ id: "shared", title: "Shared", position: 0 }]),
    };

    const options = workflowStepOptions(snapshots);

    expect(options).toHaveLength(1);
    expect(options[0]?.group).toBe("Alpha");
  });
});
