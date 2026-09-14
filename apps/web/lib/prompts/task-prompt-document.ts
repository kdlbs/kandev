import { buildPromptMentionNames } from "./prompt-mention-parser";
import { splitMarkdownPromptMentionSegments } from "@/components/task/chat/messages/prompt-mention-components";

export type TaskPromptReference = {
  name: string;
  value: string;
};

export type TaskPromptTextNode = {
  type: "text";
  text: string;
};

export type TaskPromptReferenceNode = {
  type: "promptReference";
  attrs: TaskPromptReference;
};

export type TaskPromptHardBreakNode = {
  type: "hardBreak";
};

export type TaskPromptInlineNode =
  | TaskPromptTextNode
  | TaskPromptReferenceNode
  | TaskPromptHardBreakNode;

export type TaskPromptParagraph = {
  type: "paragraph";
  content?: TaskPromptInlineNode[];
};

export type TaskPromptDocument = {
  type: "doc";
  content: TaskPromptParagraph[];
};

export function buildTaskPromptDocument(
  text: string,
  promptNames: readonly string[],
): TaskPromptDocument {
  const preparedNames = buildPromptMentionNames([...promptNames]);
  const paragraphs: TaskPromptParagraph[] = [{ type: "paragraph", content: [] }];
  const segments = splitMarkdownPromptMentionSegments(text, preparedNames);

  for (const segment of segments) {
    if (segment.kind === "prompt") {
      currentParagraph(paragraphs).content!.push({
        type: "promptReference",
        attrs: { name: segment.name, value: segment.value },
      });
      continue;
    }

    appendText(paragraphs, segment.value);
  }

  return {
    type: "doc",
    content: paragraphs.map((paragraph) =>
      paragraph.content && paragraph.content.length > 0 ? paragraph : { type: "paragraph" },
    ),
  };
}

export function serializeTaskPromptDocument(document: TaskPromptDocument): string {
  return document.content
    .map((paragraph) =>
      (paragraph.content ?? [])
        .map((node) => {
          if (node.type === "text") return node.text;
          if (node.type === "hardBreak") return "\n";
          return node.attrs.value;
        })
        .join(""),
    )
    .join("\n");
}

export function collectTaskPromptReferences(document: TaskPromptDocument): TaskPromptReference[] {
  return document.content.flatMap((paragraph) =>
    (paragraph.content ?? [])
      .filter((node): node is TaskPromptReferenceNode => node.type === "promptReference")
      .map((node) => ({ ...node.attrs })),
  );
}

function currentParagraph(paragraphs: TaskPromptParagraph[]): TaskPromptParagraph {
  return paragraphs[paragraphs.length - 1];
}

function appendText(paragraphs: TaskPromptParagraph[], value: string) {
  let start = 0;
  for (let index = 0; index <= value.length; index += 1) {
    if (index < value.length && value[index] !== "\n") continue;
    const line = value.slice(start, index);
    if (line) currentParagraph(paragraphs).content!.push({ type: "text", text: line });
    if (index < value.length) {
      paragraphs.push({ type: "paragraph", content: [] });
    }
    start = index + 1;
  }
}
