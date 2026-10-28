import { describe, expect, it } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  entityReferenceMarkdown,
  entityReferencesForAdmission,
  entityReferencesFromMetadata,
  matchEntityReferenceLink,
  survivingEntityReferences,
} from "./message-references";

function taskReference(overrides: Partial<EntityReference> = {}): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:kandev:task:workspace-1:task-1",
    provider: "kandev",
    kind: "task",
    id: "task-1",
    key: "KAN-1",
    title: "Fix authentication",
    url: "/t/task-1",
    scope: "workspace-1",
    ...overrides,
  };
}

function issueReference(overrides: Partial<EntityReference> = {}): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:jira:issue:https%3A%2F%2Fjira.example:10001",
    provider: "jira",
    kind: "issue",
    id: "10001",
    key: "ENG[1]",
    title: "Fix the login flow",
    url: "https://jira.example/browse/ENG-1?q=hello world(test)",
    scope: "https://jira.example",
    ...overrides,
  };
}

function pullRequestReference(overrides: Partial<EntityReference> = {}): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:github:pull_request:github.com%2Facme%2Fapp:PR_kwDOAA",
    provider: "github",
    kind: "pull_request",
    id: "PR_kwDOAA",
    key: "acme/app#12",
    title: "Add authentication",
    url: "https://github.com/acme/app/pull/12",
    scope: "github.com/acme/app",
    ...overrides,
  };
}

describe("entityReferencesFromMetadata", () => {
  it("runtime-validates v1 references and deduplicates ref in first-valid order", () => {
    const first = taskReference();
    const second = issueReference();
    const metadata = {
      entity_references: [
        { ...first, version: 2 },
        { ...first, url: "javascript:alert(1)" },
        first,
        { ...first, title: "Conflicting duplicate" },
        second,
      ],
    };

    expect(entityReferencesFromMetadata(metadata)).toEqual([first, second]);
  });

  it("rejects malformed metadata containers and noncanonical identities", () => {
    expect(entityReferencesFromMetadata({ entity_references: "not-an-array" })).toEqual([]);
    expect(
      entityReferencesFromMetadata({
        entity_references: [taskReference({ ref: "made-up", title: "" })],
      }),
    ).toEqual([]);
  });

  it("preserves valid selection intent and rejects forged linking modes", () => {
    const linked = pullRequestReference({
      selection_id: "selection-1",
      association_mode: "link_on_send",
    });
    expect(entityReferencesFromMetadata({ entity_references: [linked] })).toEqual([linked]);
    expect(
      entityReferencesFromMetadata({
        entity_references: [
          pullRequestReference({ association_mode: "link_on_send" }),
          pullRequestReference({
            selection_id: "selection-2",
            association_mode: "anything" as "context_only",
          }),
        ],
      }),
    ).toEqual([]);
  });
});

describe("entity reference admission", () => {
  const gitlabMergeRequest = pullRequestReference({
    provider: "gitlab",
    kind: "merge_request",
    scope: "gitlab.example/acme/app",
    ref: "mention:v1:gitlab:merge_request:gitlab.example%2Facme%2Fapp:12",
    selection_id: "selection-gl",
    association_mode: "link_on_send",
  });

  it.each([
    pullRequestReference({ selection_id: "selection-gh", association_mode: "link_on_send" }),
    gitlabMergeRequest,
  ])("downgrades unsupported queued registration intent to context-only", (reference) => {
    expect(entityReferencesForAdmission([reference], false)).toEqual([
      { ...reference, association_mode: "context_only" },
    ]);
  });

  it("preserves direct admission intent and non-registration references", () => {
    const selected = pullRequestReference({
      selection_id: "selection-direct",
      association_mode: "link_on_send",
    });
    const literal = taskReference();
    expect(entityReferencesForAdmission([selected, literal], true)).toEqual([selected, literal]);
    expect(entityReferencesForAdmission([literal], false)).toEqual([literal]);
  });
});

describe("entity reference Markdown matching", () => {
  it("matches only the exact generated label and encoded URL", () => {
    const reference = issueReference();

    expect(entityReferenceMarkdown(reference)).toBe(
      "[#ENG\\[1\\]](https://jira.example/browse/ENG-1?q=hello%20world%28test%29)",
    );
    expect(
      matchEntityReferenceLink([reference], "#ENG[1]", reference.url.replaceAll(" ", "%20")),
    ).toBeNull();
    expect(
      matchEntityReferenceLink(
        [reference],
        "#ENG[1]",
        "https://jira.example/browse/ENG-1?q=hello%20world%28test%29",
      ),
    ).toEqual(reference);
    expect(
      matchEntityReferenceLink(
        [reference],
        "#Wrong",
        "https://jira.example/browse/ENG-1?q=hello%20world%28test%29",
      ),
    ).toBeNull();
    expect(
      matchEntityReferenceLink([reference], "#ENG[1]", "https://evil.example/ENG-1"),
    ).toBeNull();
  });
});

describe("survivingEntityReferences", () => {
  it("returns exact generated links in document order and deduplicates repeats", () => {
    const task = taskReference();
    const issue = issueReference();
    const taskMarkdown = entityReferenceMarkdown(task);
    const issueMarkdown = entityReferenceMarkdown(issue);
    const content = [
      issueMarkdown,
      "[wrong label](/t/task-1)",
      taskMarkdown,
      issueMarkdown,
      "[#KAN-1](https://evil.example/task-1)",
    ].join(" then ");

    expect(survivingEntityReferences(content, [task, issue])).toEqual([issue, task]);
  });

  it("returns an empty replacement when no generated links survive", () => {
    expect(survivingEntityReferences("plain edited text", [taskReference()])).toEqual([]);
  });

  it("ignores generated-link text that Markdown treats as literal code or escaped text", () => {
    const reference = taskReference();
    const markdown = entityReferenceMarkdown(reference);
    const content = [
      `\\${markdown}`,
      `\`${markdown}\``,
      `\`\`\`md\n${markdown}\n\`\`\``,
      `!${markdown}`,
    ].join("\n");

    expect(survivingEntityReferences(content, [reference])).toEqual([]);
  });
});
