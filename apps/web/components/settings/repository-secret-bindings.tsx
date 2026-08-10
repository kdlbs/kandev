"use client";

import { IconPlus, IconTrash } from "@tabler/icons-react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useWorkspaceSecretOptions } from "@/hooks/domains/settings/use-workspace-secret-options";
import type { Repository, RepositorySecretBinding } from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";

const EMPTY_SECRET = "__empty_secret__";
const POSIX_KEY = /^[A-Za-z_][A-Za-z0-9_]*$/;

export type RepositorySecretBindingValidation =
  | { kind: "key"; key: string }
  | { kind: "duplicate"; key: string }
  | { kind: "reserved"; key: string }
  | { kind: "secret"; key: string }
  | null;

export function validateRepositorySecretBindings(
  bindings: RepositorySecretBinding[] = [],
): RepositorySecretBindingValidation {
  const seen = new Set<string>();
  for (const binding of bindings) {
    const key = binding.key.trim();
    if (!key || key !== binding.key || key.length > 256 || !POSIX_KEY.test(key)) {
      return { kind: "key", key };
    }
    if (key === "TASK_DESCRIPTION" || key.startsWith("KANDEV_")) {
      return { kind: "reserved", key };
    }
    if (seen.has(key)) return { kind: "duplicate", key };
    seen.add(key);
    if (!binding.secret_id.trim()) return { kind: "secret", key };
  }
  return null;
}

function secretScopeLabel(t: TFunction, secret: SecretListItem) {
  return secret.scope === "workspace"
    ? t("workspaces:workspaceSecretScope")
    : t("workspaces:globalSecretScope");
}

function secretLabel(t: TFunction, secret: SecretListItem) {
  return `${secret.name} - ${secretScopeLabel(t, secret)}`;
}

type BindingRowProps = {
  binding: RepositorySecretBinding;
  index: number;
  options: SecretListItem[];
  onUpdate: (index: number, patch: Partial<RepositorySecretBinding>) => void;
  onRemove: (index: number) => void;
};

function BindingRow({ binding, index, options, onUpdate, onRemove }: BindingRowProps) {
  const { t } = useTranslation();
  const selected = options.find((secret) => secret.id === binding.secret_id);
  const hasMissingSecret = Boolean(binding.secret_id) && !selected;
  return (
    <div
      className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] sm:items-end"
      data-testid={`repository-secret-binding-${index}`}
    >
      <div className="min-w-0 space-y-1">
        <Label htmlFor={`repository-secret-key-${index}`} className="text-xs">
          {t("workspaces:environmentSecretKey")}
        </Label>
        <Input
          id={`repository-secret-key-${index}`}
          value={binding.key}
          onChange={(event) => onUpdate(index, { key: event.target.value })}
          placeholder={t("workspaces:environmentSecretKeyPlaceholder")}
          className="min-h-11 font-mono text-xs"
          data-testid={`repository-secret-key-${index}`}
        />
      </div>
      <div className="min-w-0 space-y-1">
        <Label htmlFor={`repository-secret-value-${index}`} className="text-xs">
          {t("workspaces:environmentSecretValue")}
        </Label>
        <Select
          value={binding.secret_id || EMPTY_SECRET}
          onValueChange={(secretId) =>
            onUpdate(index, { secret_id: secretId === EMPTY_SECRET ? "" : secretId })
          }
        >
          <SelectTrigger
            id={`repository-secret-value-${index}`}
            className="min-h-11 min-w-0 text-xs"
            data-testid={`repository-secret-select-${index}`}
          >
            <SelectValue placeholder={t("workspaces:selectEnvironmentSecret")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={EMPTY_SECRET}>{t("workspaces:selectEnvironmentSecret")}</SelectItem>
            {hasMissingSecret && (
              <SelectItem value={binding.secret_id}>
                {t("workspaces:missingSecretReference")}
              </SelectItem>
            )}
            {options.map((secret) => (
              <SelectItem key={secret.id} value={secret.id}>
                {secretLabel(t, secret)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-11 w-11 cursor-pointer justify-self-end"
        onClick={() => onRemove(index)}
        aria-label={t("workspaces:removeEnvironmentSecret", { key: binding.key || index + 1 })}
        data-testid={`repository-secret-remove-${index}`}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}

type RepositorySecretBindingsProps = {
  repository: Repository;
  onUpdate: (repositoryId: string, updates: Partial<Repository>) => void;
};

export function RepositorySecretBindings({ repository, onUpdate }: RepositorySecretBindingsProps) {
  const { t } = useTranslation();
  const { items, loaded } = useWorkspaceSecretOptions(repository.workspace_id);
  const bindings = repository.secret_bindings ?? [];

  const updateBinding = (index: number, patch: Partial<RepositorySecretBinding>) => {
    onUpdate(repository.id, {
      secret_bindings: bindings.map((binding, currentIndex) =>
        currentIndex === index ? { ...binding, ...patch } : binding,
      ),
    });
  };

  const removeBinding = (index: number) => {
    onUpdate(repository.id, {
      secret_bindings: bindings.filter((_, currentIndex) => currentIndex !== index),
    });
  };

  const addBinding = () => {
    onUpdate(repository.id, {
      secret_bindings: [...bindings, { key: "", secret_id: "" }],
    });
  };

  return (
    <div className="space-y-3" data-testid="repository-secret-bindings">
      <div>
        <div className="text-sm font-medium text-foreground">
          {t("workspaces:environmentSecrets")}
        </div>
        <p className="text-xs text-muted-foreground">
          {t("workspaces:environmentSecretsDescription")}
        </p>
      </div>
      {bindings.length === 0 ? (
        <p className="rounded-md border border-dashed border-border/70 p-3 text-xs text-muted-foreground">
          {t("workspaces:noEnvironmentSecrets")}
        </p>
      ) : (
        <div className="space-y-3" data-testid="repository-secret-binding-list">
          {bindings.map((binding, index) => (
            <BindingRow
              key={index}
              binding={binding}
              index={index}
              options={items}
              onUpdate={updateBinding}
              onRemove={removeBinding}
            />
          ))}
        </div>
      )}
      {!loaded && <p className="text-xs text-muted-foreground">{t("settings:loadingSecrets")}</p>}
      {loaded && items.length === 0 && bindings.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("workspaces:noAvailableEnvironmentSecrets")}
        </p>
      )}
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={addBinding}
        data-testid="repository-secret-add"
      >
        <IconPlus className="mr-1 h-4 w-4" />
        {t("workspaces:addEnvironmentSecret")}
      </Button>
    </div>
  );
}
