import { describe, expect, it } from "vitest";
import { sessionUsesSubscriptionUsage } from "./subscription-usage";

const antigravityProfile = { id: "profile-antigravity", agent_name: "antigravity-acp" };
const claudeProfile = { id: "profile-claude", agent_name: "claude-acp" };

describe("sessionUsesSubscriptionUsage", () => {
  it("matches the Antigravity profile identity", () => {
    expect(
      sessionUsesSubscriptionUsage({ agent_profile_id: "profile-antigravity" }, [
        antigravityProfile,
        claudeProfile,
      ]),
    ).toBe(true);
  });

  it("falls back to the session profile snapshot when profiles are not hydrated", () => {
    expect(
      sessionUsesSubscriptionUsage(
        {
          agent_profile_id: "profile-antigravity",
          agent_profile_snapshot: { agent_name: "antigravity-acp" },
        },
        [],
      ),
    ).toBe(true);
  });

  it("does not disclose for other agents or absent sessions", () => {
    expect(
      sessionUsesSubscriptionUsage({ agent_profile_id: "profile-claude" }, [
        antigravityProfile,
        claudeProfile,
      ]),
    ).toBe(false);
    expect(sessionUsesSubscriptionUsage(null, [antigravityProfile])).toBe(false);
    expect(sessionUsesSubscriptionUsage({}, [antigravityProfile])).toBe(false);
  });
});
