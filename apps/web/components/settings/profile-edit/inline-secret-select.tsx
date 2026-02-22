"use client";

import { useState, useCallback } from "react";
import { IconPlus, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { createSecret } from "@/lib/api/domains/secrets-api";
import { useAppStore } from "@/components/state-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";

const NONE_VALUE = "__none__";
const CREATE_VALUE = "__create__";

type InlineSecretSelectProps = {
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
  label?: string;
  placeholder?: string;
};

export function InlineSecretSelect({
  secretId,
  onSecretIdChange,
  secrets,
  label,
  placeholder = "Select a secret...",
}: InlineSecretSelectProps) {
  const [creating, setCreating] = useState(false);

  const handleValueChange = (v: string) => {
    if (v === CREATE_VALUE) {
      setCreating(true);
      return;
    }
    onSecretIdChange(v === NONE_VALUE ? null : v);
  };

  return (
    <div className="space-y-2">
      {label && <Label className="text-xs text-muted-foreground">{label}</Label>}
      <Select value={secretId ?? NONE_VALUE} onValueChange={handleValueChange}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE_VALUE}>None</SelectItem>
          {secrets.map((s) => (
            <SelectItem key={s.id} value={s.id}>
              {s.name}
            </SelectItem>
          ))}
          <SelectItem value={CREATE_VALUE}>
            <span className="flex items-center gap-1">
              <IconPlus className="h-3.5 w-3.5" />
              Create new secret...
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
      {creating && (
        <InlineCreateForm
          onCreated={(item) => {
            onSecretIdChange(item.id);
            setCreating(false);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}

function InlineCreateForm({
  onCreated,
  onCancel,
}: {
  onCreated: (item: SecretListItem) => void;
  onCancel: () => void;
}) {
  const addSecret = useAppStore((state) => state.addSecret);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = useCallback(async () => {
    if (!name.trim() || !value.trim()) return;
    setSaving(true);
    setError(null);
    try {
      const item = await createSecret({ name: name.trim(), value: value.trim() });
      addSecret(item);
      onCreated(item);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create secret");
      setSaving(false);
    }
  }, [name, value, addSecret, onCreated]);

  return (
    <div className="rounded-md border p-3 space-y-3 bg-muted/30">
      <div className="space-y-1.5">
        <Label className="text-xs">Name</Label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. my-api-token"
          className="h-8 text-sm"
        />
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs">Value</Label>
        <Textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="Paste your secret value..."
          className="text-sm min-h-[60px]"
        />
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
      <div className="flex items-center gap-2 justify-end">
        <Button
          variant="outline"
          size="sm"
          onClick={onCancel}
          disabled={saving}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button
          size="sm"
          onClick={handleSave}
          disabled={!name.trim() || !value.trim() || saving}
          className="cursor-pointer"
        >
          {saving ? <IconLoader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
          Save
        </Button>
      </div>
    </div>
  );
}
