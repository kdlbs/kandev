"use client";

import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

/**
 * `kind` is a discriminant, not copy. The row's three strings — the row label,
 * the empty-state button and the remove tooltip — are each a whole sentence in
 * the catalog, so a translator can inflect "reviewer"/"approver" for the case
 * each one needs. Interpolating an already-translated noun into a translated
 * frame ("Add {{label}}") cannot do that: the frame cannot reorder and the noun
 * cannot agree.
 */
type ParticipantKind = "reviewer" | "approver";

const PARTICIPANT_COPY: Record<ParticipantKind, { label: string; add: string; remove: string }> = {
  reviewer: {
    label: "office:reviewer",
    add: "office:addReviewers",
    remove: "office:removeReviewer",
  },
  approver: {
    label: "office:approver",
    add: "office:addApprovers",
    remove: "office:removeApprover",
  },
};

type ParticipantRowProps = {
  kind: ParticipantKind;
  agents: AgentProfile[];
  selectedIds: string[];
  onSelect: (ids: string[]) => void;
  onHide: () => void;
};

export function ParticipantRow({
  kind,
  agents,
  selectedIds,
  onSelect,
  onHide,
}: ParticipantRowProps) {
  const { t } = useTranslation();
  const copy = PARTICIPANT_COPY[kind];
  const toggle = (id: string) => {
    onSelect(selectedIds.includes(id) ? selectedIds.filter((x) => x !== id) : [...selectedIds, id]);
  };

  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      <span className="w-16 shrink-0">{t(copy.label)}</span>
      <Popover>
        <PopoverTrigger asChild>
          <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
            {selectedIds.length > 0
              ? t("office:participantsSelected", { count: selectedIds.length })
              : t(copy.add)}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-48 p-1" align="start">
          {agents.map((agent) => (
            <button
              key={agent.id}
              type="button"
              className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer flex items-center gap-2"
              onClick={() => toggle(agent.id)}
            >
              <span
                className={`h-3 w-3 rounded-sm border ${selectedIds.includes(agent.id) ? "bg-primary border-primary" : "border-muted-foreground"}`}
              />
              {agent.name}
            </button>
          ))}
        </PopoverContent>
      </Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" className="h-6 w-6 cursor-pointer" onClick={onHide}>
            <IconX className="h-3 w-3" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t(copy.remove)}</TooltipContent>
      </Tooltip>
    </div>
  );
}
