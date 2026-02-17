export type ContextMenuState = {
  x: number;
  y: number;
  lineNumber: number;
  side: 'original' | 'modified';
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
const menuCls = 'fixed z-50 min-w-32 rounded-lg border bg-popover text-popover-foreground shadow-md ring-1 ring-foreground/10 p-1';
const itemCls = 'flex w-full cursor-default select-none items-center gap-2 rounded-md px-2 py-1 text-xs outline-hidden focus:bg-accent focus:text-accent-foreground hover:bg-accent hover:text-accent-foreground';
const separatorCls = 'bg-border/50 -mx-1 my-1 h-px';

export function DiffViewerContextMenu({
  contextMenu,
  onCopyAllChanged,
  onClose,
  onRevert,
  filePath,
}: DiffViewerContextMenuProps) {
  return (
    <div
      role="menu"
      className={menuCls}
      style={{ left: contextMenu.x, top: contextMenu.y }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <button role="menuitem" className={itemCls} onClick={onCopyAllChanged}>
        Copy all changed lines
      </button>
      {contextMenu.isChangedLine && (
        <button
          role="menuitem"
          className={itemCls}
          onClick={() => {
            navigator.clipboard.writeText(contextMenu.lineContent);
            onClose();
          }}
        >
          Copy line {contextMenu.lineNumber}
        </button>
      )}
      {onRevert && (
        <>
          <div className={separatorCls} />
          <button
            role="menuitem"
            className={`${itemCls} text-destructive focus:text-destructive`}
            onClick={() => {
              onRevert(filePath);
              onClose();
            }}
          >
            Revert all changes
          </button>
        </>
      )}
    </div>
  );
}
