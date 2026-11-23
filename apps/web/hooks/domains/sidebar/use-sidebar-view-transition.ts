import type { KeyboardEvent, SyntheticEvent } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";

export function useSidebarViewTransition() {
  const pending = useAppStore((state) => Boolean(selectSidebarViews(state).transitionPending));
  const guard = (event: SyntheticEvent) => {
    if (!pending) return;
    event.preventDefault();
    event.stopPropagation();
  };
  return {
    pending,
    interactionProps: {
      "aria-disabled": pending || undefined,
      "aria-busy": pending || undefined,
      onPointerDownCapture: guard,
      onClickCapture: guard,
      onKeyDownCapture: (event: KeyboardEvent) => {
        if (["Enter", " ", "ArrowDown", "ArrowUp"].includes(event.key)) guard(event);
      },
    },
  };
}
