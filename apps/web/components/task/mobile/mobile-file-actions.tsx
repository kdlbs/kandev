"use client";

import {
  IconChevronDown,
  IconDeviceFloppy,
  IconDots,
  IconLoader2,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import {
  capitalize,
  isMarkdownFileModeSupported,
  type MarkdownFileMode,
} from "../markdown-file-mode";

type MobileFileActionsProps = {
  path: string;
  mode?: MarkdownFileMode;
  hybridAvailable: boolean;
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onModeChange: (mode: MarkdownFileMode) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
};

const MODES: readonly MarkdownFileMode[] = ["preview", "edit", "source"];
const TOUCH_CONTROL = "h-11 min-w-11 cursor-pointer gap-1 px-2 text-xs";

function MobileModeMenu({
  path,
  mode,
  hybridAvailable,
  onModeChange,
}: Pick<MobileFileActionsProps, "path" | "hybridAvailable" | "onModeChange"> & {
  mode: MarkdownFileMode;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={`${TOUCH_CONTROL} min-w-0 flex-1 justify-between`}
          data-testid="mobile-markdown-mode-trigger"
          data-mode={mode}
          aria-label={`${t("task:markdownModes")}: ${t(`task:markdownMode${capitalize(mode)}`)}`}
        >
          <span className="truncate">{t(`task:markdownMode${capitalize(mode)}`)}</span>
          <IconChevronDown className="h-4 w-4 shrink-0" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        className="min-w-44 max-w-[calc(100vw-1rem)] overscroll-contain"
        data-testid="mobile-markdown-mode-menu"
      >
        <DropdownMenuRadioGroup
          value={mode}
          onValueChange={(next) => {
            const candidate = MODES.find((value) => value === next);
            if (candidate && (candidate !== "edit" || hybridAvailable)) onModeChange(candidate);
          }}
        >
          {MODES.filter((candidate) => isMarkdownFileModeSupported(path, candidate)).map(
            (candidate) => (
              <DropdownMenuRadioItem
                key={candidate}
                value={candidate}
                className="min-h-11 cursor-pointer"
                data-testid={`mobile-markdown-mode-${candidate}`}
                disabled={candidate === "edit" && !hybridAvailable}
                title={
                  candidate === "edit" && !hybridAvailable
                    ? t("task:markdownEditUnavailable")
                    : undefined
                }
              >
                {t(`task:markdownMode${capitalize(candidate)}`)}
              </DropdownMenuRadioItem>
            ),
          )}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function MobileFileActions({
  path,
  mode,
  hybridAvailable,
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onModeChange,
  onSave,
  onReloadFromAgent,
}: MobileFileActionsProps) {
  const { t } = useTranslation();
  return (
    <div
      className="flex min-h-14 min-w-0 items-center gap-2 border-t border-border/60 px-2 pb-[env(safe-area-inset-bottom,0px)]"
      data-testid="mobile-markdown-mode-controls"
    >
      {mode ? (
        <MobileModeMenu
          path={path}
          mode={mode}
          hybridAvailable={hybridAvailable}
          onModeChange={onModeChange}
        />
      ) : (
        <div className="min-w-0 flex-1" />
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className={`${TOUCH_CONTROL} shrink-0`}
            aria-label={t("task:moreToolbarActions")}
            data-testid="mobile-file-actions-trigger"
          >
            <IconDots className="h-4 w-4" aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="min-w-44 max-w-[calc(100vw-1rem)] overscroll-contain"
        >
          <DropdownMenuItem
            className="min-h-11 cursor-pointer"
            data-testid="mobile-file-reload"
            disabled={!hasRemoteUpdate || !onReloadFromAgent}
            onSelect={onReloadFromAgent}
          >
            <IconRefresh className="h-4 w-4" aria-hidden="true" />
            {t("common:reload")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Button
        type="button"
        variant="default"
        className={`${TOUCH_CONTROL} max-w-[45%] shrink-0 px-3`}
        disabled={!isDirty || isSaving}
        onClick={onSave}
        data-testid="mobile-file-save"
      >
        {isSaving ? (
          <IconLoader2 className="h-4 w-4 shrink-0 animate-spin" aria-hidden="true" />
        ) : (
          <IconDeviceFloppy className="h-4 w-4 shrink-0" aria-hidden="true" />
        )}
        <span className="truncate">{isSaving ? t("task:saving") : t("common:save")}</span>
      </Button>
    </div>
  );
}
