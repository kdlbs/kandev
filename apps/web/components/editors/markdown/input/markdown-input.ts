import { createMarkdownNativeInput } from "./markdown-native-input";
import { createMarkdownTextareaInput } from "./markdown-textarea-input";
import type {
  MarkdownInputFactoryOptions,
  MarkdownInputTransport,
} from "./markdown-input-transport";

export type MarkdownScopedInput = MarkdownInputTransport & {
  runWhenSettled?: (callback: () => void, owner?: symbol) => void;
};

export function createMarkdownInput(
  options: MarkdownInputFactoryOptions,
  label: string,
): MarkdownScopedInput {
  try {
    return createMarkdownNativeInput(options);
  } catch {
    return createMarkdownTextareaInput(options, label);
  }
}
