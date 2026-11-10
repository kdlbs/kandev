import { describe, expect, it } from "vitest";
import { sessionUsesSubscriptionUsage } from "./subscription-usage";

const antigravityAgentID = "antigravity-acp";
const antigravityProfileID = "profile-antigravity";
const antigravityProfile = { id: antigravityProfileID, agent_name: antigravityAgentID };
const claudeProfile = { id: "profile-claude", agent_name: "claude-acp" };

describe("sessionUsesSubscriptionUsage", () => {
  it("matches the Antigravity profile identity", () => {
    expect(
      sessionUsesSubscriptionUsage({ agent_profile_id: antigravityProfileID }, [
        antigravityProfile,
        claudeProfile,
      ]),
    ).toBe(true);
  });

  it("falls back to the session profile snapshot when profiles are not hydrated", () => {
    expect(
      sessionUsesSubscriptionUsage(
        {
          agent_profile_id: antigravityProfileID,
          agent_profile_snapshot: { agent_name: antigravityAgentID },
        },
        [],
      ),
    ).toBe(true);
  });

  it("uses the stable agent_id from the summary snapshot", () => {
    expect(
      sessionUsesSubscriptionUsage(
        {
          agent_profile_id: antigravityProfileID,
          agent_profile_snapshot: { name: "Antigravity profile", agent_id: antigravityAgentID },
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
