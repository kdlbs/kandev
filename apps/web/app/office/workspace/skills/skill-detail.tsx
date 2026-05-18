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
import type { Skill, SkillSourceType } from "@/lib/state/slices/office/types";
import { FileTree, type FileTreeNode } from "@/components/shared/file-tree";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";

interface SkillDetailProps {
  skill: Skill;
  onSave: (id: string, patch: Partial<Skill>) => void;
  onDelete: (id: string) => void;
}

const FALLBACK_SOURCE_LABELS: Record<SkillSourceType, string> = {
  inline: "Inline",
  local_path: "Local",
  git: "GitHub",
  skills_sh: "skills.sh",
  user_home: "User home",
  system: "Kandev",
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
  const meta = useAppStore((s) => s.office.meta);
  const metaSource = meta?.skillSourceTypes.find((s) => s.id === sourceType);
  return {
    label: metaSource?.label ?? FALLBACK_SOURCE_LABELS[sourceType] ?? sourceType,
    readOnly: metaSource?.readOnly ?? sourceType !== "inline",
    readOnlyReason: metaSource?.readOnlyReason,
  };
}

export function SkillDetail({ skill, onSave, onDelete }: SkillDetailProps) {
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [draft, setDraft] = useState(skill.content ?? "");
  const [isSaving, setIsSaving] = useState(false);
  const sourceMeta = useSkillSourceMeta(skill.sourceType);
  // System skills are kandev-owned; they refresh on backend start so
  // local edits would just get overwritten. Lock both edit and delete
  // for them regardless of what the source meta says.
  const readOnly = sourceMeta.readOnly || !!skill.isSystem;
  const agents = useAppStore((s) => s.office.agentProfiles);
  const usedByCount = useMemo(
    () => agents.filter((a) => a.desiredSkills?.includes(skill.id)).length,
    [agents, skill.id],
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
        readOnlyReason={
          skill.isSystem
            ? `Bundled with kandev${skill.systemVersion ? ` v${skill.systemVersion}` : ""}`
            : sourceMeta.readOnlyReason
        }
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
              {isSaving ? "Saving…" : "Save"}
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

const FALLBACK_READ_ONLY_REASONS: Partial<Record<SkillSourceType, string>> = {
  git: "GitHub-managed skills are read-only",
  skills_sh: "skills.sh-managed skills are read-only",
  local_path: "Local path skills are read-only",
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
  const { copied, copy } = useCopyToClipboard();

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
            <Badge variant="outline">Read only</Badge>
            {(readOnlyReason ?? FALLBACK_READ_ONLY_REASONS[skill.sourceType]) && (
              <span className="text-xs text-muted-foreground">
                {readOnlyReason ?? FALLBACK_READ_ONLY_REASONS[skill.sourceType]}
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
          <TooltipContent>{copied ? "Copied!" : "Copy slug"}</TooltipContent>
        </Tooltip>
        {onEdit && (
          <Button variant="ghost" size="sm" onClick={onEdit} className="cursor-pointer">
            Edit
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
            <TooltipContent>Remove skill</TooltipContent>
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
  const usedByLabel =
    usedByCount === 0
      ? "No agents attached"
      : `${usedByCount} agent${usedByCount !== 1 ? "s" : ""}`;

  const roles = skill.defaultForRoles ?? [];
  return (
    <div className="grid grid-cols-4 gap-4 text-sm">
      <MetadataItem label="SOURCE">
        <SourceValue skill={skill} />
      </MetadataItem>
      <MetadataItem label="KEY">
        <span className="font-mono">{skill.slug}</span>
      </MetadataItem>
      <MetadataItem label="MODE" hint="Whether this skill's content can be edited in Kandev">
        <span>{readOnly ? "Read only" : "Editable"}</span>
      </MetadataItem>
      <MetadataItem label="USED BY" hint="Agents that have this skill assigned to them">
        <span>{usedByLabel}</span>
      </MetadataItem>
      {skill.isSystem && roles.length > 0 && (
        <MetadataItem
          label="DEFAULT FOR"
          hint="New agents matching these roles get this skill auto-attached"
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
