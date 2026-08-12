"use client";

import { Separator } from "@kandev/ui/separator";
import { EditorsSettings } from "@/components/settings/editors-settings";
import { TerminalSettings } from "@/components/settings/terminal-settings";

/** Terminal & Editors: the former Terminal and Editors pages as one page. */
export function TerminalEditorsSettings() {
  return (
    <div className="space-y-8">
      <TerminalSettings />
      <Separator />
      <EditorsSettings />
    </div>
  );
}
