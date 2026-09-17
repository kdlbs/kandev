import type { TaskSession } from "@/lib/types/http";
import { describe, expect, it } from "vitest";
import { resolveModelSelectorAgentName } from "./model-selector-provider";

describe("composer provider identity", () => {
  const profiles = [{ id: "profile-1", agent_name: "codex" }];

  it("uses session snapshot identity ahead of an edited profile", () => {
    expect(
      resolveModelSelectorAgentName(
        {
          agent_profile_id: "profile-1" as TaskSession["agent_profile_id"],
          agent_profile_snapshot: { agent_name: "claude" },
        },
        profiles,
      ),
    ).toBe("claude");
  });

  it.each([undefined, null, "", "   ", 42])(
    "falls back to the matching profile for snapshot %s",
    (agent_name) => {
      expect(
        resolveModelSelectorAgentName(
          {
            agent_profile_id: "profile-1" as TaskSession["agent_profile_id"],
            agent_profile_snapshot: { agent_name },
          },
          profiles,
        ),
      ).toBe("codex");
    },
  );

  it("does not infer a CLI from the model or agent UUID", () => {
    expect(
      resolveModelSelectorAgentName(
        {
          agent_profile_id: "deleted" as TaskSession["agent_profile_id"],
          agent_profile_snapshot: { model: "gpt-5", agent_id: "agent-uuid" },
        },
        profiles,
      ),
    ).toBeNull();
    expect(resolveModelSelectorAgentName(null, profiles)).toBeNull();
  });

  it("resolves each session independently and ignores model changes", () => {
    for (const model of ["gpt-5", "sonnet"]) {
      expect(
        resolveModelSelectorAgentName(
          {
            agent_profile_snapshot: { agent_name: "opencode", model },
          },
          profiles,
        ),
      ).toBe("opencode");
    }
    expect(
      resolveModelSelectorAgentName(
        {
          agent_profile_snapshot: { agent_name: "claude" },
        },
        profiles,
      ),
    ).toBe("claude");
  });
});
