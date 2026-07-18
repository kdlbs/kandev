import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { EditorForm, defaultFormState } from "./editor-form";
import { SettingsPageTemplate } from "./settings-page-template";
import { SettingsSaveProvider } from "./settings-save-provider";

afterEach(cleanup);

const COMMAND_PLACEHOLDER = "code --goto {file}:{line}";
const DIRTY_ATTRIBUTE_VALUE = "true";
const DIRTY_ATTRIBUTE = "data-settings-dirty";

function renderEditorForm(props: Partial<React.ComponentProps<typeof EditorForm>> = {}) {
  const onSave = props.onSave ?? vi.fn().mockResolvedValue(undefined);
  render(
    <SettingsSaveProvider>
      <EditorForm
        title="New custom editor"
        initialState={defaultFormState()}
        onCancel={vi.fn()}
        onSave={onSave}
        submitLabel="Add editor"
        isSaving={false}
        coordinatedSaveId="custom-editor:new"
        dirtyWhenMounted
        {...props}
      />
    </SettingsSaveProvider>,
  );
  return { onSave };
}

describe("EditorForm coordinated save", () => {
  it("treats a new inline editor as dirty and saves it through the route action", async () => {
    const { onSave } = renderEditorForm();

    const form = screen.getByText("New custom editor").parentElement;
    expect(form?.getAttribute(DIRTY_ATTRIBUTE)).toBe(DIRTY_ATTRIBUTE_VALUE);
    expect(screen.queryByRole("button", { name: "Add editor" })).toBeNull();
    expect(screen.getByRole("button", { name: "Save changes" }).hasAttribute("disabled")).toBe(
      true,
    );

    fireEvent.change(screen.getByPlaceholderText(COMMAND_PLACEHOLDER), {
      target: { value: COMMAND_PLACEHOLDER },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "code --goto {file}:{line}",
        command: COMMAND_PLACEHOLDER,
      }),
    );
  });

  it("marks the owning page card dirty when an existing editor changes", async () => {
    const initialState = {
      ...defaultFormState(),
      name: "VS Code",
      command: "code {file}",
    };
    render(
      <SettingsSaveProvider>
        <SettingsPageTemplate title="Editors" isDirty={false} saveStatus="idle" onSave={vi.fn()}>
          <EditorForm
            title="Edit VS Code"
            initialState={initialState}
            onCancel={vi.fn()}
            onSave={vi.fn()}
            submitLabel="Save editor"
            isSaving={false}
            coordinatedSaveId="custom-editor:editor-1"
          />
        </SettingsPageTemplate>
      </SettingsSaveProvider>,
    );

    const pageCard = document.querySelector('[data-settings-dirty-level="card"]');
    expect(pageCard?.getAttribute(DIRTY_ATTRIBUTE)).toBe("false");
    fireEvent.change(screen.getByPlaceholderText(COMMAND_PLACEHOLDER), {
      target: { value: "cursor {file}" },
    });

    await waitFor(() =>
      expect(pageCard?.getAttribute(DIRTY_ATTRIBUTE)).toBe(DIRTY_ATTRIBUTE_VALUE),
    );
    expect(screen.getByText("Edit VS Code").parentElement?.getAttribute(DIRTY_ATTRIBUTE)).toBe(
      DIRTY_ATTRIBUTE_VALUE,
    );
  });
});
