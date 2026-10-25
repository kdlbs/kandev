import { useCallback, useLayoutEffect, useRef } from "react";

export function useColumnNaturalHeight(
  stepId: string,
  onNaturalHeightChange?: (stepId: string, height: number) => void,
) {
  const columnRef = useRef<HTMLDivElement | null>(null);
  const headerRef = useRef<HTMLDivElement | null>(null);
  const contentRef = useRef<{ height: number; element: HTMLDivElement } | null>(null);
  const report = useCallback(() => {
    const column = columnRef.current;
    const content = contentRef.current;
    if (!column || !content || !onNaturalHeightChange) return;
    const columnStyle = getComputedStyle(column);
    const scrollStyle = getComputedStyle(content.element);
    const header = headerRef.current;
    const headerHeight = header
      ? header.getBoundingClientRect().height + parseFloat(getComputedStyle(header).marginBottom)
      : 0;
    const padding = [columnStyle, scrollStyle].reduce(
      (total, style) => total + parseFloat(style.paddingTop) + parseFloat(style.paddingBottom),
      0,
    );
    onNaturalHeightChange(stepId, Math.ceil(content.height + headerHeight + padding));
  }, [onNaturalHeightChange, stepId]);
  const onContentHeightChange = useCallback(
    (height: number, element: HTMLDivElement) => {
      contentRef.current = { height, element };
      report();
    },
    [report],
  );
  useLayoutEffect(() => {
    if (!onNaturalHeightChange) return;
    const observer = new ResizeObserver(report);
    if (columnRef.current) observer.observe(columnRef.current);
    if (headerRef.current) observer.observe(headerRef.current);
    report();
    return () => observer.disconnect();
  }, [onNaturalHeightChange, report]);
  return {
    columnRef,
    headerRef,
    onContentHeightChange: onNaturalHeightChange ? onContentHeightChange : undefined,
  };
}
