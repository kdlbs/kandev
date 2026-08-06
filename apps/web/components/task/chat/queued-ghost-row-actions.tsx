import {
  IconArrowMerge,
  IconChevronDown,
  IconChevronUp,
  IconEdit,
  IconSend,
  IconX,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { cn } from "@/lib/utils";

export type QueuedGhostRowActionsProps = {
  canExpand: boolean;
  expanded: boolean;
  canMerge: boolean;
  canEdit: boolean;
  canRemove: boolean;
  onToggleExpand: () => void;
  onMerge?: () => void | Promise<void>;
  onStartEdit: () => void;
  onRemove: () => void;
  onSendNow?: () => void;
  sendNowDisabled: boolean;
};

export function QueuedGhostRowActions({
  canExpand,
  expanded,
  canMerge,
  canEdit,
  canRemove,
  onToggleExpand,
  onMerge,
  onStartEdit,
  onRemove,
  onSendNow,
  sendNowDisabled,
}: QueuedGhostRowActionsProps) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "flex items-center gap-0.5 flex-shrink-0 transition-opacity",
        // Hover-reveal on devices that support hover (desktop); always
        // visible on touch surfaces where there's no hover affordance.
        "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
        "[@media(hover:none)]:opacity-100",
        "[@media(pointer:coarse)]:opacity-100",
      )}
    >
      <Button
        variant="ghost"
        size="sm"
        className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
        onClick={onSendNow}
        disabled={sendNowDisabled}
        title={t("chat:sendNowQueuedMessage")}
        data-testid="queue-entry-send-now"
      >
        <IconSend className="h-3.5 w-3.5" />
      </Button>
      {canExpand && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onToggleExpand}
          title={expanded ? t("chat:collapseMessage") : t("chat:expandMessage")}
          data-testid="queue-entry-expand"
          aria-expanded={expanded}
        >
          {expanded ? (
            <IconChevronUp className="h-3.5 w-3.5" />
          ) : (
            <IconChevronDown className="h-3.5 w-3.5" />
          )}
        </Button>
      )}
      {canMerge && onMerge && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onMerge}
          title={t("chat:mergeWithAbove")}
          data-testid="queue-entry-merge"
        >
          <IconArrowMerge className="h-3.5 w-3.5" />
        </Button>
      )}
      {canEdit && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onStartEdit}
          title={t("chat:editQueuedMessage")}
          data-testid="queue-entry-edit"
        >
          <IconEdit className="h-3.5 w-3.5" />
        </Button>
      )}
      {canRemove && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          onClick={onRemove}
          title={t("chat:removeQueuedMessage")}
          data-testid="queue-entry-remove"
        >
          <IconX className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}
