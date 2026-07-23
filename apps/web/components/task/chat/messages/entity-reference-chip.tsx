import { IconHash } from "@tabler/icons-react";
import type { ComponentPropsWithoutRef } from "react";
import type { Components, ExtraProps } from "react-markdown";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  entityReferenceHref,
  entityReferenceLabel,
  matchEntityReferenceLink,
} from "@/lib/entity-references/message-references";
import { markdownComponents } from "@/components/shared/markdown-components";

type EntityReferenceChipProps = {
  reference: EntityReference;
};

export function EntityReferenceChip({ reference }: EntityReferenceChipProps) {
  const label = entityReferenceLabel(reference);
  const isInternal = reference.url.startsWith("/") || reference.url.startsWith("#");
  return (
    <a
      href={entityReferenceHref(reference)}
      target={isInternal ? "_self" : "_blank"}
      rel={isInternal ? undefined : "noopener noreferrer"}
      aria-label={`Open ${label}: ${reference.title}`}
      title={`${reference.provider} ${reference.kind}: ${reference.title}`}
      data-testid="entity-reference-chip"
      data-entity-ref={reference.ref}
      data-entity-provider={reference.provider}
      data-entity-kind={reference.kind}
      className="inline-flex max-w-full items-center gap-0.5 rounded-md bg-primary/15 px-1.5 py-0.5 align-baseline text-[0.88em] font-medium text-primary no-underline ring-1 ring-inset ring-primary/20 hover:bg-primary/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
    >
      <IconHash className="h-3 w-3 shrink-0" aria-hidden="true" />
      <span className="min-w-0 truncate">{reference.key || reference.title}</span>
    </a>
  );
}

type EntityReferenceAnchorProps = ComponentPropsWithoutRef<"a"> &
  ExtraProps & {
    references: readonly EntityReference[];
  };

function EntityReferenceAnchor({
  references,
  children,
  href,
  ...props
}: EntityReferenceAnchorProps) {
  const node = props.node;
  const textChild = node?.children.length === 1 ? node.children[0] : null;
  const label = textChild?.type === "text" ? textChild.value : null;
  const reference = label === null ? null : matchEntityReferenceLink(references, label, href);
  if (reference) return <EntityReferenceChip reference={reference} />;
  const MarkdownAnchor = markdownComponents.a;
  return (
    <MarkdownAnchor href={href} {...props}>
      {children}
    </MarkdownAnchor>
  );
}

export function buildEntityReferenceMarkdownComponents(
  references: readonly EntityReference[],
): Components {
  if (references.length === 0) return markdownComponents;
  return {
    ...markdownComponents,
    a: (props) => <EntityReferenceAnchor {...props} references={references} />,
  };
}
