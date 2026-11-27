"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import {
  MAX_WATCHED_PROJECTS,
  switchOffProjects,
  type ProjectChoice,
  type ProjectsDraft,
} from "@/lib/coordinators/control-draft";
import type { ProjectEntryKind } from "@/lib/api/domains/coordinator-api";
import type {
  ProjectSetChoice,
  ProjectsStatus,
} from "@/hooks/domains/coordinator/use-workspace-projects";

export type ProjectsFieldsProps = {
  projects: ProjectsDraft;
  sets: ProjectSetChoice[];
  loose: ProjectChoice[];
  status: ProjectsStatus;
  onRetry: () => void;
  canManage: boolean;
  onChange: (next: ProjectsDraft) => void;
  errorMessage?: string | null;
};

type ProjectRowProps = {
  choice: ProjectChoice;
  detail?: string;
  inScope: boolean;
  disabled: boolean;
  onToggle: () => void;
};

function ProjectRow({ choice, detail, inScope, disabled, onToggle }: ProjectRowProps) {
  const { t } = useTranslation();
  return (
    <li
      className="flex flex-col gap-2 py-2 sm:flex-row sm:items-center sm:justify-between"
      data-testid={`watches-project-${choice.kind}-${choice.id}`}
    >
      <span className="flex flex-wrap items-center gap-2 text-sm">
        {choice.name}
        {detail && <span className="text-xs text-muted-foreground">{detail}</span>}
        <span className="text-xs text-muted-foreground">
          {inScope ? t("coordinator:watchesInScope") : t("coordinator:watchesOutOfScope")}
        </span>
      </span>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer max-sm:min-h-11 max-sm:w-full"
        disabled={disabled}
        onClick={onToggle}
        data-testid={`watches-project-toggle-${choice.id}`}
      >
        {inScope ? t("coordinator:watchesProjectTakeOut") : t("coordinator:watchesProjectPutIn")}
      </Button>
    </li>
  );
}

function entryIds(projects: ProjectsDraft, kind: ProjectEntryKind): Set<string> {
  return new Set(projects.entries.filter((entry) => entry.kind === kind).map((entry) => entry.id));
}

type Toggle = { disabled: boolean; checked: boolean; onChange: (value: boolean) => void };

function SwitchRow({ id, label, toggle }: { id: string; label: string; toggle: Toggle }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <label htmlFor={id} className="cursor-pointer text-sm">
        {label}
      </label>
      <Switch
        id={id}
        data-testid={id}
        checked={toggle.checked}
        disabled={toggle.disabled}
        onCheckedChange={toggle.onChange}
      />
    </div>
  );
}

function ProjectsNotices({
  status,
  noChoices,
  onRetry,
}: {
  status: ProjectsStatus;
  noChoices: boolean;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {noChoices && (
        <p className="text-sm text-muted-foreground" data-testid="watches-no-projects">
          {t("coordinator:watchesNoProjectsToChoose")}
        </p>
      )}
      {status === "error" && (
        <div className="flex items-center gap-2 text-sm" data-testid="watches-projects-failed">
          {t("coordinator:watchesProjectsFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={onRetry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      )}
    </>
  );
}

type ProjectListProps = {
  projects: ProjectsDraft;
  sets: ProjectSetChoice[];
  loose: ProjectChoice[];
  canManage: boolean;
  onToggle: (choice: ProjectChoice, inScope: boolean) => void;
};

function ProjectList({ projects, sets, loose, canManage, onToggle }: ProjectListProps) {
  const { t } = useTranslation();
  const selectedSets = entryIds(projects, "repository_set");
  const selectedRepos = entryIds(projects, "repository");
  const atCap = projects.entries.length >= MAX_WATCHED_PROJECTS;
  const nothing = projects.entries.length === 0 && !projects.includeNoRepository;
  const row = (choice: ProjectChoice, selected: Set<string>, detail?: string) => (
    <ProjectRow
      key={`${choice.kind}-${choice.id}`}
      choice={choice}
      detail={detail}
      inScope={selected.has(choice.id)}
      disabled={!canManage || (!selected.has(choice.id) && atCap)}
      onToggle={() => onToggle(choice, selected.has(choice.id))}
    />
  );
  return (
    <>
      <ul className="divide-y" data-testid="watches-project-list">
        {sets.map((set) =>
          row(
            set,
            selectedSets,
            t("coordinator:watchesProjectSetCount", { count: set.repositoryCount }),
          ),
        )}
        {loose.map((repository) => row(repository, selectedRepos))}
      </ul>
      {atCap && (
        <p className="text-xs text-muted-foreground">{t("coordinator:watchesProjectsAtMost")}</p>
      )}
      {nothing && (
        <p className="text-xs text-muted-foreground" data-testid="watches-keep-one-project">
          {t("coordinator:watchesKeepOneProject")}
        </p>
      )}
    </>
  );
}

/** The Projects part of the Watches section: scope switch, no-repository toggle, and the project rows. */
export function ProjectsFields({
  projects,
  sets,
  loose,
  status,
  onRetry,
  canManage,
  onChange,
  errorMessage,
}: ProjectsFieldsProps) {
  const { t } = useTranslation();
  const choices = [...sets, ...loose];
  const noChoices = status === "ready" && choices.length === 0;
  const selected = projects.scope === "selected";
  const cannotSwitchOff =
    !selected &&
    (status !== "ready" || noChoices) &&
    projects.entries.length === 0 &&
    !projects.includeNoRepository;

  const toggleAll = (watchAll: boolean) => {
    if (watchAll) return onChange({ ...projects, scope: "all" });
    const next = switchOffProjects(projects, choices);
    if (next) onChange(next);
  };
  const toggleEntry = (choice: ProjectChoice, inScope: boolean) => {
    const entries = inScope
      ? projects.entries.filter((e) => !(e.kind === choice.kind && e.id === choice.id))
      : [...projects.entries, { kind: choice.kind, id: choice.id }];
    onChange({ ...projects, scope: "selected", entries });
  };

  return (
    <div className="space-y-3" data-testid="watches-projects">
      <h4 className="text-sm font-medium">{t("coordinator:watchesProjects")}</h4>
      <SwitchRow
        id="watches-projects-all"
        label={t("coordinator:watchesProjectsAll")}
        toggle={{
          checked: !selected,
          disabled: !canManage || cannotSwitchOff,
          onChange: toggleAll,
        }}
      />
      {errorMessage && (
        <p role="alert" className="text-sm text-destructive" data-testid="watches-projects-error">
          {errorMessage}
        </p>
      )}
      {!selected && (
        <p className="text-xs text-muted-foreground">{t("coordinator:watchesProjectsAllHelp")}</p>
      )}
      {selected && (
        <>
          <SwitchRow
            id="watches-projects-no-repo"
            label={t("coordinator:watchesIncludeNoRepository")}
            toggle={{
              checked: projects.includeNoRepository,
              disabled: !canManage,
              onChange: (includeNoRepository) => onChange({ ...projects, includeNoRepository }),
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t("coordinator:watchesNoRepositoryNote")}
          </p>
        </>
      )}
      <ProjectsNotices status={status} noChoices={noChoices} onRetry={onRetry} />
      {selected && choices.length > 0 && (
        <ProjectList
          projects={projects}
          sets={sets}
          loose={loose}
          canManage={canManage}
          onToggle={toggleEntry}
        />
      )}
    </div>
  );
}
