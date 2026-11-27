import { useEffect, useRef } from "react";

type BoardDemandOptions = {
  workflowIds: readonly string[];
  collapsedIds: readonly string[];
  focused: boolean;
  onDemandChange?: (workflowIds: string[]) => void;
};

function observeBoardScroll(
  root: HTMLElement,
  eligible: string[],
  publish: (ids: string[]) => void,
) {
  const update = () => {
    const bounds = root.getBoundingClientRect();
    if (bounds.bottom <= bounds.top) return;
    const visible = Array.from(root.querySelectorAll<HTMLElement>("[data-workflow-id]"))
      .filter((lane) => {
        const laneBounds = lane.getBoundingClientRect();
        return laneBounds.bottom > bounds.top && laneBounds.top < bounds.bottom + 300;
      })
      .map((lane) => lane.dataset.workflowId);
    publish(eligible.filter((id) => visible.includes(id)));
  };
  root.addEventListener("scroll", update, { passive: true });
  window.addEventListener("resize", update);
  update();
  return () => {
    root.removeEventListener("scroll", update);
    window.removeEventListener("resize", update);
  };
}

export function useWorkflowBoardDemand({
  workflowIds,
  collapsedIds,
  focused,
  onDemandChange,
}: BoardDemandOptions) {
  const rootRef = useRef<HTMLDivElement>(null);
  const workflowKey = JSON.stringify(workflowIds);
  const collapsedKey = JSON.stringify(collapsedIds);
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const orderedIds: string[] = JSON.parse(workflowKey);
    const collapsed = new Set<string>(JSON.parse(collapsedKey));
    const eligible = orderedIds.filter((id) => !collapsed.has(id));
    const demanded = new Set(eligible.slice(0, 1));
    const publish = () => onDemandChange?.(eligible.filter((id) => demanded.has(id)));
    publish();
    if (focused) return () => onDemandChange?.([]);
    if (typeof IntersectionObserver === "undefined") {
      const stop = observeBoardScroll(root, eligible, (ids) => onDemandChange?.(ids));
      return () => {
        stop();
        onDemandChange?.([]);
      };
    }
    let active = true;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!active) return;
        for (const entry of entries) {
          const id = (entry.target as HTMLElement).dataset.workflowId;
          if (!id || collapsed.has(id)) continue;
          if (entry.isIntersecting) demanded.add(id);
          else demanded.delete(id);
        }
        publish();
      },
      { root, rootMargin: "0px 0px 300px 0px" },
    );
    root.querySelectorAll<HTMLElement>("[data-workflow-id]").forEach((lane) => {
      if (lane.dataset.workflowId && !collapsed.has(lane.dataset.workflowId))
        observer.observe(lane);
    });
    return () => {
      active = false;
      observer.disconnect();
      onDemandChange?.([]);
    };
  }, [collapsedKey, focused, onDemandChange, workflowKey]);
  return rootRef;
}
