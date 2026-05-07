# Homepage Running Session Border Design

## Goal
Make running sessions on the homepage visually obvious by showing an animated dashed border around the full session card.

## Scope
Only change the homepage session list UI at `/`. Do not change the session detail page, backend status semantics, or non-running card states.

## Proposed Behavior
- When a session is in `running` state, its card gets a dashed animated border.
- The animation should feel like a lightweight loading indicator (“marching ants”), not a spinner or pulse.
- The effect applies to the whole session card.
- The effect is border-only: no glow, no background tint, no content movement.
- When the session leaves `running`, the card returns to its normal appearance.
- If live status cannot be determined, the card should fall back to the normal non-running style.

## UI Design
### Recommended approach: pseudo-element overlay
Use a `session-card--running` modifier class and render the animated border with a `::before` pseudo-element.

Why this approach:
- preserves the existing base border and hover behavior,
- avoids layout shifts,
- gives better control over rounded corners, inset spacing, and animation styling,
- keeps the running treatment isolated from the default card styles.

### Visual details
- dashed border color: warm red/orange accent similar to the user mockup,
- rounded corners matching the existing card radius,
- slight inset so the animated border sits cleanly inside the card,
- slow, subtle motion to avoid visual noise when multiple sessions are running.

## Data / State Flow
The homepage needs to know which sessions are currently running.

Recommended implementation shape:
- extend the homepage data model to track running state per session id,
- populate initial state from existing rendered markup or a lightweight fetch path,
- subscribe to live updates so cards can enter/leave running state without a full page reload,
- toggle the `session-card--running` class based on that state.

If the current homepage already has enough live events to infer running state, reuse them. Otherwise add the smallest possible client-side status refresh path needed to keep the indicator accurate.

## Accessibility and UX
- Running state must not rely only on hover.
- Animation should be subtle enough not to distract from scanning the list.
- Keep the effect purely decorative; card click targets and keyboard behavior must remain unchanged.
- Respect reduced-motion preferences by disabling or minimizing the animation under `prefers-reduced-motion`.

## Error Handling
- Unknown or unavailable status should not show a running border.
- A stale running style must be cleared when the client learns the session is idle or errored.

## Testing
Add or update frontend tests to cover:
- running class application when a session is marked running,
- running class removal when status becomes non-running,
- no false positive running class on unknown sessions,
- reduced-motion fallback if implemented in a testable way.

## Out of Scope
- glow effects,
- background tint changes,
- changes to card layout/content,
- non-homepage running indicators.
