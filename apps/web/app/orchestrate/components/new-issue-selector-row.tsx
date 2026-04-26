"use client";

import {
  IconDotsVertical,
  IconEye,
  IconCircleCheck,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import type { AgentInstance, Project } from "@/lib/state/slices/orchestrate/types";
import type { IssueDraft } from "./new-issue-draft";
import { ParticipantRow } from "./new-issue-participant-row";

type Props = {
  draft: IssueDraft;
  onUpdate: (patch: Partial<IssueDraft>) => void;
};

function AgentPickerPopover({
  agents,
  selectedId,
  onSelect,
}: {
  agents: AgentInstance[];
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const selected = agents.find((a) => a.id === selectedId);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          {selected?.name ?? "Assignee"}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <button
          type="button"
          className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer"
          onClick={() => onSelect("")}
        >
          Unassigned
        </button>
        {agents.map((agent) => (
          <button
            key={agent.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer"
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
  const selected = projects.find((p) => p.id === selectedId);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="cursor-pointer h-7 text-xs">
          {selected?.name ?? "Project"}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-48 p-1" align="start">
        <button
          type="button"
          className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer"
          onClick={() => onSelect("")}
        >
          No project
        </button>
        {projects.map((project) => (
          <button
            key={project.id}
            type="button"
            className="w-full text-left px-2 py-1.5 text-sm rounded hover:bg-accent cursor-pointer flex items-center gap-2"
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

export function NewIssueSelectorRow({ draft, onUpdate }: Props) {
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const projects = useAppStore((s) => s.orchestrate.projects);

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
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
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
              <IconDotsVertical className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => onUpdate({ showReviewer: !draft.showReviewer })}
            >
              <IconEye className="h-4 w-4 mr-2" />
              {draft.showReviewer ? "Hide reviewer" : "Add reviewer"}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer"
              onClick={() => onUpdate({ showApprover: !draft.showApprover })}
            >
              <IconCircleCheck className="h-4 w-4 mr-2" />
              {draft.showApprover ? "Hide approver" : "Add approver"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {draft.showReviewer && (
        <ParticipantRow
          label="Reviewer"
          agents={agents}
          selectedIds={draft.reviewerIds}
          onSelect={(ids) => onUpdate({ reviewerIds: ids })}
          onHide={() => onUpdate({ showReviewer: false, reviewerIds: [] })}
        />
      )}

      {draft.showApprover && (
        <ParticipantRow
          label="Approver"
          agents={agents}
          selectedIds={draft.approverIds}
          onSelect={(ids) => onUpdate({ approverIds: ids })}
          onHide={() => onUpdate({ showApprover: false, approverIds: [] })}
        />
      )}
    </div>
  );
}
