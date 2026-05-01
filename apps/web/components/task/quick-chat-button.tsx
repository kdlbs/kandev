"use client";

import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useAppStore } from "@/components/state-provider";
import type { ComponentProps } from "react";

type QuickChatButtonProps = {
  workspaceId?: string | null;
  size?: ComponentProps<typeof Button>["size"];
};

/** Quick Chat button that opens the quick chat modal */
export function QuickChatButton({ workspaceId, size = "default" }: QuickChatButtonProps) {
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const quickChatShortcut = getShortcut("QUICK_CHAT", keyboardShortcuts);

  if (!workspaceId) return null;

  return (
    <KeyboardShortcutTooltip shortcut={quickChatShortcut} description="Quick Chat">
      <Button
        variant="outline"
        size={size}
        className="cursor-pointer gap-2"
        onClick={handleOpenQuickChat}
      >
        <IconMessageCircle className="h-4 w-4" />
        Chat
      </Button>
    </KeyboardShortcutTooltip>
  );
}
