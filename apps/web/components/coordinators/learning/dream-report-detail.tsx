"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { useDreamDetail } from "@/hooks/domains/coordinator/use-learning";
import type {
  DreamDetail,
  DreamItem,
  DreamRating,
} from "@/lib/api/domains/coordinator-learning-api";
import { DREAM_RATINGS, isRetireItem } from "@/lib/coordinator/learning";
import { formatDateTime } from "@/lib/i18n/formats";
import { costText } from "./learning-reports";

type RatingProps = {
  item: DreamItem;
  current: DreamRating | undefined;
  canManage: boolean;
  onRate: (rating: DreamRating) => void;
};

function RatingControl({ item, current, canManage, onRate }: RatingProps) {
  const { t } = useTranslation();
  return (
    <div
      role="radiogroup"
      aria-label={t("coordinator:learningRatingLabel")}
      className="grid grid-cols-3 gap-1 sm:inline-grid"
      data-testid={`learning-rating-${item.id}`}
    >
      {DREAM_RATINGS.map((rating) => (
        <Button
          key={rating}
          type="button"
          role="radio"
          aria-checked={current === rating}
          size="sm"
          variant={current === rating ? "default" : "outline"}
          disabled={!canManage}
          className="min-h-11 cursor-pointer md:min-h-8"
          onClick={() => onRate(rating)}
          data-testid={`learning-rate-${item.id}-${rating}`}
        >
          {t(`coordinator:learningRating_${rating}`)}
        </Button>
      ))}
    </div>
  );
}

type ItemProps = {
  item: DreamItem;
  rating: DreamRating | undefined;
  canManage: boolean;
  failed: boolean;
  onRate: (rating: DreamRating) => void;
};

function ReplayLine({ item }: { item: DreamItem }) {
  const { t } = useTranslation();
  const verdict = item.replay_verdict || "unmeasured";
  const known = ["improvement", "not_an_improvement", "blocked", "unmeasured"].includes(verdict);
  return (
    <span data-testid="learning-item-replay">
      {t("coordinator:learningReplay", {
        result: known ? t(`coordinator:learningReplay_${verdict}`) : verdict,
      })}
    </span>
  );
}

function ItemCard({ item, rating, canManage, failed, onRate }: ItemProps) {
  const { t } = useTranslation();
  const refused = item.gate !== "pass";
  return (
    <li className="space-y-2 rounded-md border p-3" data-testid={`learning-item-${item.id}`}>
      <p className="flex flex-wrap gap-x-3 text-sm">
        <span className="font-medium">
          {t("coordinator:learningItemHeading", { n: item.position + 1 })}
        </span>
        <span>{t(`coordinator:learningKind_${item.kind}`, { defaultValue: item.kind })}</span>
        <span data-testid="learning-item-gate">
          {refused
            ? t("coordinator:learningGateRefused", {
                reason: t(`coordinator:learningGateReason_${item.gate}`, {
                  defaultValue: item.gate,
                }),
              })
            : t("coordinator:learningGatePass")}
        </span>
        <ReplayLine item={item} />
      </p>
      <p className="whitespace-pre-wrap text-sm">{item.text}</p>
      {item.cited_turn_ids.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("coordinator:learningCited", { turns: item.cited_turn_ids.join(", ") })}
        </p>
      )}
      <RatingControl item={item} current={rating} canManage={canManage} onRate={onRate} />
      {failed && (
        <p className="text-sm text-destructive" data-testid="learning-rating-error">
          {t("coordinator:learningRatingFailed")}
        </p>
      )}
    </li>
  );
}

function Considered({ lines }: { lines: string[] }) {
  const { t } = useTranslation();
  return (
    <section className="space-y-1" data-testid="learning-considered">
      <h4 className="text-sm font-medium">{t("coordinator:learningConsideredTitle")}</h4>
      <p className="text-xs text-muted-foreground">{t("coordinator:learningConsideredNote")}</p>
      {lines.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("coordinator:learningConsideredNone")}</p>
      ) : (
        <ul className="list-disc pl-5 text-sm">
          {lines.map((line, i) => (
            <li key={`${i}-${line}`}>{line}</li>
          ))}
        </ul>
      )}
    </section>
  );
}

function RetireSection({ items }: { items: DreamItem[] }) {
  const { t } = useTranslation();
  const retire = items.filter(isRetireItem);
  return (
    <section className="space-y-1" data-testid="learning-retire">
      <h4 className="text-sm font-medium">{t("coordinator:learningRetireTitle")}</h4>
      {retire.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("coordinator:learningRetireNone")}</p>
      ) : (
        <ul className="list-disc pl-5 text-sm">
          {retire.map((it) => (
            <li key={it.id}>
              {t(`coordinator:learningKind_${it.kind}`, { defaultValue: it.kind })}: {it.text}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Header({ dream }: { dream: DreamDetail }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 text-sm" data-testid="learning-detail-header">
      <p className="font-medium">
        {formatDateTime(dream.started_at)}.{" "}
        {t(`coordinator:learningStatus_${dream.status}`, { defaultValue: dream.status })}
      </p>
      <p className="text-muted-foreground">
        {t("coordinator:learningDetailWindow", {
          start: formatDateTime(dream.window_start),
          end: formatDateTime(dream.window_end),
        })}
        {". "}
        {t("coordinator:learningTurns", { count: dream.turn_count })}
        {". "}
        {costText(dream.cost_subcents, t)}
      </p>
      {dream.reason && (
        <p className="text-muted-foreground">
          {t("coordinator:learningFailedReason", { reason: dream.reason })}
        </p>
      )}
    </div>
  );
}

type Props = {
  workspaceId: string;
  coordinatorId: string;
  dreamId: string;
  canManage: boolean;
  onBack: () => void;
};

/** One report: its items with the rating control, what the agent considered, and retirements. */
export function DreamReportDetail({
  workspaceId,
  coordinatorId,
  dreamId,
  canManage,
  onBack,
}: Props) {
  const { t } = useTranslation();
  const detail = useDreamDetail(workspaceId, coordinatorId, dreamId);
  const dream = detail.value;
  return (
    <div className="space-y-4" data-testid="learning-detail">
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-0"
        onClick={onBack}
        data-testid="learning-detail-back"
      >
        {t("coordinator:learningBack")}
      </Button>
      {detail.status === "error" && !dream && (
        <div className="space-y-2" data-testid="learning-detail-error">
          <p className="text-sm text-destructive">{t("coordinator:learningDetailError")}</p>
          <Button
            type="button"
            variant="outline"
            className="min-h-11 cursor-pointer"
            onClick={detail.retry}
          >
            {t("coordinator:retry")}
          </Button>
        </div>
      )}
      {detail.status === "loading" && !dream && (
        <Skeleton className="h-4 w-64" data-testid="learning-detail-loading" />
      )}
      {dream && (
        <>
          <Header dream={dream} />
          {dream.items.length === 0 ? (
            <p className="text-sm text-muted-foreground" data-testid="learning-detail-no-items">
              {t("coordinator:learningNoItems")}
            </p>
          ) : (
            <ul className="space-y-3">
              {dream.items.map((item) => (
                <ItemCard
                  key={item.id}
                  item={item}
                  rating={detail.ratings[item.id] ?? item.rating}
                  canManage={canManage}
                  failed={detail.failedItem === item.id}
                  onRate={(rating) => void detail.rate(item.id, rating)}
                />
              ))}
            </ul>
          )}
          <Considered lines={dream.considered} />
          <RetireSection items={dream.items} />
        </>
      )}
    </div>
  );
}
