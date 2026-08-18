import { IconMessageCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { selectQuickChatHasUnseenIdle } from "@/lib/state/slices/ui/quick-chat-unseen-selectors";

/** Quick Chat action in the mobile task-switcher sheet, with the unseen-idle dot. */
export function QuickChatSheetButton({
  workspaceId,
  onClick,
}: {
  workspaceId: string;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const hasUnseenIdle = useAppStore((state) => selectQuickChatHasUnseenIdle(state, workspaceId));
  const quickChatLabel = t(hasUnseenIdle ? "sidebar:quickChatUnseen" : "sidebar:quickChat");
  return (
    <Button
      size="sm"
      variant="outline"
      className="h-7 gap-1 cursor-pointer"
      onClick={onClick}
      aria-label={quickChatLabel}
      data-testid="mobile-sheet-quick-chat"
    >
      <span className="relative flex">
        <IconMessageCircle className="h-4 w-4" />
        {hasUnseenIdle && (
          <span
            aria-hidden="true"
            className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-red-500 ring-2 ring-background"
            data-testid="quick-chat-unseen-dot"
          />
        )}
      </span>
      {t("task:chat")}
    </Button>
  );
}
