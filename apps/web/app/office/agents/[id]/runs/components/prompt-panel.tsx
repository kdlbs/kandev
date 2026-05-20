"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { RunDetail } from "@/lib/api/domains/office-extended-api";

type Props = {
  run: Pick<RunDetail, "assembled_prompt" | "summary_injected" | "result_json">;
};

/**
 * Prompt panel — surfaces the inspection fields persisted by the
 * heartbeat-rework PR 1: the assembled prompt the agent received,
 * the continuation-summary slice that was prepended (taskless runs
 * only), and the structured adapter output captured at run finish.
 *
 * All three fields are collapsible so the panel stays compact when
 * empty / for short runs. Renders nothing when the run row pre-dates
 * the rework (every field empty).
 */
export function PromptPanel({ run }: Props) {
  const assembled = run.assembled_prompt ?? "";
  const summary = run.summary_injected ?? "";
  const result = run.result_json ?? "";
  const hasResult = result !== "" && result !== "{}";
  const hasAny = assembled !== "" || summary !== "" || hasResult;
  if (!hasAny) return null;

  return (
    <div className="rounded-lg border border-border" data-testid="prompt-panel">
      <div className="px-4 py-2 border-b border-border">
        <h2 className="text-sm font-semibold">Prompt</h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          What the agent received and produced.
        </p>
      </div>
      <div className="divide-y divide-border">
        {summary !== "" && (
          <PromptSection
            label="Injected continuation summary"
            content={summary}
            testid="prompt-summary-injected"
          />
        )}
        {assembled !== "" && (
          <PromptSection
            label="Assembled prompt"
            content={assembled}
            testid="prompt-assembled"
            defaultOpen
          />
        )}
        {hasResult && (
          <PromptSection
            label="Structured result"
            content={formatJSONIfPossible(result)}
            testid="prompt-result-json"
          />
        )}
      </div>
    </div>
  );
}

function PromptSection({
  label,
  content,
  testid,
  defaultOpen = false,
}: {
  label: string;
  content: string;
  testid: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger
        className="w-full flex items-center justify-between px-4 py-2 cursor-pointer hover:bg-muted/40"
        data-testid={`${testid}-trigger`}
      >
        <span className="text-xs font-medium">{label}</span>
        <IconChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
      </CollapsibleTrigger>
      <CollapsibleContent className="px-4 pb-3">
        <pre
          className="text-xs font-mono whitespace-pre-wrap break-words bg-muted/40 rounded p-3 max-h-[480px] overflow-auto"
          data-testid={testid}
        >
          {content}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

function formatJSONIfPossible(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
