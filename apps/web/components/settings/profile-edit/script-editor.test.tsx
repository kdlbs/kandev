import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useEffect } from "react";
import type { CancellationToken, Position, editor, languages } from "monaco-editor";

const mocks = vi.hoisted(() => {
  const registrations: Array<{
    provider: languages.CompletionItemProvider;
    dispose: ReturnType<typeof vi.fn>;
  }> = [];
  const mountedModels = new Map<string, editor.ITextModel>();
  const pendingMounts: Array<() => void> = [];
  const editorOptions: Array<{ ariaLabel?: string }> = [];
  const monaco = {
    editor: { defineTheme: vi.fn() },
    languages: {
      CompletionItemKind: { Reference: 2, Variable: 1 },
      registerCompletionItemProvider: vi.fn(
        (_language: string, provider: languages.CompletionItemProvider) => {
          const registration = { provider, dispose: vi.fn() };
          registrations.push(registration);
          return registration;
        },
      ),
    },
  };
  return { monaco, registrations, mountedModels, pendingMounts, editorOptions, deferMount: false };
});

const MENTION_EDITOR_VALUE = "mention-editor";

vi.mock("@/lib/routing/client-dynamic", () => ({
  default: () =>
    function MockMonacoEditor({
      value,
      beforeMount,
      onMount,
      options,
    }: {
      value: string;
      beforeMount?: (monaco: typeof mocks.monaco) => void;
      onMount?: (
        editor: { getModel: () => editor.ITextModel },
        monaco: typeof mocks.monaco,
      ) => void;
      options?: { ariaLabel?: string };
    }) {
      mocks.editorOptions.push(options ?? {});
      useEffect(() => {
        const model = {
          getLineContent: () => (value === MENTION_EDITOR_VALUE ? "@" : "{{"),
        } as unknown as editor.ITextModel;
        const mount = () => {
          mocks.mountedModels.set(value, model);
          beforeMount?.(mocks.monaco);
          onMount?.({ getModel: () => model }, mocks.monaco);
        };
        if (mocks.deferMount) mocks.pendingMounts.push(mount);
        else mount();
      }, [beforeMount, onMount, value]);
      return null;
    },
}));

import { ScriptEditor } from "./script-editor";

function complete(
  provider: languages.CompletionItemProvider,
  model: editor.ITextModel,
): languages.CompletionList {
  return provider.provideCompletionItems(
    model,
    { lineNumber: 1, column: 3 } as unknown as Position,
    {} as languages.CompletionContext,
    {} as CancellationToken,
  ) as languages.CompletionList;
}

describe("ScriptEditor completion ownership", () => {
  afterEach(() => {
    cleanup();
    mocks.registrations.length = 0;
    mocks.mountedModels.clear();
    mocks.pendingMounts.length = 0;
    mocks.editorOptions.length = 0;
    mocks.deferMount = false;
    vi.clearAllMocks();
  });

  it("passes the accessible name to Monaco", () => {
    render(<ScriptEditor value="" onChange={() => undefined} ariaLabel="Prompt editor" />);

    expect(mocks.editorOptions.at(-1)?.ariaLabel).toBe("Prompt editor");
  });

  it("keeps completion providers scoped to the Monaco model that mounted them", async () => {
    render(
      <>
        <ScriptEditor
          value="first"
          onChange={() => undefined}
          placeholders={[
            { key: "first", description: "First", example: "one", executor_types: [] },
          ]}
        />
        <ScriptEditor
          value="second"
          onChange={() => undefined}
          placeholders={[
            { key: "second", description: "Second", example: "two", executor_types: [] },
          ]}
        />
      </>,
    );

    const activeRegistrations = () =>
      mocks.registrations.filter((registration) => registration.dispose.mock.calls.length === 0);
    await waitFor(() => expect(activeRegistrations()).toHaveLength(2));

    const firstModel = mocks.mountedModels.get("first");
    const secondModel = mocks.mountedModels.get("second");
    const [firstProvider, secondProvider] = activeRegistrations().map(
      (registration) => registration.provider,
    );

    expect(firstModel).toBeDefined();
    expect(secondModel).toBeDefined();
    expect(complete(firstProvider, firstModel!).suggestions[0].insertText).toBe("first}}");
    expect(complete(secondProvider, secondModel!).suggestions[0].insertText).toBe("second}}");
    expect(complete(firstProvider, secondModel!).suggestions).toEqual([]);
    expect(complete(secondProvider, firstModel!).suggestions).toEqual([]);
  });

  it("updates one editor without disposing the other editor's provider", async () => {
    const firstPlaceholders = [
      { key: "first", description: "First", example: "one", executor_types: [] },
    ];
    const secondPlaceholders = [
      { key: "second", description: "Second", example: "two", executor_types: [] },
    ];
    const view = render(
      <>
        <ScriptEditor value="first" onChange={() => undefined} placeholders={firstPlaceholders} />
        <ScriptEditor value="second" onChange={() => undefined} placeholders={secondPlaceholders} />
      </>,
    );

    const activeRegistrations = () =>
      mocks.registrations.filter((registration) => registration.dispose.mock.calls.length === 0);
    await waitFor(() => expect(activeRegistrations()).toHaveLength(2));
    const secondRegistration = activeRegistrations()[1];

    view.rerender(
      <>
        <ScriptEditor
          value="first"
          onChange={() => undefined}
          placeholders={[
            { key: "updated", description: "Updated", example: "three", executor_types: [] },
          ]}
        />
        <ScriptEditor value="second" onChange={() => undefined} placeholders={secondPlaceholders} />
      </>,
    );

    await waitFor(() => expect(activeRegistrations()).toHaveLength(2));
    expect(secondRegistration.dispose).not.toHaveBeenCalled();
  });
});

describe("ScriptEditor async completion ownership", () => {
  it("registers prompt mentions that load before Monaco mounts", async () => {
    mocks.deferMount = true;
    const prompt = { id: "prompt-1", name: "review", content: "Review this carefully." };
    const view = render(
      <ScriptEditor value={MENTION_EDITOR_VALUE} onChange={() => undefined} mentionPrompts={[]} />,
    );

    await waitFor(() => expect(mocks.pendingMounts).toHaveLength(1));
    view.rerender(
      <ScriptEditor
        value={MENTION_EDITOR_VALUE}
        onChange={() => undefined}
        mentionPrompts={[prompt]}
      />,
    );
    mocks.deferMount = false;
    mocks.pendingMounts.shift()?.();

    const activeRegistrations = () =>
      mocks.registrations.filter((registration) => registration.dispose.mock.calls.length === 0);
    await waitFor(() => expect(activeRegistrations()).toHaveLength(1));

    const model = mocks.mountedModels.get(MENTION_EDITOR_VALUE);
    const result = activeRegistrations()[0].provider.provideCompletionItems(
      model!,
      { lineNumber: 1, column: 2 } as unknown as Position,
      {} as languages.CompletionContext,
      {} as CancellationToken,
    ) as languages.CompletionList;
    expect(result.suggestions[0].label).toBe("@review");
  });
});
