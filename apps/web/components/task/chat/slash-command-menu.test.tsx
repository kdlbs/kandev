import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SlashCommand } from "./slash-command-types";
import { SlashCommandMenu } from "./slash-command-menu";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "task:slashCommandSkill": "Skill",
        "task:slashCommandMode": "Mode",
        "task:slashCommandActive": "Active",
        "task:slashCommandArguments": "Arguments",
        "task:slashCommandPlanModeOn": "Turn plan mode on",
        "task:slashCommandPlanModeOff": "Turn plan mode off",
        "task:slashCommandPlanModeToggle": "Toggle plan mode",
      })[key] ?? key,
  }),
}));

afterEach(cleanup);

function command(overrides: Record<string, unknown>): SlashCommand {
  return {
    id: "agent-test",
    label: "/test",
    description: "Run the command",
    action: "agent",
    ...overrides,
  } as SlashCommand;
}

describe("SlashCommandMenu", () => {
  it("shows localized classification and active state as noninteractive option content", async () => {
    const skill = command({ id: "agent-$retro", label: "/retro", kind: "skill" });
    const plan = command({
      id: "agent-plan",
      label: "/plan",
      modeAction: { kind: "set_config_option" },
      modeState: "active",
    });
    const select = vi.fn();
    render(
      <SlashCommandMenu
        isOpen
        position={{ x: 100, y: 100 }}
        commands={[skill, plan]}
        selectedIndex={0}
        onSelect={select}
        onClose={vi.fn()}
        setSelectedIndex={vi.fn()}
      />,
    );

    const rows = await screen.findAllByRole("option");
    expect(rows[0].textContent).toContain("Skill");
    expect(rows[0].getAttribute("aria-label")).toMatch(/\/retro.*Skill/);
    expect(rows[1].textContent).toContain("Mode");
    expect(rows[1].textContent).toContain("Active");
    expect(rows[1].textContent).toContain("Turn plan mode off");
    expect(rows[0].getAttribute("aria-selected")).toBe("true");

    fireEvent.click(rows[0]);
    expect(select).toHaveBeenCalledWith(skill);
  });

  it("shows a neutral plan description and an argument hint without a category", async () => {
    const plan = command({
      id: "agent-plan",
      label: "/plan",
      modeAction: { kind: "set_config_option" },
      modeState: "unknown",
    });
    const goal = command({
      id: "agent-goal",
      label: "/goal",
      description: "Set a goal to keep pursuing",
      inputHint: "<objective>|clear|pause|resume",
    });
    render(
      <SlashCommandMenu
        isOpen
        position={{ x: 100, y: 100 }}
        commands={[plan, goal]}
        selectedIndex={0}
        onSelect={vi.fn()}
        onClose={vi.fn()}
        setSelectedIndex={vi.fn()}
      />,
    );

    const rows = await screen.findAllByRole("option");
    expect(rows[0].textContent).toContain("Toggle plan mode");
    expect(rows[1].textContent).toContain("Set a goal to keep pursuing");
    expect(rows[1].textContent).toContain("Arguments: <objective>|clear|pause|resume");
    expect(rows[1].textContent).not.toContain("Skill");
    expect(rows[1].textContent).not.toContain("Mode");
  });
});
