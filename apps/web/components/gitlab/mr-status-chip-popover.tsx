"use client";

import { useEffect } from "react";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useMRPopoverInteractions } from "./mr-topbar-button";
import { MRCIPopover } from "./mr-ci-popover";
import { MRStatusChipTriggerContent, useMRChipTriggerAttrs } from "./mr-status-chip-trigger";
import {
  useFrozenChipSelection,
  useTriggerOutsideGuard,
  type ChipAutomation,
} from "./mr-status-chip-selection";
import type { MRChipStatus } from "./mr-task-icon";
import type { TaskMR } from "@/lib/types/gitlab";

/**
 * Fine-pointer disclosure variant: reuses the topbar button's own hover
 * lifecycle (useMRPopoverInteractions) so the 13-handler wiring can't drift
 * from `MRTopbarButton`'s (spec: Accessibility, "prefer exporting and
 * reusing" over hand-copying).
 */
export function MRStatusChipPopover({
  mrs,
  openCount,
  liveSelected,
  liveStatus,
  automation,
  taskId,
  canLink,
  onUnlink,
  onLink,
}: {
  mrs: TaskMR[];
  openCount: number;
  liveSelected: TaskMR;
  liveStatus: MRChipStatus;
  automation: ChipAutomation;
  taskId: string;
  canLink: boolean;
  onUnlink: (associationId: string) => void;
  onLink: () => void;
}) {
  const { open, onOpenChange, onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave } =
    useMRPopoverInteractions();
  const { actedOnMR, frozen, shouldForceClose } = useFrozenChipSelection(mrs, liveSelected, open);
  const { ref, onPointerDownOutside } = useTriggerOutsideGuard();

  useEffect(() => {
    if (shouldForceClose) onOpenChange(false);
  }, [shouldForceClose, onOpenChange]);

  const attrs = useMRChipTriggerAttrs({ openCount, liveStatus, actedOnMR, frozen });
  // The chip closes its own disclosure before the link dialog opens
  // (spec: Link and unlink from the chip, "closing first is required").
  const handleLink = () => {
    onOpenChange(false);
    onLink();
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>
        <button
          ref={ref}
          type="button"
          {...attrs}
          onMouseOver={onTriggerEnter}
          onMouseEnter={onTriggerEnter}
          onMouseMove={onTriggerEnter}
          onPointerOver={onTriggerEnter}
          onPointerEnter={onTriggerEnter}
          onPointerMove={onTriggerEnter}
          onFocus={onTriggerEnter}
          onMouseLeave={onTriggerLeave}
          onPointerLeave={onTriggerLeave}
          onBlur={onTriggerLeave}
        >
          <MRStatusChipTriggerContent
            openCount={openCount}
            liveStatus={liveStatus}
            automation={automation}
          />
        </button>
      </PopoverAnchor>
      <PopoverContent
        data-testid="mr-status-chip-popover"
        align="end"
        sideOffset={4}
        className="w-80"
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onPointerDownOutside={onPointerDownOutside}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        {/* shouldForceClose fires the same render `actedOnMR` falls back
            from the vanished frozen MR to the new live selection — before
            the effect below has a chance to close the disclosure. Skipping
            the render here for that one commit means the popover is never
            shown with merge/unlink controls wired to a swapped-in MR the
            user did not open it on (spec: "an action target that swaps
            under the pointer mid-interaction is a hazard"). */}
        {!shouldForceClose && (
          <MRCIPopover
            mr={actedOnMR}
            taskId={taskId}
            enabled={open}
            canLink={canLink}
            onLink={handleLink}
            onUnlink={() => onUnlink(actedOnMR.id)}
          />
        )}
      </PopoverContent>
    </Popover>
  );
}
