"use client";

import { memo, useRef, useState, type ReactNode } from "react";
import {
  IconListCheck,
  IconFile,
  IconMessageDots,
  IconPhoto,
  IconAt,
  IconGitPullRequest,
  IconRoute,
  IconX,
  IconPinFilled,
} from "@tabler/icons-react";
import type { TablerIcon } from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { HoverCard, HoverCardTrigger, HoverCardContent } from "@kandev/ui/hover-card";
import type { ContextItemKind } from "@/lib/types/context";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const ICON_BY_KIND: Record<ContextItemKind, TablerIcon> = {
  plan: IconListCheck,
  file: IconFile,
  comment: IconMessageDots,
  "plan-comment": IconMessageDots,
  "walkthrough-comment": IconRoute,
  image: IconPhoto,
  "file-attachment": IconFile,
  prompt: IconAt,
  "pr-feedback": IconGitPullRequest,
  "agent-message-comment": IconMessageDots,
};

type ContextChipProps = {
  kind: ContextItemKind;
  label: string;
  pinned?: boolean;
  preview?: ReactNode;
  /** Data URL to render as a tiny thumbnail instead of the default icon */
  thumbnail?: string;
  leadingIcon?: ReactNode;
  dataTestId?: string;
  dataPath?: string;
  dataIsDirectory?: boolean;
  onClick?: () => void;
  onUnpin?: () => void;
  onRemove?: () => void;
};

// eslint-disable-next-line max-lines-per-function -- chip variants coordinate desktop hover and mobile drawer behavior.
export const ContextChip = memo(function ContextChip({
  kind,
  label,
  pinned,
  preview,
  thumbnail,
  leadingIcon,
  dataTestId,
  dataPath,
  dataIsDirectory,
  onClick,
  onUnpin,
  onRemove,
}: ContextChipProps) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const Icon = ICON_BY_KIND[kind];
  const labelSizingClass = isFinePointer ? "min-h-0" : "min-h-11";
  let iconNode: ReactNode;
  if (leadingIcon) {
    iconNode = leadingIcon;
  } else if (thumbnail) {
    iconNode = <img src={thumbnail} alt="" className="h-3 w-3 shrink-0 rounded-sm object-cover" />;
  } else {
    iconNode = <Icon className="h-3 w-3 shrink-0" />;
  }

  const controls = (
    <>
      {pinned && onUnpin && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onUnpin();
          }}
          aria-label={t("task:unpinWillBeRemovedAfterSend")}
          className={`ml-0.5 ${
            isFinePointer ? "min-h-0 min-w-0" : "min-h-11 min-w-11"
          } cursor-pointer text-muted-foreground/70 hover:text-foreground`}
        >
          <IconPinFilled className="mx-auto h-2.5 w-2.5" />
        </button>
      )}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          aria-label={t("task:removeLabeled", { label })}
          className={`ml-0.5 ${
            isFinePointer
              ? "min-h-0 min-w-0 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
              : "min-h-11 min-w-11"
          } cursor-pointer text-muted-foreground hover:text-foreground`}
        >
          <IconX className="mx-auto h-2.5 w-2.5" />
        </button>
      )}
    </>
  );
  const labelContent = (
    <>
      {iconNode}
      <span className="truncate max-w-[120px]">{label}</span>
    </>
  );
  let labelElement: ReactNode;
  if (preview) {
    labelElement = (
      <ControlledHoverChip preview={preview} label={label} onClick={onClick}>
        {(open, usesTouchDrawer) => (
          <button
            type="button"
            onClick={usesTouchDrawer ? undefined : onClick}
            aria-haspopup="dialog"
            aria-expanded={open}
            className={`flex ${labelSizingClass} min-w-0 flex-1 items-center gap-1 text-left`}
          >
            {labelContent}
          </button>
        )}
      </ControlledHoverChip>
    );
  } else if (onClick) {
    labelElement = (
      <button
        className={`flex ${labelSizingClass} min-w-0 flex-1 items-center gap-1 text-left`}
        onClick={onClick}
      >
        {labelContent}
      </button>
    );
  } else {
    labelElement = labelContent;
  }
  const chip = (
    <div
      data-testid={dataTestId}
      data-path={dataPath}
      data-is-directory={dataIsDirectory ? "true" : "false"}
      className={`group flex items-center gap-1 px-2 py-0.5 text-xs text-muted-foreground bg-muted/50 rounded border border-border/50 ${onClick ? "cursor-pointer hover:bg-muted/80" : ""}`}
    >
      {labelElement}
      {controls}
    </div>
  );

  return chip;
});
function ControlledHoverChip({
  preview,
  label,
  onClick,
  children,
}: {
  preview: ReactNode;
  label: string;
  onClick?: () => void;
  children: (open: boolean, usesTouchDrawer: boolean) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();
  const suppressRef = useRef(false);
  const usesTouchDrawer = useTouchDrawer();

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{children(open, true)}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription className="sr-only">{label}</DrawerDescription>
          </DrawerHeader>
          <div className="max-h-[70dvh] overflow-y-auto px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {preview}
            {onClick && (
              <button
                type="button"
                className="mt-3 min-h-11 w-full rounded-md border border-border px-3 text-sm"
                onClick={() => {
                  setOpen(false);
                  onClick();
                }}
              >
                {t("common:open")}
              </button>
            )}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <HoverCard
      open={open}
      onOpenChange={(next) => {
        if (next && suppressRef.current) return;
        setOpen(next);
      }}
      openDelay={300}
      closeDelay={0}
    >
      <HoverCardTrigger
        asChild
        onClick={() => {
          suppressRef.current = true;
          setOpen(false);
          setTimeout(() => {
            suppressRef.current = false;
          }, 300);
        }}
      >
        {children(open, false)}
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
        {preview}
      </HoverCardContent>
    </HoverCard>
  );
}
