"use client";

import { Button } from "@kandev/ui/button";

/** Touch-sized pill button styling shared by annotation exit controls. */
export const annotationDoneButtonClass =
  "h-7 cursor-pointer rounded-full max-md:h-11 [@media(pointer:coarse)]:h-11";

/** Pill shown over an annotation surface so the mode is obvious and easy to leave. */
export function AnnotationModeHint({
  hint,
  doneLabel,
  onDone,
  testId,
  doneTestId,
}: {
  hint: string;
  doneLabel: string;
  onDone: () => void;
  testId?: string;
  doneTestId?: string;
}) {
  return (
    <div
      className="pointer-events-auto absolute top-2 left-2 z-10 flex max-w-[calc(100%-12rem)] items-center gap-2 rounded-full border border-primary/40 bg-background/95 py-1 pr-1 pl-3 text-xs shadow-sm max-md:max-w-[calc(100%-1rem)]"
      data-testid={testId}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className="min-w-0 truncate">{hint}</span>
      <Button
        type="button"
        size="sm"
        className={annotationDoneButtonClass}
        data-testid={doneTestId}
        onClick={onDone}
      >
        {doneLabel}
      </Button>
    </div>
  );
}
