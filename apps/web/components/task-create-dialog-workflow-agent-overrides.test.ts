import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import {
  buildWorkflowAgentOverrideRows,
  normalizeWorkflowAgentOverrides,
  type WorkflowAgentOverrideStep,
} from "./task-create-dialog-workflow-agent-overrides";

const LUNA_PROFILE_ID = "profile-luna";
const TERRA_PROFILE_ID = "profile-terra";
const SOL_PROFILE_ID = "profile-sol";

const profiles = [
  {
    id: LUNA_PROFILE_ID,
    label: "Luna Max",
    agent_id: "agent-luna",
    agent_name: "luna",
    cli_passthrough: false,
    enabled: true,
  },
  {
    id: TERRA_PROFILE_ID,
    label: "Terra",
    agent_id: "agent-terra",
    agent_name: "terra",
    cli_passthrough: false,
    enabled: true,
  },
  {
    id: SOL_PROFILE_ID,
    label: "Sol",
    agent_id: "agent-sol",
    agent_name: "sol",
    cli_passthrough: false,
    enabled: true,
  },
] satisfies AgentProfileOption[];

const replacementOptions = profiles.map((profile) => ({
  value: profile.id,
  label: profile.label,
  renderLabel: () => profile.label,
}));

function step(
  id: string,
  title: string,
  agentProfileId: string,
  position: number,
  sessionTarget?: WorkflowAgentOverrideStep["session_target"],
): WorkflowAgentOverrideStep {
  return {
    id,
    title,
    agent_profile_id: agentProfileId,
    position,
    session_target: sessionTarget,
  };
}

describe("workflow agent override helpers", () => {
  it("groups fixed steps and their explicit step consumers by source profile", () => {
    const rows = buildWorkflowAgentOverrideRows({
      steps: [
        step("review", "Review", LUNA_PROFILE_ID, 2),
        step("implement", "Implement", LUNA_PROFILE_ID, 1),
        step("pr", "PR", LUNA_PROFILE_ID, 3, { kind: "step", step_id: "implement" }),
        step("test", "Test", SOL_PROFILE_ID, 4),
      ],
      profiles,
      replacementOptions,
      overrides: { [LUNA_PROFILE_ID]: TERRA_PROFILE_ID },
    });

    expect(rows).toEqual([
      expect.objectContaining({
        sourceProfileId: LUNA_PROFILE_ID,
        sourceLabel: "Luna Max",
        stepIds: ["implement", "review", "pr"],
        stepNames: ["Implement", "Review", "PR"],
        replacementProfileId: TERRA_PROFILE_ID,
        replacementAvailable: true,
      }),
      expect.objectContaining({
        sourceProfileId: SOL_PROFILE_ID,
        sourceLabel: "Sol",
        stepIds: ["test"],
        stepNames: ["Test"],
        replacementProfileId: "",
        replacementAvailable: true,
      }),
    ]);
  });

  it("normalizes empty and self selections without rematching another workflow profile", () => {
    expect(
      normalizeWorkflowAgentOverrides({
        [LUNA_PROFILE_ID]: LUNA_PROFILE_ID,
        [SOL_PROFILE_ID]: "",
        [TERRA_PROFILE_ID]: SOL_PROFILE_ID,
      }),
    ).toEqual({ [TERRA_PROFILE_ID]: SOL_PROFILE_ID });
  });

  it("keeps an unavailable replacement visible and invalid", () => {
    const [row] = buildWorkflowAgentOverrideRows({
      steps: [step("implement", "Implement", LUNA_PROFILE_ID, 1)],
      profiles,
      replacementOptions: replacementOptions.filter((option) => option.value !== TERRA_PROFILE_ID),
      overrides: { [LUNA_PROFILE_ID]: TERRA_PROFILE_ID },
    });

    expect(row).toEqual(
      expect.objectContaining({
        replacementProfileId: TERRA_PROFILE_ID,
        replacementLabel: "Terra",
        replacementAvailable: false,
      }),
    );
  });
});
