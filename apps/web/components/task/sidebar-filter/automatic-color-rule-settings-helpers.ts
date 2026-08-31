import type { SidebarTaskColorRule } from "@/lib/task-color-automation-settings";

export function createAutomaticColorRule(number: number): SidebarTaskColorRule {
  return {
    id: `automatic-color-${Date.now().toString(36)}-${number}`,
    enabled: false,
    condition: { dimension: "task_state", value: null, label: "" },
    output: { kind: "fixed", color: "gray" },
  };
}
