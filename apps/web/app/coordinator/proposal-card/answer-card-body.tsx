"use client";

import { Trans, useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardDescription } from "@kandev/ui/card";
import TaskLink from "@/components/routing/task-link";
import type { AnswerProposal, AnswerSpec } from "@/lib/api/domains/coordinator-api";
import { isHttpUrl } from "@/lib/coordinator/answer-sources";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

const CONFIDENCE_KEYS: Record<string, string> = {
  low: "coordinator:answerConfidenceLow",
  med: "coordinator:answerConfidenceMed",
  high: "coordinator:answerConfidenceHigh",
};

export function answerSpecOf(proposal: AnswerProposal): AnswerSpec {
  return proposal.final_spec ?? proposal.spec;
}

function Sources({ spec }: { spec: AnswerSpec }) {
  const { t } = useTranslation();
  if (spec.sources.length === 0) return null;
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs"
      data-testid="answer-sources"
    >
      <span className="text-muted-foreground">{t("coordinator:answerSources")}</span>
      {spec.sources.map((source, index) => (
        <span key={`${source.ref}-${index}`} className="inline-flex items-center gap-1">
          {isHttpUrl(source.ref) ? (
            <a
              href={source.ref}
              target="_blank"
              rel="noopener noreferrer"
              className="cursor-pointer underline"
            >
              {source.label || source.ref}
            </a>
          ) : (
            <span>{source.label ? `${source.label} (${source.ref})` : source.ref}</span>
          )}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-6 cursor-pointer px-1 text-xs"
            onClick={() => void copyToClipboard(source.ref)}
          >
            {t("coordinator:answerSourceCopy")}
          </Button>
        </span>
      ))}
    </div>
  );
}

function ClarificationDraft({ spec }: { spec: AnswerSpec }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      {(spec.snapshot.questions ?? []).map((question) => {
        const entry = spec.draft.answers?.find((e) => e.question_id === question.id);
        const chosen = (entry?.option_ids ?? []).map(
          (id) => question.options.find((o) => o.id === id)?.label ?? id,
        );
        if (entry?.custom_text) chosen.push(entry.custom_text);
        return (
          <div key={question.id} className="text-sm" data-testid={`answer-question-${question.id}`}>
            <p className="font-medium">
              {t("coordinator:answerQuestion", { text: question.text })}
            </p>
            <p className="text-muted-foreground">
              {t("coordinator:answerDraft", { draft: chosen.join(", ") })}
            </p>
          </div>
        );
      })}
    </div>
  );
}

function PermissionDraft({ spec }: { spec: AnswerSpec }) {
  const { t } = useTranslation();
  const permission = spec.snapshot.permission;
  const chosen = permission?.options.find((o) => o.id === spec.draft.option_id);
  return (
    <div className="text-sm" data-testid="answer-permission">
      <p className="font-medium">
        {t("coordinator:answerPermission", { title: permission?.title ?? "" })}
      </p>
      {permission?.summary && <p className="text-muted-foreground">{permission.summary}</p>}
      <p className="text-muted-foreground">
        {t("coordinator:answerDraft", { draft: chosen?.name ?? spec.draft.option_id ?? "" })}
      </p>
    </div>
  );
}

/** Title, question, draft, sources, confidence and rationale of an answer card. */
export function AnswerCardBody({
  proposal,
  label,
  known,
}: {
  proposal: AnswerProposal;
  label: string;
  known: boolean;
}) {
  const { t } = useTranslation();
  const spec = answerSpecOf(proposal);
  const confidenceKey = CONFIDENCE_KEYS[spec.confidence];
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm font-medium">
          <Trans
            i18nKey="coordinator:kindTitleAnswer"
            values={{ card: label }}
            components={{
              card: known ? (
                <TaskLink taskId={spec.task_id} className="cursor-pointer underline" />
              ) : (
                <span />
              ),
            }}
          />
        </p>
        {spec.needs_decision && (
          <Badge variant="outline" data-testid="answer-needs-decision">
            {t("coordinator:answerNeedsDecision")}
          </Badge>
        )}
      </div>
      {spec.target === "permission" ? (
        <PermissionDraft spec={spec} />
      ) : (
        <ClarificationDraft spec={spec} />
      )}
      <CardDescription data-testid="answer-confidence">
        {t("coordinator:answerConfidence", {
          level: confidenceKey ? t(confidenceKey) : spec.confidence,
        })}
      </CardDescription>
      <Sources spec={spec} />
      {spec.rationale && <CardDescription>{spec.rationale}</CardDescription>}
    </>
  );
}
