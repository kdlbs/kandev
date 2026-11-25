import { describe, expect, it } from "vitest";
import { selectSessionRecoveryError } from "./session-recovery-presentation";

const bootstrapError = {
  session_id: "session-1",
  stamp: "bootstrap-1",
  occurred_at: "2026-09-11T10:00:00Z",
  preview: "The agent could not start.",
  phase: "bootstrap",
  category: "generic_launch_failure",
} as const;

describe("process exit recovery presentation", () => {
  it("keeps a persisted process exit cause across reload and a newer distinct failure", () => {
    const metadata = {
      last_agent_error: {
        message: "The agent exited before startup completed.",
        occurred_at: "2026-09-11T10:00:00Z",
        stamp: "process-failure",
        phase: "bootstrap",
        causes: [
          {
            operation: "resume",
            code: "process_exited",
            reason: "mise_config_untrusted",
            exit_code: 1,
          },
        ],
      },
    };
    expect(selectSessionRecoveryError(null, "session-1", metadata)).toMatchObject({
      stamp: "process-failure",
      causes: [
        {
          operation: "resume",
          code: "process_exited",
          reason: "mise_config_untrusted",
          exit_code: 1,
        },
      ],
    });

    const newer = {
      ...bootstrapError,
      stamp: "generic-failure",
      occurred_at: "2026-09-11T10:05:00Z",
      preview: "The bootstrap operation could not be completed.",
    };
    expect(selectSessionRecoveryError(newer, "session-1", metadata)).toMatchObject({
      stamp: "generic-failure",
      preview: "The bootstrap operation could not be completed.",
    });
    expect(selectSessionRecoveryError(newer, "session-1", metadata)?.causes).toBeUndefined();
  });
});
