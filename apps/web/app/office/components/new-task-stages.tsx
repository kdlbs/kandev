"use client";

import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

// Execution policy stage types

export type StagesDraft = {
  enabled: boolean;
  reviewerIds: string[];
  approverId: string;
  autoCommit: boolean;
};

export const EMPTY_STAGES: StagesDraft = {
  enabled: false,
  reviewerIds: [],
  approverId: "",
  autoCommit: false,
};

export function buildExecutionPolicy(stages: StagesDraft): string | undefined {
  if (!stages.enabled) return undefined;
  const policy: Record<string, unknown> = {
    stages: [] as unknown[],
  };
  const stageList: unknown[] = [];
  if (stages.reviewerIds.length > 0) {
    stageList.push({ type: "review", reviewers: stages.reviewerIds });
  }
  if (stages.approverId) {
    stageList.push({ type: "approval", approver: stages.approverId });
  }
  stageList.push({ type: "ship", auto_commit: stages.autoCommit });
  policy.stages = stageList;
  return JSON.stringify(policy);
}

// --- Sub-components ---

function AgentMultiSelect({
  agents,
  selectedIds,
  onChange,
  placeholder,
}: {
  agents: AgentProfile[];
  selectedIds: string[];
  onChange: (ids: string[]) => void;
  placeholder: string;
}) {
  const toggle = (id: string) => {
    onChange(selectedIds.includes(id) ? selectedIds.filter((x) => x !== id) : [...selectedIds, id]);
  };
  const label =
    selectedIds.length > 0
      ? selectedIds.map((id) => agents.find((a) => a.id === id)?.name ?? id).join(", ")
      : placeholder;

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer h-7 text-xs max-w-[180px] truncate"
        >
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-52 p-1" align="start">
        {agents.map((agent) => (
          <button
            key={agent.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer flex items-center gap-2"
            onClick={() => toggle(agent.id)}
          >
            <span
              className={`h-3 w-3 rounded-sm border shrink-0 ${
                selectedIds.includes(agent.id)
                  ? "bg-primary border-primary"
                  : "border-muted-foreground"
              }`}
            />
            {agent.name}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

function AgentSingleSelect({
  agents,
  selectedId,
  onChange,
  placeholder,
}: {
  agents: AgentProfile[];
  selectedId: string;
  onChange: (id: string) => void;
  placeholder: string;
}) {
  const { t } = useTranslation();
  const selected = agents.find((a) => a.id === selectedId);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          {selected?.name ?? placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <button
          type="button"
          className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer"
          onClick={() => onChange("")}
        >
          {t("office:none")}
        </button>
        {agents.map((agent) => (
          <button
            key={agent.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer"
            onClick={() => onChange(agent.id)}
          >
            {agent.name}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

// --- Main component ---

type Props = {
  stages: StagesDraft;
  onUpdate: (patch: Partial<StagesDraft>) => void;
};

export function NewTaskStages({ stages, onUpdate }: Props) {
  const { t } = useTranslation();
  const agents = useAppStore(selectOfficeAgentProfiles);

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        type="button"
        className="w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-muted transition-colors cursor-pointer"
        onClick={() => onUpdate({ enabled: !stages.enabled })}
      >
        {stages.enabled ? (
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        )}
        <span className="font-medium">{t("office:reviewStages")}</span>
        <span className="text-xs text-muted-foreground ml-auto">{t("office:optional")}</span>
      </button>

      {stages.enabled && (
        <div className="px-3 pb-3 pt-1 space-y-2.5 border-t border-border/50">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground w-20 shrink-0">{t("office:reviewers")}</span>
            <AgentMultiSelect
              agents={agents}
              selectedIds={stages.reviewerIds}
              onChange={(ids) => onUpdate({ reviewerIds: ids })}
              placeholder={t("office:addReviewers")}
            />
          </div>
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground w-20 shrink-0">{t("office:approver")}</span>
            <AgentSingleSelect
              agents={agents}
              selectedId={stages.approverId}
              onChange={(id) => onUpdate({ approverId: id })}
              placeholder={t("office:selectApprover")}
            />
          </div>
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground w-20 shrink-0">{t("office:ship")}</span>
            <label className="flex items-center gap-1.5 cursor-pointer">
              <input
                type="checkbox"
                checked={stages.autoCommit}
                onChange={(e) => onUpdate({ autoCommit: e.target.checked })}
                className="cursor-pointer"
              />
              <span className="text-xs">{t("office:autoCommitAfterApproval")}</span>
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
