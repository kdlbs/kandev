import { describe, expect, it } from "vitest";
import { readMcpAttachmentHistory } from "./mcp-attachment-reconciliation";

const at = "2026-10-25T13:00:00Z";
function report() {
  const manifest = {
    version: 1,
    catalog_version: 1,
    task_id: "task",
    workspace_id: "workspace",
    surface: "kanban-task",
    workspace_revision: 0,
    task_revision: 0,
    packages: ["core", "walkthroughs"],
    origins: { core: "required", walkthroughs: "workspace" },
  };
  return {
    version: 1,
    current: {
      attachment_attempt_id: "attempt",
      task_id: "task",
      session_id: "session",
      execution_id: "execution",
      started_at: at,
      builtin_packages: manifest,
      servers: [
        {
          name: "kandev-walkthroughs",
          source: "kandev",
          builtin_package: "walkthroughs",
          status: "delivered",
          accepted_at: at,
        },
      ],
    },
    builtin_binding: {
      manifest,
      session_id: "session",
      execution_id: "execution",
      attachment_attempt_id: "attempt",
      accepted_at: at,
    },
  };
}

describe("built-in package attachment projection", () => {
  it("keeps acceptance separate from connection and tools evidence", () => {
    const value = report();
    expect(readMcpAttachmentHistory(value)).toEqual(value);
    expect(readMcpAttachmentHistory(value)?.current.servers?.[0]).not.toHaveProperty("tool_count");
  });
  it.each([
    [
      "binding version",
      (value: ReturnType<typeof report>) => {
        value.builtin_binding.manifest.version = 2;
      },
    ],
    [
      "foreign task",
      (value: ReturnType<typeof report>) => {
        value.builtin_binding.manifest.task_id = "other";
      },
    ],
    [
      "foreign session",
      (value: ReturnType<typeof report>) => {
        value.builtin_binding.session_id = "other";
      },
    ],
    [
      "unknown package",
      (value: ReturnType<typeof report>) => {
        value.builtin_binding.manifest.packages.push("unknown");
      },
    ],
    [
      "missing acceptance",
      (value: ReturnType<typeof report>) => {
        value.builtin_binding.accepted_at = "";
      },
    ],
    [
      "false provenance",
      (value: ReturnType<typeof report>) => {
        value.current.servers[0].source = "profile";
      },
    ],
    [
      "lookalike identity",
      (value: ReturnType<typeof report>) => {
        value.current.servers[0].name = "kandev-walkthroughs-sse";
      },
    ],
  ])("rejects %s", (_name, corrupt) => {
    const value = report();
    corrupt(value);
    expect(readMcpAttachmentHistory(value)).toBeNull();
  });
  it("retains historical acceptance when a later execution has not accepted yet", () => {
    const value = report();
    value.current.execution_id = "replacement";
    expect(readMcpAttachmentHistory(value)).toEqual(value);
  });
});
