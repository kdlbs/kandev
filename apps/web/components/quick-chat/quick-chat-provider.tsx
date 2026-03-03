"use client";

import { useState, useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { QuickChatModal } from "./quick-chat-modal";
import { QuickChatPickerDialog } from "./quick-chat-dialog";

/**
 * Global provider for Quick Chat functionality.
 * Renders the picker dialog and modal that can be opened from anywhere in the app.
 */
export function QuickChatProvider({ children }: { children: React.ReactNode }) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const quickChatWorkspaceId = useAppStore((s) => s.quickChat.workspaceId);

  const handleOpenPicker = useCallback(() => {
    setPickerOpen(true);
  }, []);

  return (
    <>
      {children}
      {/* Only render picker if we have a workspace ID (from opening quick chat modal or picker) */}
      {quickChatWorkspaceId && (
        <QuickChatPickerDialog
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          workspaceId={quickChatWorkspaceId}
        />
      )}
      <QuickChatModal onNewChat={handleOpenPicker} />
    </>
  );
}

