import { describe, expect, it } from "vitest";
import { formatSlashCommandInsertion, type SlashCommand } from "./slash-command-types";

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
});
