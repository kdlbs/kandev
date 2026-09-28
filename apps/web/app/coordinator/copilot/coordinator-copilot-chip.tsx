import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";

type CoordinatorCopilotChipRowProps = {
  chip: CopilotChip;
  onRemove: () => void;
};

/** The "about `<id>`" chip above the composer
 *  (docs/specs/coordinator/system-design/copilot-popover.md#ask-about-this).
 *  Reuses the transcript tag's translated key so the chip and the sent
 *  message's tag read the same. */
export function CoordinatorCopilotChipRow({ chip, onRemove }: CoordinatorCopilotChipRowProps) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-1 border-b px-3 py-1.5">
      <Badge variant="secondary" className="w-fit gap-1 pr-1">
        {t("chat:coordinatorAboutTag", { id: chip.id })}
        <Button
          size="icon-sm"
          variant="ghost"
          className="cursor-pointer [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          onClick={onRemove}
          aria-label={t("coordinator:copilotRemoveChip")}
        >
          <IconX className="h-3 w-3" />
        </Button>
      </Badge>
    </div>
  );
}

type CoordinatorCopilotEmptyIntroProps = {
  onSuggest: (text: string) => void;
};

/** The empty-conversation intro and one fixed suggestion, shown above the
 *  composer only while the transcript has no messages
 *  (docs/specs/coordinator/system-design/copilot-popover.md#empty-conversation). */
export function CoordinatorCopilotEmptyIntro({ onSuggest }: CoordinatorCopilotEmptyIntroProps) {
  const { t } = useTranslation();
  const suggestion = t("coordinator:copilotSuggestionQuestion");
  return (
    <div className="shrink-0 space-y-2 border-b p-3 text-sm">
      <p className="text-muted-foreground">{t("coordinator:copilotIntro")}</p>
      <p className="text-xs font-medium text-muted-foreground">
        {t("coordinator:copilotTryAsking")}
      </p>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
        onClick={() => onSuggest(suggestion)}
      >
        {suggestion}
      </Button>
    </div>
  );
}
