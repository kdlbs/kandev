"use client";

import { mergeAttributes, Node } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from "@tiptap/react";
import { IconHash } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  changeEntityReferenceAssociationMode,
  isLinkableEntityReference,
  ReferenceRegistrationControl,
} from "./reference-registration";

export const EntityReferenceNode = Node.create<{ registrationEnabled: boolean }>({
  name: "entityReference",
  addOptions() {
    return { registrationEnabled: true };
  },
  group: "inline",
  inline: true,
  selectable: false,
  atom: true,

  addAttributes() {
    return {
      version: { default: 1 },
      ref: { default: null },
      provider: { default: null },
      kind: { default: null },
      id: { default: null },
      key: { default: null },
      title: { default: null },
      url: { default: null },
      scope: { default: null },
      selection_id: { default: null },
      association_mode: { default: null },
      registration_enabled: { default: this.options.registrationEnabled },
    };
  },

  parseHTML() {
    return [{ tag: "span[data-entity-reference]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes({ "data-entity-reference": "" }, HTMLAttributes),
      HTMLAttributes.key || HTMLAttributes.title || HTMLAttributes.id || "",
    ];
  },

  renderText({ node }) {
    return `#${node.attrs.key ?? node.attrs.title ?? node.attrs.id ?? ""}`;
  },

  addNodeView() {
    return ReactNodeViewRenderer(EntityReferenceChipView);
  },
});

function EntityReferenceChipView({ node, updateAttributes }: ReactNodeViewProps) {
  const { key, title, provider, kind, id } = node.attrs as {
    key?: string | null;
    title?: string | null;
    provider?: string | null;
    kind?: string | null;
    id?: string | null;
  };
  const label = key || title || id || t("common:reference");
  const source = [provider, kind].filter(Boolean).join(" ");
  const description = source ? `${source}: ${title || label}` : title || label;
  const reference = node.attrs as EntityReference;
  const registrationEnabled = node.attrs.registration_enabled !== false;
  return (
    <NodeViewWrapper as="span" className="inline">
      <span contentEditable={false} title={description} data-testid="entity-reference-chip">
        {isLinkableEntityReference(reference) ? (
          <ReferenceRegistrationControl
            referenceLabel={`#${label}`}
            mode={reference.association_mode}
            linkingEnabled={registrationEnabled}
            onModeChange={(mode) =>
              updateAttributes(changeEntityReferenceAssociationMode(reference, mode))
            }
          />
        ) : (
          <span className="inline-flex max-w-[180px] cursor-default items-center gap-0.5 rounded bg-primary/10 px-1.5 py-0.5 align-baseline text-xs text-primary">
            <IconHash className="h-3 w-3 shrink-0" aria-hidden="true" />
            <span className="truncate">{label}</span>
          </span>
        )}
      </span>
    </NodeViewWrapper>
  );
}
