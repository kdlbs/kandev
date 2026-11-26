"use client";

import {
  createContext,
  useContext,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from "react";
import { wipeNameLetters } from "@/lib/question-review/identity";
import { cn } from "@/lib/utils";
import type { WipeNamesReveal } from "./use-wipe-names-reveal";

/** Names on the Wipe handle; a side that is too narrow has none. */
export type WipeNames = { a?: string; b?: string; maxLetters?: number };

type Grip = { a: string[]; b: string[] };

/** The handle's height for the current question round, owned by the review controller. */
export const WipeLiftContext = createContext<{
  value?: number;
  set: (bottom: number) => void;
} | null>(null);

const LIFT_DEFAULT_PX = 64;
const LIFT_EDGE_PX = 8;

/**
 * Names and props for the Wipe handle. Both names share one height so they sit
 * level along the line; letters are stacked elements rather than a vertical
 * writing mode, which some browsers fail to paint when clipped. Dragging the
 * handle moves it up or down to uncover the image (and calls `onDrag` with the
 * horizontal travel since the press, for a divider that also moves sideways); it stays inside the stage, also after a
 * resize, and keeps its height for the rest of the question round.
 */
export function useWipeGrip(
  names: WipeNames,
  reveal: WipeNamesReveal,
  onDrag?: (event: PointerEvent<HTMLElement>, dx: number) => void,
) {
  const a = names.a ? wipeNameLetters(names.a, names.maxLetters) : [];
  const b = names.b ? wipeNameLetters(names.b, names.maxLetters) : [];
  const height = `max(2.75rem, calc(${Math.max(a.length, b.length)} * 1em + 1rem))`;
  const round = useContext(WipeLiftContext);
  const [bottom, setBottom] = useState(round?.value ?? LIFT_DEFAULT_PX);
  const latestBottom = useRef(bottom);
  const drag = useRef<{ x: number; y: number; bottom: number; max: number } | null>(null);
  const end = () => {
    if (!drag.current) return;
    round?.set(latestBottom.current);
    drag.current = null;
    reveal.hold("handle", false);
  };
  const props = {
    style: {
      height,
      bottom: `clamp(${LIFT_EDGE_PX}px, ${bottom}px, calc(100% - ${height} - ${LIFT_EDGE_PX}px))`,
    } satisfies CSSProperties,
    "data-active": reveal.shown,
    onPointerDown: (event: PointerEvent<HTMLElement>) => {
      if (event.button > 0) return;
      event.stopPropagation();
      const handle = event.currentTarget;
      handle.setPointerCapture?.(event.pointerId);
      const stageHeight = handle.parentElement?.clientHeight ?? 0;
      const max = Math.max(LIFT_EDGE_PX, stageHeight - handle.offsetHeight - LIFT_EDGE_PX);
      drag.current = { x: event.clientX, y: event.clientY, bottom: Math.min(bottom, max), max };
      latestBottom.current = drag.current.bottom;
      reveal.hold("handle", true);
    },
    onPointerMove: (event: PointerEvent<HTMLElement>) => {
      const start = drag.current;
      if (!start || !event.currentTarget.hasPointerCapture?.(event.pointerId)) return;
      event.stopPropagation();
      onDrag?.(event, event.clientX - start.x);
      const next = Math.min(
        start.max,
        Math.max(LIFT_EDGE_PX, start.bottom + start.y - event.clientY),
      );
      latestBottom.current = next;
      setBottom(next);
    },
    onPointerUp: end,
    onPointerCancel: end,
    onLostPointerCapture: end,
  };
  return { grip: { a, b } satisfies Grip, props };
}

/**
 * Wipe handle halves around the divider: A's name on its left, B's on its
 * right, letters upright and read top to bottom, beside a wider grip on the
 * line. Render inside a `group` centred on the line; the names show while it
 * is hovered or focused, or while `data-active` is set (see `useWipeNamesReveal`).
 */
export function WipeGripHalves({ grip }: { grip: Grip }) {
  return (
    <>
      <GripHalf slot="a" letters={grip.a} />
      <GripHalf slot="b" letters={grip.b} />
      <span
        className={cn(
          "pointer-events-none absolute inset-y-0 left-1/2 w-1.5 -translate-x-1/2 rounded-full bg-primary",
          "group-focus-visible:ring-2 group-focus-visible:ring-ring group-focus-visible:ring-offset-1",
          wipeLineForcedColors,
        )}
        aria-hidden="true"
      />
    </>
  );
}

/** The divider and grip keep a system-color edge in forced colors. */
export const wipeLineForcedColors = "forced-color-adjust-none forced-colors:bg-[CanvasText]";

/**
 * Purple letters with a light 1px outline in every theme, so they read over any
 * image, shown only while the handle is hovered, focused, or held. Forced
 * colors drop the outline, so the names get a system-color backing there.
 */
const nameText = cn(
  "text-primary opacity-0 transition-opacity duration-150",
  "[text-shadow:1px_0_0_white,-1px_0_0_white,0_1px_0_white,0_-1px_0_white,1px_1px_0_white,-1px_-1px_0_white,1px_-1px_0_white,-1px_1px_0_white]",
  "group-hover:opacity-100 group-focus-visible:opacity-100 group-data-[active=true]:opacity-100",
  "forced-colors:rounded-sm forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]",
);

function GripHalf({ slot, letters }: { slot: "a" | "b"; letters: string[] }) {
  return (
    <span
      className={cn(
        "pointer-events-none absolute inset-y-0 flex flex-col items-center justify-center px-1.5 py-2 leading-none font-semibold forced-color-adjust-auto max-md:px-2 [@media(pointer:coarse)]:px-2",
        nameText,
        slot === "a" ? "right-1/2 mr-0.5" : "left-1/2 ml-0.5",
      )}
      aria-hidden="true"
      data-testid={letters.length > 0 ? "question-review-wipe-label" : undefined}
      data-slot={slot}
    >
      {letters.map((letter, index) => (
        <span key={index} className="min-h-[1em]">
          {letter}
        </span>
      ))}
    </span>
  );
}
