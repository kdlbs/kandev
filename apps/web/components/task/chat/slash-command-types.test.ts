import { describe, expect, it } from "vitest";
import {
  formatSlashCommandInsertion,
  mapAvailableCommandToSlashCommand,
  type SlashCommand,
} from "./slash-command-types";

function command(overrides: Partial<SlashCommand>): SlashCommand {
  return {
    id: "agent-slow",
    label: "/slow",
    description: "Run slow response",
    action: "agent",
    agentCommandName: "slow",
    ...overrides,
  };
}

describe("formatSlashCommandInsertion", () => {
  it("uses the advertised command name with a slash and trailing space", () => {
    expect(formatSlashCommandInsertion(command({ agentCommandName: "slow" }))).toBe("/slow ");
  });

  it("keeps punctuation in command names", () => {
    expect(formatSlashCommandInsertion(command({ agentCommandName: "tool:read" }))).toBe(
      "/tool:read ",
    );
  });

  it("falls back to the label without double-prefixing a slash", () => {
    expect(formatSlashCommandInsertion(command({ agentCommandName: undefined }))).toBe("/slow ");
  });

  it("adds a slash when falling back to a bare label", () => {
    expect(
      formatSlashCommandInsertion(command({ agentCommandName: undefined, label: "slow" })),
    ).toBe("/slow ");
  });
});

describe("mapAvailableCommandToSlashCommand", () => {
  it("shows a classified skill with a clean label and raw provider invocation", () => {
    const command = mapAvailableCommandToSlashCommand({
      name: "$retro",
      kind: "skill",
      description: "Run the retrospective skill",
      input_hint: "focus",
    });

    expect(command).toMatchObject({
      id: "agent-$retro",
      label: "/retro",
      kind: "skill",
      agentCommandName: "$retro",
      inputHint: "focus",
    });
    expect(formatSlashCommandInsertion(command)).toBe("/$retro ");
  });

  it("uses a fallback description only when the provider description is empty", () => {
    expect(
      mapAvailableCommandToSlashCommand({ name: "goal" }, undefined, "Fallback description")
        .description,
    ).toBe("Fallback description");
    expect(
      mapAvailableCommandToSlashCommand(
        { name: "goal", description: "Provider description" },
        undefined,
        "Fallback description",
      ).description,
    ).toBe("Provider description");
  });

  it("keeps unknown classifications and literal dollar names unchanged", () => {
    const command = mapAvailableCommandToSlashCommand({ name: "$retro", kind: "future-kind" });

    expect(command.label).toBe("/$retro");
    expect(command.kind).toBeUndefined();
    expect(formatSlashCommandInsertion(command)).toBe("/$retro ");
  });

  it("accepts only the normalized Codex plan action and derives confirmed state", () => {
    const action = {
      kind: "set_config_option",
      config_id: "collaboration_mode",
      value: "plan",
      reset_value: "default",
    };
    const command = mapAvailableCommandToSlashCommand(
      { name: "plan", action },
      { collaboration_mode: "plan" },
    );
    const defaultCommand = mapAvailableCommandToSlashCommand(
      { name: "plan", action },
      { collaboration_mode: "default" },
    );
    const unknownCommand = mapAvailableCommandToSlashCommand({ name: "plan", action });
    const unsupported = mapAvailableCommandToSlashCommand({
      name: "plan",
      action: { ...action, config_id: "approval_policy" },
    });

    expect(command.modeState).toBe("active");
    expect(defaultCommand.modeState).toBe("default");
    expect(unknownCommand.modeState).toBe("unknown");
    expect(unsupported.modeAction).toBeUndefined();
    expect(unsupported.modeState).toBeUndefined();
  });
});
