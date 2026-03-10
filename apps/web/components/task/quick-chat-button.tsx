"use client";

import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";

/** Quick Chat button that opens the quick chat modal */
export function QuickChatButton({ workspaceId }: { workspaceId?: string | null }) {
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  if (!workspaceId) return null;

  return (
    <KeyboardShortcutTooltip shortcut={SHORTCUTS.QUICK_CHAT} description="Quick Chat">
      <Button
        size="sm"
        variant="outline"
        className="cursor-pointer px-2"
        onClick={handleOpenQuickChat}
      >
        <IconMessageCircle className="h-4 w-4" />
      </Button>
    </KeyboardShortcutTooltip>
  );
}
