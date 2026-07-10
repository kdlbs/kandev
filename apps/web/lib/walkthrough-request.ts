import { formatPromptReferenceExpansions } from "@/lib/prompts/expand-prompt-references";

export const CHANGES_WALKTHROUGH_PROMPT_NAME = "changes-walkthrough";

const DIFF_CONTEXT_PLACEHOLDER = "{{diff_context}}";
const CHANGED_FILES_PLACEHOLDER = "{{changed_files}}";
const STATIC_CONTEXT_INSTRUCTION = "Inspect the task changes and compare against the correct base.";

function buildExpansionContent(template: string): string {
  return template
    .trim()
    .replaceAll(DIFF_CONTEXT_PLACEHOLDER, STATIC_CONTEXT_INSTRUCTION)
    .replaceAll(CHANGED_FILES_PLACEHOLDER, STATIC_CONTEXT_INSTRUCTION);
}

export function buildChangesWalkthroughPrompt(template: string): string {
  const expansionContext = formatPromptReferenceExpansions([
    { name: CHANGES_WALKTHROUGH_PROMPT_NAME, content: buildExpansionContent(template) },
  ]);
  return [
    `@${CHANGES_WALKTHROUGH_PROMPT_NAME}`,
    "",
    "<kandev-system>",
    expansionContext,
    "</kandev-system>",
  ].join("\n");
}
