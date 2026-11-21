# Interface principles

These rules decide how Clowder looks and speaks. They are constraints, not
suggestions: a change that breaks one of them is wrong even if it looks
better in a screenshot.

## The interface does not explain itself

The interface shows a machine and what it is doing. It does not narrate.

- A label names a thing or an action: `Ports`, `Share`, `Connected`. It never
  describes what the app is about to do.
- State is a value, not a sentence. Show `Not shared`, `Connecting…`,
  `Connected`, a round-trip time. Do not write "You are connected".
- No helper sentence under a control. If a switch needs a paragraph to be
  understood, the switch is wrong, or the concept is.
- Do not fill empty space with reassurance. Empty space is allowed.
- No "click here", "press Share to…", "this will…", "the other side must…".
- Anything that genuinely needs explanation belongs in the README, a
  tooltip, or nowhere. Not on the page.

The one exception is a failure: an error may state what failed and why, since
that is data the user cannot infer.

## Icons

- Icons are SVG files (`ui/icons/*.svg`), parsed once and drawn as vectors.
- No emoji. No typographic glyphs (`⚙`, `▸`, `✓`, `ⓘ`). No icon fonts.
- One grid: 24×24, 1.7 px stroke, round caps, `currentColor`. A filled mark is
  the exception, not the rule.
- An icon labels an action or a machine. It never decorates a heading.
- Pair an icon with a word in buttons and rows; use an icon alone only for a
  universal action (copy, refresh, download) and give it an accessible label.

## Surfaces and depth

- Three surface values: window, card, raised control. A card is one step
  above the window, a control one step above the card.
- Hierarchy comes from value, a 1 px hairline and one soft shadow — not from
  gradients used as decoration. A gradient may describe a surface (top lit,
  bottom in shadow), never a mood.
- One accent (gold) and two semantic colours (green, red). Nothing else is
  coloured.
- Corners are small and consistent: 8 px controls, 12 px cards.
- Density is a desktop tool's, not a landing page's: grouped rows, aligned
  controls, no oversized empty bands.

## Actions

- One primary action per page, in the accent. Everything else is quiet.
- Destructive actions are quiet until hovered; they never compete with the
  primary action.
- Button labels are verbs: `Share`, `Connect`, `Copy`, `Remove`. Not
  sentences, not `OK`.
- The same action uses the same icon everywhere.

## Reference

The desktop app of a network tool is the closest reference (Tailscale's
desktop client is the example):

- a dense sidebar of machines with a status dot and a one-line state;
- vector, monochrome icons at a single size;
- a detail pane built from a few grouped surfaces, not cards inside cards;
- the important value (the address, the connection state) is the only thing
  that gets weight; metadata stays muted;
- no illustration, no mascot on the working page, no decorative copy.

macOS System Settings and the Windows 11 settings app are the reference for
grouped form rows: label on the left, control on the right, a hairline
between rows.

## Small rules that compound

These are the details that make the difference between "fine" and considered.
They come from the UI cheatsheet in [`emilkowalski/skills`](https://github.com/emilkowalski/skills),
applied to a native desktop app.

- Numbers that change while you look at them use tabular figures
  (`FontFeatures("tnum")`, or a monospaced face): latency, file sizes, ports.
- Large headings get tighter tracking (about `-0.02em`); body text does not.
- A pressed control shrinks slightly (`Scale(0.97)`); nothing else moves.
- Counts are pluralised through ICU (`{count, plural, one {# machine} other {# machines}}`),
  never "1 machines".
- Names that can be long truncate instead of overflowing: device names in the
  middle, file names at the start so the extension stays visible.
- Dates older than a week are absolute, not "340 days ago".
- Body copy is capped at a readable measure; a label that needs more is a
  paragraph, and belongs elsewhere.
- Do not animate `all`; list the properties. Do not animate layout in place.
