/**
 * Signal to iOS / password managers that xterm's helper textarea isn't a
 * fillable form field, so the AutoFill suggestion bar above the keyboard
 * doesn't appear.
 *
 * xterm already sets autocorrect/autocapitalize/spellcheck itself, but that
 * isn't enough — iOS still heuristically shows the AutoFill bar for generic
 * textareas. Adding `autocomplete="off"`, an opaque `name`, and the password-
 * manager ignore hints (1Password, LastPass, Bitwarden) makes iOS and those
 * extensions skip the field. Safe no-op on desktop.
 */
export function suppressIOSKeyboardAssists(host: HTMLElement): void {
  const textarea = host.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea");
  if (!textarea) return;
  textarea.setAttribute("autocomplete", "off");
  textarea.setAttribute("name", "__xterm_stdin__");
  textarea.setAttribute("data-form-type", "other");
  textarea.setAttribute("data-lpignore", "true");
  textarea.setAttribute("data-1p-ignore", "true");
  textarea.setAttribute("data-bwignore", "true");
}
