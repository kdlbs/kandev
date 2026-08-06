import { cn } from "@/lib/utils";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useTranslation } from "react-i18next";

export type ContextMenuState = {
  x: number;
  y: number;
  lineNumber: number;
  side: "original" | "modified";
  isChangedLine: boolean;
  lineContent: string;
} | null;

interface DiffViewerContextMenuProps {
  contextMenu: NonNullable<ContextMenuState>;
  onCopyAllChanged: () => void;
  onClose: () => void;
  onRevert?: (filePath: string) => void;
  filePath: string;
}

// Matches shadcn ContextMenuContent / ContextMenuItem class names.
// We can't use the full Radix ContextMenu tree because the menu is opened
// programmatically from Monaco's onContextMenu event at arbitrary coordinates.
const menuCls =
  "fixed z-50 min-w-32 rounded-lg border bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10 p-1";
const itemCls =
  "flex w-full cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1 text-xs outline-hidden focus:bg-muted focus:text-foreground hover:bg-muted hover:text-foreground";
const separatorCls = "bg-border/50 -mx-1 my-1 h-px";

export function DiffViewerContextMenu({
  contextMenu,
  onCopyAllChanged,
  onClose,
  onRevert,
  filePath,
}: DiffViewerContextMenuProps) {
  const { t } = useTranslation();
  return (
    <div
      role="menu"
      className={menuCls}
      style={{ left: contextMenu.x, top: contextMenu.y }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <button role="menuitem" className={itemCls} onClick={onCopyAllChanged}>
        {t("editors:copyAllChangedLines")}
      </button>
      {contextMenu.isChangedLine && (
        <button
          role="menuitem"
          className={itemCls}
          onClick={() => {
            void copyToClipboard(contextMenu.lineContent);
            onClose();
          }}
        >
          {t("editors:copyLine", { lineNumber: contextMenu.lineNumber })}
        </button>
      )}
      {onRevert && (
        <>
          <div className={separatorCls} />
          <button
            role="menuitem"
            className={cn(
              itemCls,
              "text-destructive focus:bg-destructive/10 focus:text-destructive hover:bg-destructive/10 hover:text-destructive",
            )}
            onClick={() => {
              onRevert(filePath);
              onClose();
            }}
          >
            {t("editors:revertAllChanges")}
          </button>
        </>
      )}
    </div>
  );
}
