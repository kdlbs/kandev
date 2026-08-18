"use client";

import { IconDotsVertical, IconEye, IconCircleCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import {
  selectOfficeAgentProfiles,
  selectOfficeProjects,
} from "@/lib/state/slices/office/selectors";
import type { AgentProfile, Project } from "@/lib/state/slices/office/types";
import type { IssueDraft } from "./new-task-draft";
import { ParticipantRow } from "./new-task-participant-row";
import { Trans, useTranslation } from "react-i18next";

type Props = {
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

function AgentPickerPopover({
  agents,
  selectedId,
  onSelect,
}: {
  agents: AgentProfile[];
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  const selected = agents.find((a) => a.id === selectedId);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          {selected?.name ?? t("office:assignee")}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <button
          type="button"
          className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer"
          onClick={() => onSelect("")}
        >
          {t("office:unassigned")}
        </button>
        {agents.map((agent) => (
          <button
            key={agent.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer"
            onClick={() => onSelect(agent.id)}
          >
            {agent.name}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

function ProjectPickerPopover({
  projects,
  selectedId,
  onSelect,
}: {
  projects: Project[];
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  const selected = projects.find((p) => p.id === selectedId);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          {selected?.name ?? t("office:project")}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <button
          type="button"
          className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer"
          onClick={() => onSelect("")}
        >
          {t("office:noProject")}
        </button>
        {projects.map((project) => (
          <button
            key={project.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-muted cursor-pointer flex items-center gap-2"
            onClick={() => onSelect(project.id)}
          >
            {project.color && (
              <span
                className="h-2.5 w-2.5 rounded-sm shrink-0"
                style={{ backgroundColor: project.color }}
              />
            )}
            {project.name}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}

export function NewTaskSelectorRow({ draft, onUpdate }: Props) {
  const { t } = useTranslation();
  const agents = useAppStore(selectOfficeAgentProfiles);
  const projects = useAppStore(selectOfficeProjects);

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {/*
          One key, not four: "For <agent> in <project>" is a sentence whose word
          order changes per language, and two separately-keyed fragments would
          freeze the English order. The pickers are element children so a
          translator can move them; `<Trans>` renders into a fragment, so they
          stay direct children of this flex row.
        */}
        <Trans i18nKey="office:forAgentInProject">
          <span>For</span>
          <AgentPickerPopover
            agents={agents}
            selectedId={draft.assigneeId}
            onSelect={(id) => onUpdate({ assigneeId: id })}
          />
          <span>in</span>
          <ProjectPickerPopover
            projects={projects}
            selectedId={draft.projectId}
            onSelect={(id) => onUpdate({ projectId: id })}
          />
        </Trans>
        <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
                  <IconDotsVertical className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent>{t("office:moreOptions")}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="start">
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => onUpdate({ showReviewer: !draft.showReviewer })}
            >
              <IconEye className="h-4 w-4 mr-2" />
              {draft.showReviewer ? t("office:hideReviewer") : t("office:addReviewer")}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => onUpdate({ showApprover: !draft.showApprover })}
            >
              <IconCircleCheck className="h-4 w-4 mr-2" />
              {draft.showApprover ? t("office:hideApprover") : t("office:addApprover")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {draft.showReviewer && (
        <ParticipantRow
          kind="reviewer"
          agents={agents}
          selectedIds={draft.reviewerIds}
          onSelect={(ids) => onUpdate({ reviewerIds: ids })}
          onHide={() => onUpdate({ showReviewer: false, reviewerIds: [] })}
        />
      )}

      {draft.showApprover && (
        <ParticipantRow
          kind="approver"
          agents={agents}
          selectedIds={draft.approverIds}
          onSelect={(ids) => onUpdate({ approverIds: ids })}
          onHide={() => onUpdate({ showApprover: false, approverIds: [] })}
        />
      )}
    </div>
  );
}
