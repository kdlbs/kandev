"use client";

import { useTranslation } from "react-i18next";

type AppSidebarResizeHandleProps = {
  onMouseDown: (e: React.MouseEvent) => void;
};

/**
 * Drag handle centered over the right edge of the expanded AppSidebar.
 * The full hit area highlights to match the dockview resize sashes.
 */
export function AppSidebarResizeHandle({ onMouseDown }: AppSidebarResizeHandleProps) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      aria-label={t("sidebar:resizeSidebar")}
      onMouseDown={onMouseDown}
      tabIndex={-1}
      className="absolute -right-px top-0 z-10 h-full w-1 translate-x-1/2 cursor-ew-resize bg-transparent transition-colors hover:bg-primary active:bg-primary"
    />
  );
}
