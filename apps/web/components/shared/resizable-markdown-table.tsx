"use client";

import { useTranslation } from "react-i18next";
import type { ReactNode, Ref } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { canResizeColumnBoundary, MIN_MARKDOWN_COLUMN_WIDTH } from "@/lib/markdown/table-resize";
import { useMarkdownTableResize } from "./use-markdown-table-resize";

type ResizableMarkdownTableProps = {
  children?: ReactNode;
  renderWrapper?: (props: {
    children: ReactNode;
    className: string;
    elementRef: Ref<HTMLDivElement>;
  }) => ReactNode;
};

export function ResizableMarkdownTable({ children, renderWrapper }: ResizableMarkdownTableProps) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const resizeEnabled = isFinePointer && !isMobile;
  const resize = useMarkdownTableResize(resizeEnabled);
  const displayedWidths = resize.columnWidths ?? resize.geometry?.columnWidths;

  const tableContent = (
    <>
      <table
        ref={resize.tableRef}
        style={
          resize.columnWidths && resize.fixedTableWidth
            ? { tableLayout: "fixed", width: resize.fixedTableWidth }
            : undefined
        }
      >
        {resize.columnWidths && (
          <colgroup>
            {resize.columnWidths.map((width, index) => (
              <col key={index} style={{ width }} />
            ))}
          </colgroup>
        )}
        {children}
      </table>
      {resizeEnabled &&
        resize.geometry?.boundaries.map((left, index) => {
          const leftWidth = displayedWidths?.[index];
          const rightWidth = displayedWidths?.[index + 1];
          if (
            !displayedWidths ||
            leftWidth === undefined ||
            rightWidth === undefined ||
            !canResizeColumnBoundary(displayedWidths, index)
          ) {
            return null;
          }
          return (
            <button
              key={index}
              type="button"
              role="separator"
              aria-orientation="vertical"
              aria-label={t("common:resizeTableColumns", { left: index + 1, right: index + 2 })}
              aria-valuemin={MIN_MARKDOWN_COLUMN_WIDTH}
              aria-valuemax={Math.max(
                MIN_MARKDOWN_COLUMN_WIDTH,
                leftWidth + rightWidth - MIN_MARKDOWN_COLUMN_WIDTH,
              )}
              aria-valuenow={Math.round(leftWidth)}
              data-active={resize.activeBoundary === index}
              data-testid={`markdown-table-resizer-${index}`}
              className="markdown-table-resizer"
              style={{
                height: resize.geometry?.height,
                left,
                top: resize.geometry?.top,
              }}
              onDoubleClick={resize.reset}
              onKeyDown={(event) => resize.resizeWithKeyboard(index, event)}
              onPointerCancel={(event) => resize.finishDrag(event, true)}
              onPointerDown={(event) => resize.startResize(index, event)}
              onPointerMove={resize.moveResize}
              onPointerUp={(event) => resize.finishDrag(event, false)}
            />
          );
        })}
    </>
  );

  if (renderWrapper) {
    return renderWrapper({
      children: tableContent,
      className: "markdown-table-scroll overflow-x-auto",
      elementRef: resize.wrapperRef,
    });
  }

  return (
    <div ref={resize.wrapperRef} className="markdown-table-scroll overflow-x-auto">
      {tableContent}
    </div>
  );
}
