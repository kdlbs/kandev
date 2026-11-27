import { normalizeSlashCommandName } from "./tiptap-slash-command-utils";

export type SlashCommandModeAction = {
  kind: "set_config_option";
  config_id: "collaboration_mode";
  value: "plan";
  reset_value: "default";
};

export type SlashCommandModeState = "active" | "default" | "unknown";
export type SlashCommandAction = "agent";

export type SlashCommand = {
  id: string;
  label: string;
  description: string;
  action: SlashCommandAction;
  agentCommandName?: string;
  kind?: "skill";
  modeAction?: SlashCommandModeAction;
  modeState?: SlashCommandModeState;
  inputHint?: string;
};

type AvailableCommandInput = {
  name: string;
  description?: string;
  input_hint?: string;
  kind?: unknown;
  action?: unknown;
};

function normalizedPlanAction(action: unknown): SlashCommandModeAction | undefined {
  if (!action || typeof action !== "object" || Array.isArray(action)) return undefined;
  const fields = action as Record<string, unknown>;
  if (
    fields.kind !== "set_config_option" ||
    fields.config_id !== "collaboration_mode" ||
    fields.value !== "plan" ||
    fields.reset_value !== "default"
  ) {
    return undefined;
  }
  return {
    kind: "set_config_option",
    config_id: "collaboration_mode",
    value: "plan",
    reset_value: "default",
  };
}

function confirmedModeState(
  action: SlashCommandModeAction | undefined,
  confirmedConfigOptions: Record<string, string> | undefined,
): SlashCommandModeState | undefined {
  if (!action) return undefined;
  const confirmedValue = confirmedConfigOptions?.[action.config_id];
  if (confirmedValue === action.value) return "active";
  if (confirmedValue === action.reset_value) return "default";
  return "unknown";
}

export function mapAvailableCommandToSlashCommand(
  command: AvailableCommandInput,
  confirmedConfigOptions?: Record<string, string>,
  fallbackDescription = "",
): SlashCommand {
  const kind = command.kind === "skill" ? "skill" : undefined;
  const displayName =
    kind === "skill" && command.name.startsWith("$") && command.name.length > 1
      ? command.name.slice(1)
      : command.name;
  const modeAction = normalizedPlanAction(command.action);
  const modeState = confirmedModeState(modeAction, confirmedConfigOptions);

  return {
    id: `agent-${command.name}`,
    label: `/${displayName}`,
    description: command.description || fallbackDescription,
    action: "agent",
    agentCommandName: command.name,
    ...(kind ? { kind } : {}),
    ...(modeAction ? { modeAction, modeState } : {}),
    ...(command.input_hint ? { inputHint: command.input_hint } : {}),
  };
}

export function formatSlashCommandInsertion(command: SlashCommand): string {
  const rawName = command.agentCommandName || command.label;
  const name = normalizeSlashCommandName(rawName);
  return `/${name} `;
}
