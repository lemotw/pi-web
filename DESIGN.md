# Follow Button Reposition — Design Doc

## Problem
The "follow" / "new messages" button currently appears as a `position: fixed` element at the bottom-right of the viewport. The user wants it moved to the **top of the chatbox** (inside the scrollable message area) so it sits just above the messages and is more contextually placed.

## Proposed Approach: Sticky Button Inside Messages Container

Replace the fixed-position button with a `position: sticky; top: 0` button bar that lives as the **first child** of `#messages` (or is injected just before the first message). It uses CSS transitions for show/hide instead of opacity on a fixed element.

### Why this approach?
- **Contextual placement**: The button naturally sits at the top edge of the message scroll area, exactly where the user's eyes are when reading scrolled-up history.
- **No viewport collision**: Doesn't overlap composer controls or other bottom-right UI.
- **Clean hide/show**: Can use `transform + opacity` transitions without fighting `position: fixed` stacking.

### Trade-offs vs. alternatives
| Approach | Pros | Cons |
|----------|------|------|
| **A. Sticky inside #messages** (recommended) | Contextual, clean CSS, no viewport overlap | Requires a small CSS addition to `export/template.css` |
| **B. Fixed but moved to top-right** | Minimal code change | Still detached from chat context, overlaps header |
| **C. Absolutely positioned inside scroll container** | Closer to chat | Breaks on nested scroll containers, harder to maintain |

## Files to Modify

1. **`web/src/session/live/live-scroll.js`**
   - `createFollowButton` → change from `position: fixed; bottom: 20px; right: 20px` on `document.body` to creating a wrapper inside `#messages` with `position: sticky; top: 0`.
   - `removeFollowButton` → remove the wrapper from `#messages` instead of fading out a fixed button.
   - `setFollowButtonText` → keep signature, target the text node inside the sticky wrapper.

2. **`web/src/session/live/live-reload-runner.js`**
   - `showFollowButton` / `hideFollowButton` — logic stays the same (track `followBtn`, `pendingCount`), just call the updated `liveScroll` helpers.

3. **`web/src/session/live/live-scroll.test.js`**
   - Update test DOM to include `#messages`.
   - Assert the button is appended **inside `#messages`**, not `document.body`.
   - Assert the wrapper has the expected sticky CSS class.

4. **`export/template.css`**
   - Add `.follow-bar` and `.follow-btn` rules for the sticky positioning, transitions, and theming.

5. **`live_templates/live_reload.js`**
   - Update the inlined IIFE versions of the same functions to match the new behavior (this file is manually kept in sync with the ES module sources).

## Test Plan (TDD)
1. Write a failing test: `createFollowButton` appends to `#messages` and uses sticky positioning.
2. Write a failing test: `removeFollowButton` removes the wrapper from `#messages`.
3. Implement the changes.
4. Run `cd web && npm test` — all `live-scroll.test.js` and dependent tests should pass.
5. Run `go test ./...` — Go embed tests should still pass (after updating `live_reload.js`).

## Open Questions
1. **Should we completely remove the old fixed-position code, or keep it as a commented fallback?**
   - Recommendation: Remove it entirely. The old code is simple and can be recovered from git history.

2. **Should the button text stay as `"↓ 3 news"` or change to something else for the new position?**
   - Recommendation: Keep the same text format for consistency, but we can easily adjust.

3. **The `live_templates/live_reload.js` is manually maintained. Should we update it in this PR or is it auto-generated elsewhere?**
   - It is manually kept in sync. We will update it in this PR.

---
**Awaiting user approval before proceeding to implementation.**
