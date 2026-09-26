"use client";

import { useEffect, useMemo, useState } from "react";
import {
  IconBrandGithub,
  IconFolder,
  IconCode,
  IconTrash,
  IconBoxMultiple,
  IconCopy,
  IconCheck,
  IconDeviceFloppy,
  IconExternalLink,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import type { Skill, SkillSourceType } from "@/lib/state/slices/office/types";
import { FileTree, type FileTreeNode } from "@/components/shared/file-tree";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import { useTranslation } from "react-i18next";

interface SkillDetailProps {
  skill: Skill;
  onSave: (id: string, patch: Partial<Skill>) => void;
  onDelete: (id: string) => void;
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire `SkillSourceType` values. `skills.sh` and `Kandev`
// are product names and resolve to themselves.
const FALLBACK_SOURCE_LABEL_KEYS: Record<SkillSourceType, string> = {
  inline: "office:skillSourceInline",
  local_path: "office:skillSourceLocal",
  git: "office:skillSourceGithub",
  skills_sh: "office:skillSourceSkillsSh",
  user_home: "office:skillSourceUserHome",
  system: "office:skillSourceKandev",
};

function SourceIcon({ sourceType }: { sourceType: SkillSourceType }) {
  switch (sourceType) {
    case "git":
    case "skills_sh":
      return <IconBrandGithub className="h-4 w-4" />;
    case "local_path":
      return <IconFolder className="h-4 w-4" />;
    default:
      return <IconCode className="h-4 w-4" />;
  }
}

function useSkillSourceMeta(sourceType: SkillSourceType) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const metaSource = meta?.skillSourceTypes.find((s) => s.id === sourceType);
  const fallbackKey = FALLBACK_SOURCE_LABEL_KEYS[sourceType];
  return {
    // `?? sourceType` keeps an unknown wire value visible rather than blank.
    label: metaSource?.label ?? (fallbackKey ? t(fallbackKey) : sourceType),
    readOnly: metaSource?.readOnly ?? sourceType !== "inline",
    readOnlyReason: metaSource?.readOnlyReason,
  };
}

export function SkillDetail({ skill, onSave, onDelete }: SkillDetailProps) {
  const { t } = useTranslation();
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [draft, setDraft] = useState(skill.content ?? "");
  const [isSaving, setIsSaving] = useState(false);
  const sourceMeta = useSkillSourceMeta(skill.sourceType);
  const bundledReason = skill.systemVersion
    ? t("office:bundledWithKandevVersion", { version: skill.systemVersion })
    : t("office:bundledWithKandev");
  // System skills are kandev-owned; they refresh on backend start so
  // local edits would just get overwritten. Lock both edit and delete
  // for them regardless of what the source meta says.
  const readOnly = sourceMeta.readOnly || !!skill.isSystem;
  const agents = useAppStore(selectOfficeAgentProfiles);
  const usedByCount = useMemo(
    () =>
      agents.filter((a) => a.skillIds?.includes(skill.id) || a.desiredSkills?.includes(skill.slug))
        .length,
    [agents, skill.id, skill.slug],
  );

  const fileTree = useMemo(() => buildFileTree(skill.fileInventory), [skill.fileInventory]);
  const hasFiles = fileTree.length > 0;

  const activeFilePath = selectedFile ?? "SKILL.md";
  const isDirty = !readOnly && draft !== (skill.content ?? "");

  // Reset the draft when the user navigates to a different skill (or
  // the skill row gets re-synced and the canonical content shifts).
  useEffect(() => {
    setDraft(skill.content ?? "");
  }, [skill.id, skill.content]);

  const handleSave = async () => {
    setIsSaving(true);
    try {
      await onSave(skill.id, { content: draft });
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <SkillDetailHeader
        skill={skill}
        readOnly={readOnly}
        readOnlyReason={skill.isSystem ? bundledReason : sourceMeta.readOnlyReason}
        onDelete={!skill.isSystem ? () => onDelete(skill.id) : undefined}
      />
      <Separator />
      <SkillMetadataRow skill={skill} readOnly={readOnly} usedByCount={usedByCount} />

      {hasFiles && (
        <div className="border border-border rounded-lg max-h-[200px] overflow-y-auto">
          <FileTree
            nodes={fileTree}
            selectedPath={activeFilePath}
            onSelectPath={setSelectedFile}
            defaultExpanded
          />
        </div>
      )}

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-sm font-mono text-muted-foreground">{activeFilePath}</span>
          {!readOnly && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleSave}
              disabled={!isDirty || isSaving}
              className="cursor-pointer"
            >
              <IconDeviceFloppy className="h-4 w-4 mr-1" />
              {isSaving ? t("office:savingEllipsis") : t("common:save")}
            </Button>
          )}
        </div>
        <div
          className="border border-border rounded-lg overflow-hidden"
          data-testid="skill-content-editor"
          data-readonly={readOnly ? "true" : "false"}
        >
          {readOnly && <span data-testid="skill-content-readonly" hidden />}
          <ScriptEditor
            value={draft}
            onChange={setDraft}
            language="markdown"
            height="520px"
            readOnly={readOnly}
          />
        </div>
      </div>
    </div>
  );
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire `SkillSourceType` values.
const FALLBACK_READ_ONLY_REASON_KEYS: Partial<Record<SkillSourceType, string>> = {
  git: "office:readOnlyReasonGit",
  skills_sh: "office:readOnlyReasonSkillsSh",
  local_path: "office:readOnlyReasonLocalPath",
};

function SkillDetailHeader({
  skill,
  readOnly,
  readOnlyReason,
  onEdit,
  onDelete,
}: {
  skill: Skill;
  readOnly: boolean;
  readOnlyReason?: string;
  onEdit?: () => void;
  // Optional: system skills hide the delete affordance since they
  // refresh from the kandev binary on every backend start.
  onDelete?: () => void;
}) {
  const { t } = useTranslation();
  const { copied, copy } = useCopyToClipboard();
  const fallbackReadOnlyReasonKey = FALLBACK_READ_ONLY_REASON_KEYS[skill.sourceType];

  return (
    <div className="flex items-start justify-between">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted">
          <IconBoxMultiple className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="space-y-0.5">
          <h2 className="text-lg font-semibold">{skill.name}</h2>
          {skill.description && (
            <p className="text-sm text-muted-foreground">{skill.description}</p>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        {readOnly && (
          <>
            <Badge variant="outline">{t("office:readOnly")}</Badge>
            {(readOnlyReason ?? fallbackReadOnlyReasonKey) && (
              <span className="text-xs text-muted-foreground">
                {readOnlyReason ??
                  (fallbackReadOnlyReasonKey ? t(fallbackReadOnlyReasonKey) : null)}
              </span>
            )}
          </>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => copy(skill.slug)}
              className="h-7 w-7 p-0 cursor-pointer"
            >
              {copied ? (
                <IconCheck className="h-4 w-4 text-green-500" />
              ) : (
                <IconCopy className="h-4 w-4" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{copied ? t("office:copied") : t("office:copySlug")}</TooltipContent>
        </Tooltip>
        {onEdit && (
          <Button variant="ghost" size="sm" onClick={onEdit} className="cursor-pointer">
            {t("common:edit")}
          </Button>
        )}
        {onDelete && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                onClick={onDelete}
                className="h-7 w-7 p-0 text-destructive cursor-pointer"
              >
                <IconTrash className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("office:removeSkill")}</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}

function SourceValue({ skill }: { skill: Skill }) {
  const sourceMeta = useSkillSourceMeta(skill.sourceType);
  const isLink = skill.sourceLocator?.startsWith("http");
  return (
    <div className="flex items-center gap-1.5">
      <SourceIcon sourceType={skill.sourceType} />
      {isLink ? (
        <a
          href={skill.sourceLocator}
          target="_blank"
          rel="noopener noreferrer"
          className="hover:underline cursor-pointer"
        >
          {sourceMeta.label}
          <IconExternalLink className="h-3 w-3 inline ml-1" />
        </a>
      ) : (
        <span>{sourceMeta.label}</span>
      )}
    </div>
  );
}

function SkillMetadataRow({
  skill,
  readOnly,
  usedByCount,
}: {
  skill: Skill;
  readOnly: boolean;
  usedByCount: number;
}) {
  const { t } = useTranslation();
  // `count` + `_one`/`_other`, never a hand-written "s".
  const usedByLabel =
    usedByCount === 0
      ? t("office:noAgentsAttached")
      : t("office:agentCount", { count: usedByCount });

  const roles = skill.defaultForRoles ?? [];
  return (
    <div className="grid grid-cols-4 gap-4 text-sm">
      <MetadataItem label={t("office:metaSource")}>
        <SourceValue skill={skill} />
      </MetadataItem>
      <MetadataItem label={t("office:metaKey")}>
        <span className="font-mono">{skill.slug}</span>
      </MetadataItem>
      <MetadataItem label={t("office:metaMode")} hint={t("office:whetherThisSkillSContentCan")}>
        <span>{readOnly ? t("office:readOnly") : t("office:editable")}</span>
      </MetadataItem>
      <MetadataItem label={t("office:usedBy")} hint={t("office:agentsThatHaveThisSkillAssigned")}>
        <span>{usedByLabel}</span>
      </MetadataItem>
      {skill.isSystem && roles.length > 0 && (
        <MetadataItem
          label={t("office:defaultFor")}
          hint={t("office:newAgentsMatchingTheseRolesGet")}
        >
          <span>{roles.join(", ")}</span>
        </MetadataItem>
      )}
    </div>
  );
}

function MetadataItem({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  const labelEl = (
    <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
      {label}
    </div>
  );

  return (
    <div className="space-y-1">
      {hint ? (
        <Tooltip>
          <TooltipTrigger asChild>{labelEl}</TooltipTrigger>
          <TooltipContent>{hint}</TooltipContent>
        </Tooltip>
      ) : (
        labelEl
      )}
      <div>{children}</div>
    </div>
  );
}

/** Build a FileTreeNode[] from a flat list of file paths */
function buildFileTree(paths?: string[]): FileTreeNode[] {
  if (!paths || paths.length <= 1) return [];

  const root: FileTreeNode[] = [];
  for (const filePath of paths) {
    const parts = filePath.split("/");
    let current = root;
    let accumulated = "";
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      accumulated = accumulated ? `${accumulated}/${part}` : part;
      const isLast = i === parts.length - 1;
      let existing = current.find((n) => n.name === part);
      if (!existing) {
        existing = {
          name: part,
          path: accumulated,
          isDir: !isLast,
          children: [],
        };
        current.push(existing);
      }
      current = existing.children;
    }
  }
  return root;
}
