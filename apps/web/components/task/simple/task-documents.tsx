"use client";

import { useState, useCallback } from "react";
import {
  IconChevronDown,
  IconChevronRight,
  IconPlus,
  IconTrash,
  IconDownload,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { toast } from "sonner";
import { formatRelativeTime } from "@/lib/utils";
import {
  listDocuments,
  createOrUpdateDocument,
  deleteDocument,
  type TaskDocument,
} from "@/lib/api/domains/office-extended-api";

// --- Document type config ---

type DocTypeConfig = {
  label: string;
  className: string;
};

const DOC_TYPE_CONFIG: Record<string, DocTypeConfig> = {
  PLAN: {
    label: "PLAN",
    className: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  },
  SPEC: {
    label: "SPEC",
    className: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
  },
  NOTES: {
    label: "NOTES",
    className: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400",
  },
  REVIEW: {
    label: "REVIEW",
    className: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
  },
  ATTACHMENT: {
    label: "ATTACH",
    className: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  },
};

const DOCUMENT_TYPES = ["PLAN", "SPEC", "NOTES", "REVIEW", "ATTACHMENT"] as const;

function getTypeConfig(type: string): DocTypeConfig {
  return (
    DOC_TYPE_CONFIG[type.toUpperCase()] ?? {
      label: type,
      className: "bg-muted text-muted-foreground",
    }
  );
}

// --- New document form ---

type NewDocFormState = {
  key: string;
  type: string;
  title: string;
  content: string;
};

const EMPTY_FORM: NewDocFormState = {
  key: "",
  type: "NOTES",
  title: "",
  content: "",
};

function NewDocumentForm({
  taskId,
  onCreated,
  onCancel,
}: {
  taskId: string;
  onCreated: () => void;
  onCancel: () => void;
}) {
  const [form, setForm] = useState<NewDocFormState>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);

  const update = useCallback((patch: Partial<NewDocFormState>) => {
    setForm((prev) => ({ ...prev, ...patch }));
  }, []);

  const handleSubmit = async () => {
    if (!form.key.trim() || !form.content.trim()) {
      toast.error("Key and content are required");
      return;
    }
    setSaving(true);
    try {
      await createOrUpdateDocument(taskId, form.key.trim(), {
        type: form.type,
        title: form.title.trim() || undefined,
        content: form.content,
      });
      toast.success("Document saved");
      onCreated();
    } catch {
      toast.error("Failed to save document");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="border border-border rounded-lg p-3 space-y-2 bg-muted/30">
      <div className="flex gap-2">
        <input
          className="flex-1 px-2 py-1 text-sm border border-border rounded bg-background"
          placeholder="Key (e.g. plan, notes)"
          value={form.key}
          onChange={(e) => update({ key: e.target.value })}
        />
        <select
          className="px-2 py-1 text-sm border border-border rounded bg-background cursor-pointer"
          value={form.type}
          onChange={(e) => update({ type: e.target.value })}
        >
          {DOCUMENT_TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </div>
      <input
        className="w-full px-2 py-1 text-sm border border-border rounded bg-background"
        placeholder="Title (optional)"
        value={form.title}
        onChange={(e) => update({ title: e.target.value })}
      />
      <Textarea
        placeholder="Content (markdown supported)"
        value={form.content}
        onChange={(e) => update({ content: e.target.value })}
        className="min-h-[100px] text-sm"
      />
      <div className="flex gap-2 justify-end">
        <Button variant="ghost" size="sm" className="cursor-pointer" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" className="cursor-pointer" disabled={saving} onClick={handleSubmit}>
          {saving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  );
}

// --- Document card ---

function AttachmentBody({ doc }: { doc: TaskDocument }) {
  return (
    <div className="flex items-center gap-2 px-3 pb-3">
      <span className="text-sm text-muted-foreground flex-1">
        {doc.filename ?? doc.key}
        {doc.size !== undefined && <span className="ml-1">({Math.round(doc.size / 1024)} KB)</span>}
      </span>
      <a
        href={`/api/v1/office/tasks/${doc.taskId}/documents/${encodeURIComponent(doc.key)}/download`}
        download={doc.filename ?? doc.key}
        className="cursor-pointer"
      >
        <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
          <IconDownload className="h-3.5 w-3.5" />
        </Button>
      </a>
    </div>
  );
}

function DocumentCard({ doc, onDelete }: { doc: TaskDocument; onDelete: (key: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const typeConfig = getTypeConfig(doc.type);
  const isAttachment = doc.type.toUpperCase() === "ATTACHMENT";

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        type="button"
        className="w-full flex items-center gap-2 px-3 py-2.5 hover:bg-accent/50 transition-colors cursor-pointer text-left"
        onClick={() => setExpanded((v) => !v)}
      >
        {expanded ? (
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        ) : (
          <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        )}
        <span
          className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium shrink-0 ${typeConfig.className}`}
        >
          {typeConfig.label}
        </span>
        <span className="flex-1 text-sm font-medium truncate">{doc.title || doc.key}</span>
        <span className="text-xs text-muted-foreground shrink-0">rev {doc.revision}</span>
        <span className="text-xs text-muted-foreground shrink-0 ml-2">
          {formatRelativeTime(doc.updatedAt)}
        </span>
        <button
          type="button"
          className="ml-1 cursor-pointer p-1 rounded hover:bg-destructive/10 hover:text-destructive"
          onClick={(e) => {
            e.stopPropagation();
            onDelete(doc.key);
          }}
        >
          <IconTrash className="h-3 w-3" />
        </button>
      </button>
      {expanded &&
        (isAttachment ? (
          <AttachmentBody doc={doc} />
        ) : (
          <div className="px-3 pb-3 pt-1 text-sm text-muted-foreground whitespace-pre-wrap border-t border-border/50 max-h-[400px] overflow-y-auto">
            {doc.content || <span className="italic">No content</span>}
          </div>
        ))}
    </div>
  );
}

// --- Main component ---

type Props = {
  taskId: string;
};

export function TaskDocuments({ taskId }: Props) {
  const [documents, setDocuments] = useState<TaskDocument[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [showForm, setShowForm] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await listDocuments(taskId);
      setDocuments(res.documents ?? []);
      setLoaded(true);
    } catch {
      toast.error("Failed to load documents");
    } finally {
      setLoading(false);
    }
  }, [taskId]);

  const handleDelete = useCallback(
    async (key: string) => {
      try {
        await deleteDocument(taskId, key);
        setDocuments((prev) => prev?.filter((d) => d.key !== key) ?? null);
        toast.success("Document deleted");
      } catch {
        toast.error("Failed to delete document");
      }
    },
    [taskId],
  );

  const handleCreated = useCallback(async () => {
    setShowForm(false);
    await load();
  }, [load]);

  // Lazy-load on first reveal
  if (!loaded && !loading) {
    load();
  }

  const docs = documents ?? [];

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold">Documents</h2>
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer h-7 text-xs"
          onClick={() => setShowForm((v) => !v)}
        >
          <IconPlus className="h-3.5 w-3.5 mr-1" />
          New document
        </Button>
      </div>

      {showForm && (
        <div className="mb-3">
          <NewDocumentForm
            taskId={taskId}
            onCreated={handleCreated}
            onCancel={() => setShowForm(false)}
          />
        </div>
      )}

      {loading && docs.length === 0 && <p className="text-sm text-muted-foreground">Loading...</p>}

      {loaded && docs.length === 0 && !showForm && (
        <p className="text-sm text-muted-foreground">No documents yet.</p>
      )}

      <div className="space-y-2">
        {docs.map((doc) => (
          <DocumentCard key={doc.key} doc={doc} onDelete={handleDelete} />
        ))}
      </div>
    </div>
  );
}
