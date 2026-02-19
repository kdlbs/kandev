"use client";

import { memo, useRef, useState, type ReactNode } from "react";
import {
  IconListCheck,
  IconFile,
  IconMessageDots,
  IconPhoto,
  IconAt,
  IconX,
  IconPinFilled,
} from "@tabler/icons-react";
import type { TablerIcon } from "@tabler/icons-react";
import { HoverCard, HoverCardTrigger, HoverCardContent } from "@kandev/ui/hover-card";
import type { ContextItemKind } from "@/lib/types/context";

const ICON_BY_KIND: Record<ContextItemKind, TablerIcon> = {
  plan: IconListCheck,
  file: IconFile,
  comment: IconMessageDots,
  "plan-comment": IconMessageDots,
  image: IconPhoto,
  prompt: IconAt,
};

type ContextChipProps = {
  kind: ContextItemKind;
  label: string;
  pinned?: boolean;
  preview?: ReactNode;
  /** Data URL to render as a tiny thumbnail instead of the default icon */
  thumbnail?: string;
  onClick?: () => void;
  onUnpin?: () => void;
  onRemove?: () => void;
};

export const ContextChip = memo(function ContextChip({
  kind,
  label,
  pinned,
  preview,
  thumbnail,
  onClick,
  onUnpin,
  onRemove,
}: ContextChipProps) {
  const Icon = ICON_BY_KIND[kind];

  const chip = (
    <div
      className={`group flex items-center gap-1 px-2 py-0.5 text-xs text-muted-foreground bg-muted/50 rounded border border-border/50 ${onClick ? "cursor-pointer hover:bg-muted/80" : ""}`}
      onClick={onClick}
    >
      {thumbnail ? (
        // eslint-disable-next-line @next/next/no-img-element -- base64 thumbnail
        <img src={thumbnail} alt="" className="h-3 w-3 shrink-0 rounded-sm object-cover" />
      ) : (
        <Icon className="h-3 w-3 shrink-0" />
      )}
      <span className="truncate max-w-[120px]">{label}</span>
      {pinned && onUnpin && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onUnpin();
          }}
          className="ml-0.5 text-muted-foreground/70 hover:text-foreground cursor-pointer"
          title="Unpin (will be removed after send)"
        >
          <IconPinFilled className="h-2.5 w-2.5" />
        </button>
      )}
      {onRemove && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className="opacity-0 group-hover:opacity-100 ml-0.5 hover:text-foreground cursor-pointer"
        >
          <IconX className="h-2.5 w-2.5" />
        </button>
      )}
    </div>
  );

  if (!preview) return chip;

  return <ControlledHoverChip preview={preview}>{chip}</ControlledHoverChip>;
});

function ControlledHoverChip({ preview, children }: { preview: ReactNode; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const suppressRef = useRef(false);

  return (
    <HoverCard
      open={open}
      onOpenChange={(next) => {
        if (next && suppressRef.current) return;
        setOpen(next);
      }}
      openDelay={300}
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
        {children}
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
        {preview}
      </HoverCardContent>
    </HoverCard>
  );
}
