"use client";

import { useMemo, useState } from "react";
import {
  IconBrandGithub,
  IconFolder,
  IconCode,
  IconEye,
  IconTrash,
  IconBoxMultiple,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
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
        {readOnly && <Badge variant="outline">Read only</Badge>}
        {onEdit && (
          <Button variant="ghost" size="sm" onClick={onEdit} className="cursor-pointer">
            Edit
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          onClick={onDelete}
          className="text-destructive cursor-pointer"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function SkillMetadataRow({ skill, readOnly }: { skill: Skill; readOnly: boolean }) {
  return (
    <div className="grid grid-cols-4 gap-4 text-sm">
      <MetadataItem label="SOURCE">
        <div className="flex items-center gap-1.5">
          <SourceIcon sourceType={skill.sourceType} />
          <span>{SOURCE_LABELS[skill.sourceType]}</span>
        </div>
      </MetadataItem>
      <MetadataItem label="KEY">
        <span className="font-mono">{skill.slug}</span>
      </MetadataItem>
      <MetadataItem label="MODE">
        <span>{readOnly ? "Read only" : "Editable"}</span>
      </MetadataItem>
      <MetadataItem label="USED BY">
        <span>--</span>
      </MetadataItem>
    </div>
  );
}

function MetadataItem({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
        {label}
      </div>
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
