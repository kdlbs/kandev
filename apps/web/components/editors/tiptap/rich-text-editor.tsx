/**
 * Narrow, plugin-facing wrappers over the Plan panel's tiptap editor
 * (`TipTapPlanEditor`/`PlanReadOnlyMarkdown`). Exposed on `host.ui` as
 * `RichTextEditor`/`RichTextReadOnly` (docs/plans/plugins/PLUGIN-API.md).
 *
 * Deliberately forward only `{ value, onChange, placeholder, className,
 * testId }` (+ a required `taskId` for the editable variant) — not the plan
 * editor's `comments`/`onSelectionChange`/`onCommentClick`/`onCommentDeleted`/
 * `onEditorReady` props. Keeping the plugin-facing surface small lets the
 * plan editor's internals keep evolving without breaking the frozen contract.
 */
import { PlanReadOnlyMarkdown } from "./tiptap-plan-readonly";
import { TipTapPlanEditor } from "./tiptap-plan-editor";

export interface RichTextEditorProps {
  /** Required by the underlying plan editor (mermaid/image asset scoping). */
  taskId: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  testId?: string;
}

export function RichTextEditor({
  taskId,
  value,
  onChange,
  placeholder,
  className,
  testId,
}: RichTextEditorProps) {
  return (
    <div className={className} data-testid={testId}>
      <TipTapPlanEditor
        taskId={taskId}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
      />
    </div>
  );
}

export interface RichTextReadOnlyProps {
  value: string;
  className?: string;
  testId?: string;
}

export function RichTextReadOnly({ value, className, testId }: RichTextReadOnlyProps) {
  return <PlanReadOnlyMarkdown content={value} className={className} testId={testId} />;
}
