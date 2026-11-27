"use client";

import { useTranslation } from "react-i18next";
import type { CompareState } from "@/lib/question-review/compare";
import { visibleIdentity } from "@/lib/question-review/identity";
import type { ReviewImage } from "@/lib/question-review/segments";
import type { Size } from "@/lib/question-review/viewport";
import { useCompareModeLabel } from "./review-compare-toolbar";
import { useSlotText } from "./review-identity-badges";
import type { ReviewDiffState } from "./use-review-diff";

type Props = {
  state: CompareState;
  a?: ReviewImage;
  b?: ReviewImage;
  sizes: Record<string, Size>;
  diff: ReviewDiffState;
  /** Phone status chips: separate items, and sizes only when they differ. */
  chips?: boolean;
  /** Short names of the images in A and B, as the identity badges show them. */
  names?: { a: string; b: string };
};

function sameSize(a: Size, b: Size) {
  return a.width === b.width && a.height === b.height;
}

function formatPercent(changed: number, total: number, locale: string) {
  return new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 2 }).format(
    total > 0 ? changed / total : 0,
  );
}

/** Image sizes, the difference result, and a live announcement of what the stage shows. */
export function ReviewCompareStatus({ state, a, b, sizes, diff, chips = false, names }: Props) {
  const Wrapper = chips ? FragmentWrapper : StatusRow;
  return (
    <Wrapper>
      <SizeStatus
        aSize={a ? sizes[a.id] : undefined}
        bSize={b ? sizes[b.id] : undefined}
        chips={chips}
      />
      {state.mode === "diff" && <DiffStatus diff={diff} />}
      <Announcement state={state} a={a} b={b} names={names} />
    </Wrapper>
  );
}

function SizeStatus({ aSize, bSize, chips }: { aSize?: Size; bSize?: Size; chips: boolean }) {
  const { t } = useTranslation();
  if (aSize && bSize && !sameSize(aSize, bSize)) {
    return (
      <span data-testid="question-review-sizes-differ" className="tabular-nums">
        {t("task:questionReviewSizesDiffer", {
          a: t("task:questionReviewImageSize", aSize),
          b: t("task:questionReviewImageSize", bSize),
        })}
      </span>
    );
  }
  if (chips || !aSize) return null;
  return <span className="tabular-nums">{t("task:questionReviewImageSize", aSize)}</span>;
}

function DiffStatus({ diff }: { diff: ReviewDiffState }) {
  const { t, i18n } = useTranslation();
  let text = "";
  if (diff.status === "ready") {
    text = t("task:questionReviewDiffResult", {
      count: diff.result.changed,
      percent: formatPercent(diff.result.changed, diff.result.total, i18n.language),
    });
  } else if (diff.status === "computing") text = t("task:questionReviewDiffComputing");
  else if (diff.status === "unavailable") text = t("task:questionReviewDiffUnavailable");
  return (
    <span data-testid="question-review-diff-status" role="status">
      {text}
    </span>
  );
}

function Announcement({ state, a, b, names }: Pick<Props, "state" | "a" | "b" | "names">) {
  const text = useIdentityAnnouncement(state, a, b, names);
  return (
    <span role="status" className="sr-only" data-testid="question-review-announcement">
      {text}
    </span>
  );
}

/** The identity badges' slot names, each with its image's description. */
function useIdentityAnnouncement(
  state: CompareState,
  a: ReviewImage | undefined,
  b: ReviewImage | undefined,
  names: { a: string; b: string } = { a: "", b: "" },
) {
  const { t } = useTranslation();
  const modeLabel = useCompareModeLabel();
  const slotText = useSlotText(names);
  if (!a || !b) return modeLabel(state.mode);
  const described = (slot: "a" | "b") =>
    t("task:questionReviewIdentityDescribed", {
      image: slotText(slot),
      alt: (slot === "a" ? a : b).alt,
    });
  const identity = visibleIdentity(state, false);
  switch (identity.kind) {
    case "one":
      return t("task:questionReviewShowingImage", { image: described(identity.slot) });
    case "sides":
      return t("task:questionReviewAnnounceWipe", { a: described("a"), b: described("b") });
    case "panes":
      return t("task:questionReviewAnnounceSide", { a: described("a"), b: described("b") });
    case "over":
      return t("task:questionReviewAnnounceOnion", {
        a: described("a"),
        b: described("b"),
        opacity: identity.opacity,
      });
    case "versus":
      return t("task:questionReviewAnnounceDiff", { a: described("a"), b: described("b") });
  }
}

function StatusRow({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {children}
    </div>
  );
}

function FragmentWrapper({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
