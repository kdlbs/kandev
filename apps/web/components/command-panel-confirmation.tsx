import type { CommandItem } from "@/lib/commands/types";

type CommandGroup = [string, CommandItem[]];

export function getCommandConfirmationState(commands: CommandItem[], grouped: CommandGroup[]) {
  const confirmationCommand = commands.find((command) => command.confirmation);
  const visibleCommands = confirmationCommand
    ? commands.filter((command) => !command.confirmation)
    : commands;
  const visibleIds = new Set(visibleCommands.map((command) => command.id));
  const visibleGroups = confirmationCommand
    ? grouped.map(
        ([group, items]) =>
          [group, items.filter((command) => visibleIds.has(command.id))] as CommandGroup,
      )
    : grouped;
  return { confirmationCommand, visibleCommands, visibleGroups };
}

export function dismissCommandConfirmation(commands: CommandItem[]) {
  commands.find((command) => command.confirmation)?.onConfirmationDismiss?.();
}

export function CommandPanelConfirmation({ command }: { command: CommandItem }) {
  return (
    <div
      role="alertdialog"
      aria-label={command.label}
      data-testid="command-panel-confirmation"
      className="border-b border-border p-3"
    >
      {command.confirmation}
    </div>
  );
}
