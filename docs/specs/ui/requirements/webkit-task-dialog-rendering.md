---
status: active
system: ui
created: 2026-08-01
owners:
  - product
---
# WebKit Task Dialog Rendering Requirements

## Overview

People opening the Create Task dialog in Safari or a WebKit-backed Tauri app see softer, lower-clarity text and controls than people opening the same Kandev surface in Chromium. The dialog must remain visually sharp without removing the existing Chromium motion that already renders correctly.

## Requirements

### REQ-UI-WEBKIT-TASK-DIALOG-RENDERING-001: WebKit Task Dialog Rendering

**Intent:** People opening the Create Task dialog in Safari or a WebKit-backed Tauri app see softer, lower-clarity text and controls than people opening the same Kandev surface in Chromium. The dialog must remain visually sharp without removing the existing Chromium motion that already renders correctly.

#### Acceptance criteria

- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.1:** Kandev identifies actual WebKit runtimes before the application renders and exposes that result as a non-persistent document rendering-engine marker.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.2:** The Create Task dialog uses opacity-only open and close motion in WebKit; the animation does not apply a transform to the text-bearing dialog surface.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.3:** The Create Task dialog is centered without a transform in WebKit and remains above its overlay.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.4:** Safari, macOS WKWebView, WebKitGTK, and browsers constrained to WebKit on iOS receive the WebKit rendering path.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.5:** Desktop Chromium, Android Chromium, Edge/WebView2, Firefox, and other non-WebKit engines retain the existing Create Task dialog animation and positioning.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.6:** The WebKit rendering path preserves the existing task-creation form, focus behavior, dismissal, keyboard behavior, dimensions, internal scrolling, responsive full-height phone presentation, and safe-area behavior.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.7:** The Create Task dialog omits the generic top-right close control in every browser; the footer Cancel action remains available for dismissal.
- **AC-UI-WEBKIT-TASK-DIALOG-RENDERING-001.8:** The dialog does not reserve top padding for the removed control while retaining its side and bottom spacing.

## Migrated source detail

## Why

People opening the Create Task dialog in Safari or a WebKit-backed Tauri app see softer,
lower-clarity text and controls than people opening the same Kandev surface in Chromium. The
dialog must remain visually sharp without removing the existing Chromium motion that already
renders correctly.

## What

- Kandev identifies actual WebKit runtimes before the application renders and exposes that result
  as a non-persistent document rendering-engine marker.
- The Create Task dialog uses opacity-only open and close motion in WebKit; the animation does not
  apply a transform to the text-bearing dialog surface.
- The Create Task dialog is centered without a transform in WebKit and remains above its overlay.
- Safari, macOS WKWebView, WebKitGTK, and browsers constrained to WebKit on iOS receive the WebKit
  rendering path.
- Desktop Chromium, Android Chromium, Edge/WebView2, Firefox, and other non-WebKit engines retain
  the existing Create Task dialog animation and positioning.
- The WebKit rendering path preserves the existing task-creation form, focus behavior, dismissal,
  keyboard behavior, dimensions, internal scrolling, responsive full-height phone presentation,
  and safe-area behavior.
- The Create Task dialog omits the generic top-right close control in every browser; the footer
  Cancel action remains available for dismissal.
- The dialog does not reserve top padding for the removed control while retaining its side and
  bottom spacing.
- Rendering-engine classification is runtime-only and does not add a user setting or persisted
  preference.

## Failure modes

- If the runtime cannot be classified as WebKit, Kandev uses the existing default dialog rendering
  rather than applying the workaround broadly.
- User-agent compatibility tokens such as desktop Chromium's `AppleWebKit` token must not classify
  Blink as WebKit. iOS browser brands remain classified as WebKit because their underlying engine
  is WebKit.

## Scenarios

- **GIVEN** Kandev is running in Safari, WKWebView, or WebKitGTK, **WHEN** the user opens Create
  Task, **THEN** the dialog enters with opacity-only motion, has no transform on its text-bearing
  surface, remains centered, and renders above the overlay.
- **GIVEN** Kandev is running in desktop Chrome or Edge/WebView2, **WHEN** the user opens Create
  Task, **THEN** the existing scale-and-fade motion and centered geometry remain unchanged.
- **GIVEN** Kandev is running in an iOS browser, **WHEN** the user opens Create Task, **THEN** the
  WebKit rendering path is selected even when the browser has a non-Safari brand.
- **GIVEN** Create Task is open in a narrow WebKit viewport, **WHEN** the user edits or scrolls the
  form, **THEN** the dialog remains full-height, contained within the viewport, free of document
  horizontal overflow, and exposes the same task-creation controls.

## Out of scope

- Changing dialog motion in Chromium or Firefox.
- Applying the workaround to dialogs other than Create Task without a separate reproduced need.
- Replacing Tauri's system WebView, changing desktop zoom behavior, or modifying bundled fonts.
- Adding browser-specific font-smoothing, forced GPU compositing, or `will-change` workarounds.
