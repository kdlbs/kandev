"use client";

import { useMemo, useState } from "react";
import {
  IconBrandGithub,
  IconFolder,
  IconCode,
  IconEye,
  IconTrash,
  IconBoxMultiple,
  IconCopy,
  IconCheck,
  IconExternalLink,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { Skill, SkillSourceType } from "@/lib/state/slices/orchestrate/types";
import { FileTree, type FileTreeNode } from "@/components/shared/file-tree";
import { SkillContent } from "./skill-content";

type ContentMode = "view" | "code" | "edit";

interface SkillDetailProps {
  skill: Skill;
  onSave: (id: string, patch: Partial<Skill>) => void;
  onDelete: (id: string) => void;
}

const SOURCE_LABELS: Record<SkillSourceType, string> = {
  inline: "Inline",
  local_path: "Local",
  git: "GitHub",
  skills_sh: "skills.sh",
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

function isReadOnly(sourceType: SkillSourceType): boolean {
  return sourceType !== "inline";
}

export function SkillDetail({ skill, onSave, onDelete }: SkillDetailProps) {
  const [contentMode, setContentMode] = useState<ContentMode>("view");
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const readOnly = isReadOnly(skill.sourceType);

  const fileTree = useMemo(() => buildFileTree(skill.fileInventory), [skill.fileInventory]);
  const hasFiles = fileTree.length > 0;

  const activeFilePath = selectedFile ?? "SKILL.md";

  const handleSave = (content: string) => {
    onSave(skill.id, { content });
    setContentMode("view");
  };

  const handleEdit = () => setContentMode("edit");
  const handleCancelEdit = () => setContentMode("view");

  return (
    <div className="space-y-6">
      <SkillDetailHeader
        skill={skill}
        readOnly={readOnly}
        onEdit={!readOnly ? handleEdit : undefined}
        onDelete={() => onDelete(skill.id)}
      />
      <Separator />
      <SkillMetadataRow skill={skill} readOnly={readOnly} />

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
          {contentMode !== "edit" && (
            <div className="flex gap-1">
              <Button
                variant={contentMode === "view" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => setContentMode("view")}
                className="cursor-pointer"
              >
                <IconEye className="h-4 w-4 mr-1" />
                View
              </Button>
              <Button
                variant={contentMode === "code" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => setContentMode("code")}
                className="cursor-pointer"
              >
                <IconCode className="h-4 w-4 mr-1" />
                Code
              </Button>
            </div>
          )}
        </div>
        <SkillContent
          content={skill.content ?? ""}
          mode={contentMode}
          onSave={handleSave}
          onCancel={handleCancelEdit}
        />
      </div>
    </div>
  );
}

const READ_ONLY_REASONS: Partial<Record<SkillSourceType, string>> = {
  git: "GitHub-managed skills are read-only",
  skills_sh: "skills.sh-managed skills are read-only",
  local_path: "Local path skills are read-only",
};

function SkillDetailHeader({
  skill,
  readOnly,
  onEdit,
  onDelete,
}: {
  skill: Skill;
  readOnly: boolean;
  onEdit?: () => void;
  onDelete: () => void;
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
            {READ_ONLY_REASONS[skill.sourceType] && (
              <span className="text-xs text-muted-foreground">
                {READ_ONLY_REASONS[skill.sourceType]}
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
      </div>
    </div>
  );
}

function SourceValue({ skill }: { skill: Skill }) {
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
          {SOURCE_LABELS[skill.sourceType]}
          <IconExternalLink className="h-3 w-3 inline ml-1" />
        </a>
      ) : (
        <span>{SOURCE_LABELS[skill.sourceType]}</span>
      )}
    </div>
  );
}

function SkillMetadataRow({ skill, readOnly }: { skill: Skill; readOnly: boolean }) {
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
        <span>--</span>
      </MetadataItem>
    </div>
  );
}

function MetadataItem({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
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
