"use client";

import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useAppStore } from "@/components/state-provider";

/** Quick Chat button that opens the quick chat modal */
export function QuickChatButton({ workspaceId }: { workspaceId?: string | null }) {
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const quickChatShortcut = getShortcut("QUICK_CHAT", keyboardShortcuts);

  if (!workspaceId) return null;

  return (
    <KeyboardShortcutTooltip shortcut={quickChatShortcut} description="Quick Chat">
      <Button
        size="icon-sm"
        variant="outline"
        className="cursor-pointer"
        onClick={handleOpenQuickChat}
      >
        <IconMessageCircle className="h-4 w-4" />
      </Button>
    </KeyboardShortcutTooltip>
  );
}
