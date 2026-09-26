import { IconCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";

type ExecutionStage = {
  id: string;
  type: "work" | "review" | "approval" | "ship";
  participants?: { type: "agent" | "user"; agent_id?: string; user_id?: string }[];
  approvals_needed?: number;
};

type ExecutionPolicy = {
  stages: ExecutionStage[];
};

type StageResponse = {
  participant_id: string;
  verdict: "approve" | "reject";
  comments: string;
  responded_at: string;
};

type ExecutionState = {
  current_stage_index: number;
  reentry_stage_index?: number;
  responses: Record<string, StageResponse>;
  status: "pending" | "approved" | "rejected";
};

// Catalog keys, not copy: this table is module scope, so a resolved `t()` here
// would freeze at the boot locale.
const STAGE_LABEL_KEYS: Record<ExecutionStage["type"], string> = {
  work: "task:stageImplement",
  review: "task:stageReview",
  approval: "task:stageApproval",
  ship: "task:stageShip",
};

type StagePillProps = {
  label: string;
  state: "completed" | "current" | "future";
};

function StagePill({ label, state }: StagePillProps) {
  if (state === "completed") {
    return (
      <Badge variant="secondary" className="flex items-center gap-1 px-2.5 py-1 text-xs">
        <IconCheck className="h-3 w-3" />
        {label}
      </Badge>
    );
  }

  if (state === "current") {
    return (
      <Badge className="flex items-center gap-1 px-2.5 py-1 text-xs cursor-default">{label}</Badge>
    );
  }

  return (
    <Badge
      variant="outline"
      className="flex items-center gap-1 px-2.5 py-1 text-xs text-muted-foreground"
    >
      {label}
    </Badge>
  );
}

type StageProgressBarProps = {
  executionPolicy?: string;
  executionState?: string;
};

function parsePolicy(raw: string): ExecutionPolicy | null {
  try {
    return JSON.parse(raw) as ExecutionPolicy;
  } catch {
    return null;
  }
}

function parseState(raw: string): ExecutionState | null {
  try {
    return JSON.parse(raw) as ExecutionState;
  } catch {
    return null;
  }
}

export function StageProgressBar({ executionPolicy, executionState }: StageProgressBarProps) {
  const { t } = useTranslation();
  if (!executionPolicy) return null;

  const policy = parsePolicy(executionPolicy);
  if (!policy?.stages?.length) return null;

  const state = executionState ? parseState(executionState) : null;
  const currentIndex = state?.current_stage_index ?? 0;

  return (
    <div className="flex items-center gap-1 mt-3 flex-wrap">
      {policy.stages.map((stage, index) => {
        let pillState: "completed" | "current" | "future" = "future";
        if (index < currentIndex) pillState = "completed";
        else if (index === currentIndex) pillState = "current";
        return (
          <div key={stage.id} className="flex items-center gap-1">
            <StagePill
              label={STAGE_LABEL_KEYS[stage.type] ? t(STAGE_LABEL_KEYS[stage.type]) : stage.type}
              state={pillState}
            />
            {index < policy.stages.length - 1 && (
              <span className="text-muted-foreground text-xs select-none">→</span>
            )}
          </div>
        );
      })}
    </div>
  );
}
