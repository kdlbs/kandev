"use client";

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
  const name = rawName.trim().replace(/^\/+/, "");
  return `/${name} `;
}
