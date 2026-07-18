"use client";

import { Switch } from "@kandev/ui/switch";

export function ConfigurationChatToggle({
  checked,
  disabled,
  onCheckedChange,
}: {
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <section
      className="flex min-h-11 items-center justify-between gap-4"
      aria-labelledby="config-chat-mode-label"
    >
      <div className="min-w-0">
        <h3 id="config-chat-mode-label" className="text-sm font-medium">
          Configuration chat
        </h3>
        <p className="text-xs text-muted-foreground">
          Let the agent update Kandev settings, workflows, agent profiles, and MCP configuration.
        </p>
      </div>
      <Switch
        aria-label="Configuration chat"
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
        className="shrink-0 cursor-pointer"
      />
    </section>
  );
}
