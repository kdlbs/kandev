"use client";

import { useState } from "react";
import { IconTrash, IconDeviceFloppy, IconPencil } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Separator } from "@kandev/ui/separator";
import type { Skill } from "@/lib/state/slices/orchestrate/types";

type SkillEditorProps = {
  skill: Skill;
  onSave: (id: string, patch: Partial<Skill>) => void;
  onDelete: (id: string) => void;
};

export function SkillEditor({ skill, onSave, onDelete }: SkillEditorProps) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(skill.name);
  const [description, setDescription] = useState(skill.description ?? "");
  const [content, setContent] = useState(skill.content ?? "");

  const handleSave = () => {
    onSave(skill.id, { name, description, content });
    setEditing(false);
  };

  const handleCancel = () => {
    setName(skill.name);
    setDescription(skill.description ?? "");
    setContent(skill.content ?? "");
    setEditing(false);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="space-y-1">
          {editing ? (
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="text-lg font-semibold h-9"
            />
          ) : (
            <h2 className="text-lg font-semibold">{skill.name}</h2>
          )}
          <div className="flex items-center gap-2">
            <Badge variant="outline">{skill.sourceType}</Badge>
            <span className="text-xs text-muted-foreground font-mono">{skill.slug}</span>
          </div>
        </div>
        <div className="flex gap-1 shrink-0">
          {editing ? (
            <>
              <Button variant="ghost" size="sm" onClick={handleCancel} className="cursor-pointer">
                Cancel
              </Button>
              <Button size="sm" onClick={handleSave} className="cursor-pointer">
                <IconDeviceFloppy className="h-4 w-4 mr-1" /> Save
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setEditing(true)}
                className="cursor-pointer"
              >
                <IconPencil className="h-4 w-4 mr-1" /> Edit
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onDelete(skill.id)}
                className="text-destructive cursor-pointer"
              >
                <IconTrash className="h-4 w-4" />
              </Button>
            </>
          )}
        </div>
      </div>

      <Separator />

      {/* Description */}
      <div className="space-y-1">
        <label className="text-sm font-medium text-muted-foreground">Description</label>
        {editing ? (
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="One-line summary"
          />
        ) : (
          <p className="text-sm">{skill.description || "No description"}</p>
        )}
      </div>

      {/* Source info */}
      {skill.sourceType === "local_path" && skill.sourceLocator && (
        <div className="space-y-1">
          <label className="text-sm font-medium text-muted-foreground">Path</label>
          <p className="text-sm font-mono text-muted-foreground">{skill.sourceLocator}</p>
        </div>
      )}
      {skill.sourceType === "git" && skill.sourceLocator && (
        <div className="space-y-1">
          <label className="text-sm font-medium text-muted-foreground">Repository</label>
          <p className="text-sm font-mono text-muted-foreground">{skill.sourceLocator}</p>
        </div>
      )}

      {/* Content (SKILL.md) */}
      {skill.sourceType === "inline" && (
        <div className="space-y-1">
          <label className="text-sm font-medium text-muted-foreground">SKILL.md</label>
          {editing ? (
            <Textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              className="min-h-[300px] font-mono text-sm"
              placeholder="# Skill instructions..."
            />
          ) : (
            <pre className="text-sm font-mono whitespace-pre-wrap bg-muted/50 rounded-md p-4 max-h-[500px] overflow-y-auto">
              {skill.content || "No content"}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
