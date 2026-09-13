# Confirmation surfaces

Phone confirmation adapters use `mobile-action-confirmation.tsx` with an explicit
localized title/target and a stable owner outside transient menus. Keep the
responsive adapter mounted with its non-phone `fallback` so boundary changes
cancel pending decisions.

Opt existing drawers into `MobileConfirmationHost`/`MobileConfirmationHostBody`
(pickers: `confirmationHost`); use `surface="drawer"` and spread its
`contentProps` on the owning surface. Active drawer steps fit the confirmation
content; the hidden origin keeps its measured dimensions outside layout so
Cancel can restore its normal size and scroll. Do not override the host's
active sizing with a fixed-height style. Centered dialog hosts omit `surface`.
Drawer roots use `overflow: clip` so focus cannot scroll the outer shell; only
the inner list or confirmation body owns scrolling. Initial Cancel focus uses
`preventScroll` to keep the title and actions in view.
A hosted step keeps the origin mounted/inert and opens no second modal. Form
dialog hosts also disable `enterConfirms` while the step is active.

Domain callbacks still own transport and error behavior. Ordinary actions close
before dispatch. Retryable form owners explicitly use
`completionPolicy="await-with-retry"` and reject on failure; this keeps their
confirmation busy during the request and available for retry afterward.
