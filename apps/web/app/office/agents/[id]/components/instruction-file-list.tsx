"use client";

import { useState } from "react";
import { IconPlus, IconFile, IconFileText } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { cn } from "@kandev/ui/lib/utils";
import type { InstructionFile } from "./agent-instructions-tab";

type InstructionFileListProps = {
  files: InstructionFile[];
  selectedFile: string | null;
  onSelect: (filename: string) => void;
  onAdd: (filename: string) => void;
};

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

export function InstructionFileList({
  files,
  selectedFile,
  onSelect,
  onAdd,
}: InstructionFileListProps) {
  const [showInput, setShowInput] = useState(false);
  const [newFilename, setNewFilename] = useState("");

  const handleSubmit = () => {
    const name = newFilename.trim();
    if (!name) return;
    const filename = name.endsWith(".md") ? name : `${name}.md`;
    if (files.some((f) => f.filename === filename)) return;
    onAdd(filename);
    setNewFilename("");
    setShowInput(false);
  };

  return (
    <div className="w-[250px] shrink-0 border border-border rounded-lg">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Files
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 cursor-pointer"
          onClick={() => setShowInput(!showInput)}
        >
          <IconPlus className="h-4 w-4" />
        </Button>
      </div>
      {showInput && (
        <div className="px-3 py-2 border-b border-border">
          <Input
            placeholder="FILENAME.md"
            value={newFilename}
            onChange={(e) => setNewFilename(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSubmit();
              if (e.key === "Escape") setShowInput(false);
            }}
            className="h-7 text-xs"
            autoFocus
          />
        </div>
      )}
      <div className="py-1">
        {files.length === 0 && (
          <p className="px-3 py-4 text-xs text-muted-foreground text-center">
            No instruction files yet.
          </p>
        )}
        {files.map((f) => (
          <FileRow
            key={f.filename}
            file={f}
            isSelected={f.filename === selectedFile}
            onSelect={() => onSelect(f.filename)}
          />
        ))}
      </div>
    </div>
  );
}

function FileRow({
  file,
  isSelected,
  onSelect,
}: {
  file: InstructionFile;
  isSelected: boolean;
  onSelect: () => void;
}) {
  const Icon = file.is_entry ? IconFileText : IconFile;
  const byteSize = new Blob([file.content]).size;

  return (
    <button
      onClick={onSelect}
      className={cn(
        "w-full flex items-center gap-2 px-3 py-1.5 text-left text-sm cursor-pointer",
        "hover:bg-accent/50 transition-colors",
        isSelected && "bg-accent",
      )}
    >
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="flex-1 truncate">{file.filename}</span>
      <div className="flex items-center gap-1.5 shrink-0">
        {file.is_entry && (
          <Badge variant="secondary" className="text-[10px] px-1 py-0">
            ENTRY
          </Badge>
        )}
        <span className="text-[10px] text-muted-foreground">{formatBytes(byteSize)}</span>
      </div>
    </button>
  );
}
