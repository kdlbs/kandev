# Confirmation surfaces

Phone confirmation adapters use `mobile-action-confirmation.tsx` with an explicit
localized title/target and a stable owner outside transient menus. Keep the
responsive adapter mounted with its non-phone `fallback` so boundary changes
cancel pending decisions.

Opt existing drawers into `MobileConfirmationHost`/`MobileConfirmationHostBody`
(pickers: `confirmationHost`); spread its `contentProps` on the owning surface.
A hosted step keeps the origin mounted/inert and opens no second modal. Form
dialog hosts also disable `enterConfirms` while the step is active.

Domain callbacks still own transport and error behavior. Ordinary actions close
before dispatch. Retryable form owners explicitly use
`completionPolicy="await-with-retry"` and reject on failure; this keeps their
confirmation busy during the request and available for retry afterward.
