"use client";

import { useEffect, type RefObject } from "react";
import { getAncestorPaths } from "./file-tree-utils";
import { ROOT_FOCUS_EVENT } from "./root-context-menu";

export function useFileBrowserRootFocus(
  containerRef: RefObject<HTMLElement | null>,
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>,
) {
  useEffect(() => {
    const handleRootFocus = (event: Event) => {
      const detail = (event as CustomEvent<{ surface?: string; path?: string }>).detail;
      if (detail?.surface !== "files" || !detail.path) return;
      const path = detail.path;
      setExpandedPaths((previous) => new Set([...previous, ...getAncestorPaths(path)]));
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          const nodes = containerRef.current?.querySelectorAll("[data-path]") ?? [];
          const target = Array.from(nodes).find((node) => node.getAttribute("data-path") === path);
          target?.scrollIntoView({ block: "nearest" });
        });
      });
    };
    window.addEventListener(ROOT_FOCUS_EVENT, handleRootFocus);
    return () => window.removeEventListener(ROOT_FOCUS_EVENT, handleRootFocus);
  }, [containerRef, setExpandedPaths]);
}
