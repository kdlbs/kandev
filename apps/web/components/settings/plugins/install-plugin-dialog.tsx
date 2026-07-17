"use client";

import { useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { cn } from "@/lib/utils";

type InstallTab = "url" | "upload";

type InstallPluginDialogProps = {
  open: boolean;
  busy: boolean;
  error: string | null;
  onOpenChange: (open: boolean) => void;
  onSubmitUrl: (url: string) => void;
  onSubmitFile: (file: File) => void;
};

/**
 * Replaces the old "Register plugin" manifest-paste dialog with the
 * install-based flow (docs/plans/plugins/GRPC-CONTRACT.md §7): install from a
 * remote tarball URL, or upload a .tar.gz package directly. No credentials
 * are ever shown — installing a plugin has nothing to copy or reveal.
 */
export function InstallPluginDialog({
  open,
  busy,
  error,
  onOpenChange,
  onSubmitUrl,
  onSubmitFile,
}: InstallPluginDialogProps) {
  const [tab, setTab] = useState<InstallTab>("url");
  const [url, setUrl] = useState("");

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      setUrl("");
      setTab("url");
    }
    onOpenChange(next);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg" data-testid="install-plugin-dialog">
        <DialogHeader>
          <DialogTitle>Install plugin</DialogTitle>
          <DialogDescription>
            Install a plugin package from a URL, or upload a .tar.gz file directly.
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as InstallTab)}>
          <TabsList>
            <TabsTrigger
              value="url"
              className="cursor-pointer"
              data-testid="install-plugin-tab-url"
            >
              From URL
            </TabsTrigger>
            <TabsTrigger
              value="upload"
              className="cursor-pointer"
              data-testid="install-plugin-tab-upload"
            >
              Upload file
            </TabsTrigger>
          </TabsList>
          <TabsContent value="url" className="pt-3">
            <UrlTab url={url} setUrl={setUrl} busy={busy} onSubmit={onSubmitUrl} />
          </TabsContent>
          <TabsContent value="upload" className="pt-3">
            <UploadTab busy={busy} onSubmit={onSubmitFile} />
          </TabsContent>
        </Tabs>

        {error && (
          <div data-testid="install-plugin-error" className="text-xs text-destructive" role="alert">
            {error}
          </div>
        )}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function UrlTab({
  url,
  setUrl,
  busy,
  onSubmit,
}: {
  url: string;
  setUrl: (v: string) => void;
  busy: boolean;
  onSubmit: (url: string) => void;
}) {
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="install-plugin-url">Package URL</Label>
        <Input
          id="install-plugin-url"
          data-testid="install-plugin-url-input"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://example.com/acme-tools-1.0.0.tar.gz"
          className="font-mono text-sm"
        />
      </div>
      <Button
        type="button"
        data-testid="install-plugin-url-submit"
        onClick={() => onSubmit(url.trim())}
        disabled={busy || url.trim().length === 0}
        className="cursor-pointer"
      >
        Install
      </Button>
    </div>
  );
}

function UploadTab({ busy, onSubmit }: { busy: boolean; onSubmit: (file: File) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  };

  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer.types.includes("Files")) setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
    const dropped = e.dataTransfer.files[0];
    if (dropped) setFile(dropped);
  };

  return (
    <div className="space-y-3">
      <div
        data-testid="install-plugin-dropzone"
        onDragOver={handleDragOver}
        onDragEnter={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        className={cn(
          "flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-border/70 p-6 text-center text-sm text-muted-foreground cursor-pointer",
          isDragging && "border-primary bg-primary/5 ring-1 ring-primary/30",
        )}
      >
        <span>
          {file ? (
            <span className="font-mono text-foreground">{file.name}</span>
          ) : (
            "Drag and drop a .tar.gz package here, or click to browse"
          )}
        </span>
        <input
          ref={inputRef}
          type="file"
          accept=".tar.gz,.tgz,application/gzip"
          data-testid="install-plugin-file-input"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          className="hidden"
        />
      </div>
      <Button
        type="button"
        data-testid="install-plugin-upload-submit"
        onClick={() => file && onSubmit(file)}
        disabled={busy || !file}
        className="cursor-pointer"
      >
        Install
      </Button>
    </div>
  );
}
