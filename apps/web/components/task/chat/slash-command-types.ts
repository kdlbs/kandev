import { normalizeSlashCommandName } from "./tiptap-slash-command-utils";

export type SlashCommandAction = "agent";

export type SlashCommand = {
  id: string;
  label: string;
  description: string;
  action: SlashCommandAction;
  agentCommandName?: string;
};

export function formatSlashCommandInsertion(command: SlashCommand): string {
  const rawName = command.agentCommandName || command.label;
  const name = normalizeSlashCommandName(rawName);
  return `/${name} `;
}
