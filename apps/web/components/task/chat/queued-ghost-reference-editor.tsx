"use client";

import { useCallback } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { EntityReference, EntityReferenceAssociationMode } from "@/lib/types/entity-reference";
import { Button } from "@kandev/ui/button";
import { EntityReferenceMenu } from "./entity-reference-menu";
import {
  changeEntityReferenceAssociationMode,
  isLinkableEntityReference,
  ReferenceRegistrationControl,
} from "./reference-registration";
import { removeQueueReference } from "@/hooks/domains/session/queue-registration";
import type { TFunction } from "i18next";
import { useQueuedGhostReferenceSearch } from "./queued-ghost-reference-search";

type QueuedGhostReferenceEditorProps = {
  taskId: string;
  sessionId: string;
  value: string;
  references: readonly EntityReference[];
  disabled: boolean;
  onValueChange: (value: string) => void;
  onReferencesChange: (references: EntityReference[]) => void;
};

function registrationStatusLabel(reference: EntityReference, t: TFunction): string | null {
  const number = reference.url.split(/[/?#]/u).filter(Boolean).at(-1) || reference.id;
  switch (reference.registration_status) {
    case "pending":
      return t("task:referenceRegistrationPending");
    case "linked":
      return t(
        reference.provider === "gitlab"
          ? "task:referenceRegistrationLinkedMergeRequest"
          : "task:referenceRegistrationLinkedPullRequest",
        { number },
      );
    case "failed":
      return t("task:referenceRegistrationFailed");
    case "suppressed":
      return t("task:referenceRegistrationSuppressed");
    case "uncertain":
      return t("task:referenceRegistrationUncertain");
    case "unlinked":
      return t("task:referenceRegistrationUnlinked");
    default:
      return null;
  }
}

type QueuedGhostReferenceChipProps = {
  reference: EntityReference;
  disabled: boolean;
  onModeChange: (mode: EntityReferenceAssociationMode) => void;
  onRemove: () => void;
};

function QueuedGhostReferenceChip({
  reference,
  disabled,
  onModeChange,
  onRemove,
}: QueuedGhostReferenceChipProps) {
  const { t } = useTranslation();
  const status = registrationStatusLabel(reference, t);
  return (
    <div
      className="flex min-w-0 items-center gap-1 rounded-md bg-muted/60 px-1"
      data-testid="queue-reference-chip"
      data-selection-id={reference.selection_id}
    >
      {isLinkableEntityReference(reference) ? (
        <ReferenceRegistrationControl
          referenceLabel={`#${reference.key || reference.title}`}
          mode={reference.association_mode}
          commitOn="save"
          linkAgain={reference.registration_status === "unlinked"}
          onModeChange={onModeChange}
        />
      ) : (
        <span className="max-w-[16rem] truncate px-1.5 py-1 text-xs">
          #{reference.key || reference.title}
        </span>
      )}
      <button
        type="button"
        aria-label={t("task:remove2", { name: reference.key || reference.title })}
        data-testid="queue-reference-remove"
        disabled={disabled}
        className="inline-flex min-h-7 min-w-7 cursor-pointer items-center justify-center rounded text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
        onClick={onRemove}
      >
        <IconX className="size-3.5" aria-hidden="true" />
      </button>
      {status && (
        <span
          className="max-w-[14rem] truncate px-1 text-xs text-muted-foreground"
          data-testid="queue-reference-registration-status"
        >
          {status}
        </span>
      )}
    </div>
  );
}

type QueuedGhostReferenceSearchPanelProps = Pick<
  ReturnType<typeof useQueuedGhostReferenceSearch>,
  | "inputRef"
  | "query"
  | "setQuery"
  | "searchOpen"
  | "setSearchOpen"
  | "selectedIndex"
  | "setSelectedIndex"
  | "search"
  | "clientRect"
  | "selectReference"
  | "handleSearchKeyDown"
> & { disabled: boolean };

function QueuedGhostReferenceSearchPanel({
  inputRef,
  query,
  setQuery,
  searchOpen,
  setSearchOpen,
  selectedIndex,
  setSelectedIndex,
  search,
  clientRect,
  selectReference,
  handleSearchKeyDown,
  disabled,
}: QueuedGhostReferenceSearchPanelProps) {
  const { t } = useTranslation();
  if (!searchOpen) return null;
  return (
    <>
      <input
        ref={inputRef}
        aria-label={t("task:referenceWorkItems")}
        data-testid="queue-reference-search"
        value={query}
        disabled={disabled}
        placeholder={t("task:typeToSearchWorkItems")}
        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
        onFocus={() => setSearchOpen(true)}
        onChange={(event) => {
          setQuery(event.target.value);
          setSelectedIndex(0);
        }}
        onKeyDown={handleSearchKeyDown}
      />
      <EntityReferenceMenu
        isOpen={searchOpen}
        clientRect={clientRect}
        groups={search.groups}
        query={query}
        selectedIndex={selectedIndex}
        isSearching={search.isSearching}
        error={search.error}
        onRetry={search.retry}
        onSelect={selectReference}
        onClose={() => setSearchOpen(false)}
        setSelectedIndex={setSelectedIndex}
      />
    </>
  );
}

export function QueuedGhostReferenceEditor({
  taskId,
  sessionId,
  value,
  references,
  disabled,
  onValueChange,
  onReferencesChange,
}: QueuedGhostReferenceEditorProps) {
  const { t } = useTranslation();
  const searchState = useQueuedGhostReferenceSearch({
    taskId,
    sessionId,
    value,
    references,
    onValueChange,
    onReferencesChange,
  });
  const { inputRef, setSearchOpen } = searchState;

  const updateMode = useCallback(
    (reference: EntityReference, mode: EntityReferenceAssociationMode) => {
      onReferencesChange(
        references.map((current) =>
          current.ref === reference.ref
            ? changeEntityReferenceAssociationMode(current, mode)
            : current,
        ),
      );
    },
    [onReferencesChange, references],
  );

  const removeReference = useCallback(
    (reference: EntityReference) => {
      const next = removeQueueReference(value, references, reference.ref);
      onValueChange(next.value);
      onReferencesChange(next.references);
    },
    [onReferencesChange, onValueChange, references, value],
  );

  return (
    <div
      className="space-y-2 rounded-md border border-border/70 p-2"
      data-testid="queue-reference-editor"
    >
      <div className="flex flex-wrap items-center gap-2">
        {references.map((reference) => (
          <QueuedGhostReferenceChip
            key={reference.ref}
            reference={reference}
            disabled={disabled}
            onModeChange={(mode) => updateMode(reference, mode)}
            onRemove={() => removeReference(reference)}
          />
        ))}
        <Button
          type="button"
          size="sm"
          variant="outline"
          data-testid="queue-reference-add"
          disabled={disabled}
          className="min-h-8 cursor-pointer [@media(pointer:coarse)]:min-h-11"
          onClick={() => {
            setSearchOpen(true);
            requestAnimationFrame(() => inputRef.current?.focus());
          }}
        >
          <IconPlus className="mr-1 size-3.5" aria-hidden="true" />
          {t("task:referenceWorkItems")}
        </Button>
      </div>
      {references.some(isLinkableEntityReference) && (
        <p className="text-xs text-muted-foreground" data-testid="queue-reference-disclosure">
          {t("task:referenceRetainedLinksDisclosure")}
        </p>
      )}
      <QueuedGhostReferenceSearchPanel {...searchState} disabled={disabled} />
    </div>
  );
}
