"use client";

import { useTranslation } from "react-i18next";
import { visibleIdentity } from "@/lib/question-review/identity";
import { cn } from "@/lib/utils";
import type { CompareStageModel } from "./use-compare-stage";

// i18n-exempt: comparison slot identifiers, written the same in every locale
const SLOT_LETTER = { a: "A", b: "B" } as const;
const labelSurface =
  "pointer-events-none border border-border bg-background/90 font-medium shadow-sm forced-color-adjust-auto";

type Slot = "a" | "b";

/** "A · before": the slot letter and the image's label or number. */
export function useSlotText(names: { a: string; b: string }) {
  const { t } = useTranslation();
  return (slot: Slot) =>
    t("task:questionReviewIdentitySlot", { slot: SLOT_LETTER[slot], name: names[slot] });
}

const badgeTestId = "question-review-identity-badge";
const layerTag = "text-[10px] tracking-wide text-muted-foreground uppercase";

function Badge({ text }: { text: string }) {
  return (
    <span
      className={cn(
        labelSurface,
        "flex min-h-9 max-w-full items-center rounded-full px-3 py-1 break-words max-md:min-h-11",
      )}
      aria-hidden="true"
      data-testid={badgeTestId}
    >
      {text}
    </span>
  );
}

/** Onion skin as a layers list: the top row is the image drawn on top. */
function LayerBadge({ top, below, opacity }: { top: string; below: string; opacity: number }) {
  const { t } = useTranslation();
  return (
    <span
      className={cn(
        labelSurface,
        "grid max-w-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 gap-y-0.5 rounded-2xl px-3 py-1.5",
      )}
      aria-hidden="true"
      data-testid={badgeTestId}
    >
      <span className={layerTag}>{t("task:questionReviewLayerTop")}</span>
      <span className="min-w-0 break-words">{top}</span>
      <span className="text-muted-foreground tabular-nums">
        {t("task:questionReviewPercent", { value: opacity })}
      </span>
      <span className={layerTag}>{t("task:questionReviewLayerBelow")}</span>
      <span className="col-span-2 min-w-0 break-words">{below}</span>
    </span>
  );
}

/**
 * Which image the stage or pane shows, for `StageBottomRow`. Wipe names its
 * sides on the handle instead (`WipeGripHalves`). The stage's status region
 * announces the same facts, so the badge is hidden from assistive technology.
 */
export function IdentityBadges({
  model,
  pane,
}: {
  model: CompareStageModel;
  pane: "a" | "b" | "both";
}) {
  const { t } = useTranslation();
  const slotText = useSlotText(model.names);
  const identity = visibleIdentity(model.state, model.hold.holding);
  if (identity.kind === "panes") return pane === "both" ? null : <Badge text={slotText(pane)} />;
  if (pane !== "both") return null;
  switch (identity.kind) {
    case "one":
      return <Badge text={slotText(identity.slot)} />;
    case "sides":
      return null;
    case "over":
      return <LayerBadge top={slotText("b")} below={slotText("a")} opacity={identity.opacity} />;
    case "versus":
      return (
        <Badge
          text={t("task:questionReviewIdentityVersus", { a: slotText("a"), b: slotText("b") })}
        />
      );
  }
}
