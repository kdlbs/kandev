"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import type { SkillSourceType } from "@/lib/state/slices/office/types";

const FALLBACK_SOURCE_TYPES = [
  { id: "inline", label: "Inline (edit in browser)" },
  { id: "local_path", label: "Local Path" },
  { id: "git", label: "Git Repository" },
];

type CreateSkillFormProps = {
  onCreate: (data: {
    name: string;
    description: string;
    sourceType: SkillSourceType;
    sourceLocator: string;
    content: string;
  }) => void;
  onCancel: () => void;
};

export function CreateSkillForm({ onCreate, onCancel }: CreateSkillFormProps) {
  const meta = useAppStore((s) => s.office.meta);
  // Only show creatable (non-read-only) source types, plus exclude skills_sh from creation
  const sourceTypes = meta
    ? meta.skillSourceTypes.filter((s) => !s.readOnly).map((s) => ({ id: s.id, label: s.label }))
    : FALLBACK_SOURCE_TYPES;

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [sourceType, setSourceType] = useState<SkillSourceType>("inline");
  const [sourceLocator, setSourceLocator] = useState("");
  const [content, setContent] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onCreate({ name: name.trim(), description, sourceType, sourceLocator, content });
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <h2 className="text-lg font-semibold">New Skill</h2>

      <div className="space-y-1">
        <label className="text-sm font-medium">Name</label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. code-review"
          autoFocus
        />
      </div>

      <div className="space-y-1">
        <label className="text-sm font-medium">Description</label>
        <Input
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="One-line summary"
        />
      </div>

      <div className="space-y-1">
        <label className="text-sm font-medium">Source Type</label>
        <Select value={sourceType} onValueChange={(v) => setSourceType(v as SkillSourceType)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {sourceTypes.map((st) => (
              <SelectItem key={st.id} value={st.id}>
                {st.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {sourceType === "local_path" && (
        <div className="space-y-1">
          <label className="text-sm font-medium">Directory Path</label>
          <Input
            value={sourceLocator}
            onChange={(e) => setSourceLocator(e.target.value)}
            placeholder="/path/to/skill-directory"
            className="font-mono"
          />
        </div>
      )}

      {sourceType === "git" && (
        <div className="space-y-1">
          <label className="text-sm font-medium">Repository URL</label>
          <Input
            value={sourceLocator}
            onChange={(e) => setSourceLocator(e.target.value)}
            placeholder="https://github.com/org/repo"
            className="font-mono"
          />
        </div>
      )}

      {sourceType === "inline" && (
        <div className="space-y-1">
          <label className="text-sm font-medium">SKILL.md Content</label>
          <Textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            className="min-h-[200px] font-mono text-sm"
            placeholder="# Skill Name&#10;&#10;Instructions for the agent..."
          />
        </div>
      )}

      <div className="flex justify-end gap-2 pt-4 border-t border-border">
        <Button type="button" variant="ghost" onClick={onCancel} className="cursor-pointer">
          Cancel
        </Button>
        <Button type="submit" disabled={!name.trim()} className="cursor-pointer">
          Create Skill
        </Button>
      </div>
    </form>
  );
}
