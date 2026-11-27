import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

export function WorkflowSelectorPopoverHeader({
  showTouchClose,
  onClose,
}: {
  showTouchClose: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center justify-between border-b px-2 py-1.5 text-xs text-muted-foreground">
      <span>{t("workflows:workflow")}</span>
      {showTouchClose ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-11 w-11 shrink-0"
          aria-label={t("common:close")}
          data-testid="workflow-selector-close"
          onClick={onClose}
        >
          <IconX className="h-4 w-4" aria-hidden="true" />
        </Button>
      ) : null}
    </div>
  );
}
