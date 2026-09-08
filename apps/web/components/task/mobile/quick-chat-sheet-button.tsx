import { IconMessageCircle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { QuickChatActivityIndicator } from "@/components/quick-chat/quick-chat-activity-indicator";
import { useQuickChatActivity } from "@/components/quick-chat/use-quick-chat-activity";

/** Quick Chat action in the mobile task-switcher sheet, with activity state. */
export function QuickChatSheetButton({
  workspaceId,
  onClick,
}: {
  workspaceId: string;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const { activity: quickChatActivity, label: quickChatLabel } = useQuickChatActivity(workspaceId);
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
        <QuickChatActivityIndicator activity={quickChatActivity} />
      </span>
      {t("task:chat")}
    </Button>
  );
}
