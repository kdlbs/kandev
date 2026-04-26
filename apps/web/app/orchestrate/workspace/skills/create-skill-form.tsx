"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import type { SkillSourceType } from "@/lib/state/slices/orchestrate/types";

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
            <SelectItem value="inline">Inline (edit in browser)</SelectItem>
            <SelectItem value="local_path">Local Path</SelectItem>
            <SelectItem value="git">Git Repository</SelectItem>
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
