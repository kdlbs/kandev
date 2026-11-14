"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  NOTE_CITATION_KINDS,
  type NoteCitation,
  type NoteCitationKind,
} from "@/lib/api/domains/coordinator-notes-api";
import { NOTE_CITATIONS_MAX, citationKindLabel, emptyCitation } from "@/lib/coordinator/notes";

type CitationEditorProps = {
  value: NoteCitation[];
  onChange: (next: NoteCitation[]) => void;
  disabled: boolean;
  /** Index of the citation the server rejected, when it named one. */
  failedIndex?: number;
};

type RowProps = {
  index: number;
  citation: NoteCitation;
  disabled: boolean;
  failed: boolean;
  onChange: (next: NoteCitation) => void;
  onRemove: () => void;
};

function Field(props: {
  id: string;
  label: string;
  value: string;
  disabled: boolean;
  type?: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="min-w-0 flex-1 space-y-1">
      <Label htmlFor={props.id} className="text-xs">
        {props.label}
      </Label>
      <Input
        id={props.id}
        type={props.type}
        className="min-h-11 sm:min-h-8"
        value={props.value}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.target.value)}
      />
    </div>
  );
}

function CitationFields({ index, citation, disabled, onChange }: RowProps) {
  const { t } = useTranslation();
  const id = (name: string) => `note-citation-${index}-${name}`;
  if (citation.kind === "pr") {
    return (
      <>
        <Field
          id={id("repo")}
          label={t("coordinator:noteCitationRepository")}
          value={citation.repository_id ?? ""}
          disabled={disabled}
          onChange={(repository_id) => onChange({ ...citation, repository_id })}
        />
        <Field
          id={id("number")}
          type="number"
          label={t("coordinator:noteCitationNumber")}
          value={citation.number === undefined ? "" : String(citation.number)}
          disabled={disabled}
          onChange={(value) =>
            onChange({ ...citation, number: value === "" ? undefined : Number(value) })
          }
        />
      </>
    );
  }
  if (citation.kind === "file") {
    return (
      <>
        <Field
          id={id("repo")}
          label={t("coordinator:noteCitationRepository")}
          value={citation.repository_id ?? ""}
          disabled={disabled}
          onChange={(repository_id) => onChange({ ...citation, repository_id })}
        />
        <Field
          id={id("path")}
          label={t("coordinator:noteCitationPath")}
          value={citation.path ?? ""}
          disabled={disabled}
          onChange={(path) => onChange({ ...citation, path })}
        />
        <Field
          id={id("sha")}
          label={t("coordinator:noteCitationSha")}
          value={citation.sha ?? ""}
          disabled={disabled}
          onChange={(sha) => onChange({ ...citation, sha })}
        />
      </>
    );
  }
  return (
    <Field
      id={id("id")}
      label={t("coordinator:noteCitationId")}
      value={citation.id ?? ""}
      disabled={disabled}
      onChange={(value) => onChange({ ...citation, id: value })}
    />
  );
}

function CitationRow(props: RowProps) {
  const { t } = useTranslation();
  const { index, citation, disabled, failed, onChange, onRemove } = props;
  return (
    <li
      className="flex flex-col gap-2 rounded-md border p-2 sm:flex-row sm:items-end"
      data-testid={`note-citation-row-${index}`}
      aria-invalid={failed || undefined}
    >
      <div className="space-y-1 sm:w-40">
        <Label htmlFor={`note-citation-${index}-kind`} className="text-xs">
          {t("coordinator:noteCitationKind")}
        </Label>
        <Select
          value={citation.kind}
          disabled={disabled}
          onValueChange={(kind) => onChange({ ...emptyCitation(), kind: kind as NoteCitationKind })}
        >
          <SelectTrigger
            id={`note-citation-${index}-kind`}
            className="min-h-11 w-full cursor-pointer sm:min-h-8"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {NOTE_CITATION_KINDS.map((kind) => (
              <SelectItem key={kind} value={kind}>
                {citationKindLabel(t, kind)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <CitationFields {...props} />
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        className="min-h-11 cursor-pointer sm:min-h-8"
        data-testid={`note-citation-remove-${index}`}
        onClick={onRemove}
      >
        {t("coordinator:noteCitationRemove")}
      </Button>
    </li>
  );
}

/** Typed citation rows: kind picker plus the identifier fields of that kind. */
export function CitationEditor({ value, onChange, disabled, failedIndex }: CitationEditorProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="note-citations">
      <p className="text-sm font-medium">{t("coordinator:noteCitationsLabel")}</p>
      <p className="text-xs text-muted-foreground">{t("coordinator:noteCitationsHelp")}</p>
      <ul className="space-y-2">
        {value.map((citation, index) => (
          <CitationRow
            key={index}
            index={index}
            citation={citation}
            disabled={disabled}
            failed={failedIndex === index}
            onChange={(next) => onChange(value.map((c, i) => (i === index ? next : c)))}
            onRemove={() => onChange(value.filter((_, i) => i !== index))}
          />
        ))}
      </ul>
      <Button
        type="button"
        variant="outline"
        disabled={disabled || value.length >= NOTE_CITATIONS_MAX}
        className="min-h-11 cursor-pointer sm:min-h-8"
        data-testid="note-citation-add"
        onClick={() => onChange([...value, emptyCitation()])}
      >
        {t("coordinator:noteCitationAdd")}
      </Button>
    </div>
  );
}
