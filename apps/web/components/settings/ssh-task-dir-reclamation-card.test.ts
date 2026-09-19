import { describe, expect, it } from "vitest";
import {
  isSSHReclaimEnabled,
  sshReclaimHostLabel,
  sshTaskDirRoot,
} from "./ssh-task-dir-reclamation-card";
import type { Executor } from "@/lib/types/http";

function executor(config?: Record<string, string>): Executor {
  return {
    id: "exec-1",
    name: "Build box",
    type: "ssh",
    status: "ready",
    is_system: false,
    config,
    created_at: "",
    updated_at: "",
  };
}

describe("isSSHReclaimEnabled", () => {
  it("treats only the exact string true as enabled", () => {
    expect(isSSHReclaimEnabled({ ssh_reclaim_task_dir: "true" })).toBe(true);
    expect(isSSHReclaimEnabled({ ssh_reclaim_task_dir: "  true  " })).toBe(false);
  });

  it("defaults to off for a profile that never stored a value", () => {
    expect(isSSHReclaimEnabled(undefined)).toBe(false);
    expect(isSSHReclaimEnabled({})).toBe(false);
  });

  it("does not accept a lookalike value", () => {
    for (const value of ["false", "", "TRUE", "True", "1", "yes", "enabled"]) {
      expect(isSSHReclaimEnabled({ ssh_reclaim_task_dir: value })).toBe(false);
    }
  });
});

describe("sshTaskDirRoot", () => {
  it("names the profile's own workspace root", () => {
    expect(sshTaskDirRoot({ ssh_workdir_root: "/srv/kandev" })).toBe("/srv/kandev/tasks/");
  });

  it("falls back to the executor default when the profile stores none", () => {
    expect(sshTaskDirRoot(undefined)).toBe("~/.kandev/tasks/");
    expect(sshTaskDirRoot({ ssh_workdir_root: "  " })).toBe("~/.kandev/tasks/");
  });

  it("does not double the separator for a root written with a trailing slash", () => {
    expect(sshTaskDirRoot({ ssh_workdir_root: "/srv/kandev/" })).toBe("/srv/kandev/tasks/");
  });
});

describe("sshReclaimHostLabel", () => {
  it("prefers the resolved host", () => {
    expect(
      sshReclaimHostLabel(executor({ ssh_host: "build.example", ssh_host_alias: "prod" })),
    ).toBe("build.example");
  });

  it("falls back to the alias, then to the executor name", () => {
    expect(sshReclaimHostLabel(executor({ ssh_host_alias: "prod" }))).toBe("prod");
    expect(sshReclaimHostLabel(executor({}))).toBe("Build box");
  });
});
