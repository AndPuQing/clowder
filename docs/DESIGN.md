# Clowder interface guidelines

Write the spec before the interface. This document is the result of
checking the design against Anthropic's `frontend-design` notes and the
`claude-web-skills` "AI tells: the refuse list".

## Reading

> A tool that hands "a string that expires" to another person. Quiet,
> trustworthy, with a little hand-made quality.

Three dials (set by the user: a little artistic licence is welcome):

| Dial | Value | Meaning |
|---|---|---|
| DESIGN_VARIANCE | 8 | Asymmetric; generous whitespace and unusual proportions allowed |
| MOTION_INTENSITY | 3 | State feedback only, no decorative animation |
| VISUAL_DENSITY | 3 | Sparse: one idea per screen |

## Hard constraints

- One window, 1000×680 (minimum 880×600)
- English is the source language; a Simplified Chinese translation ships with it
- The address must be readable and copyable; the QR code must scan (always on white)
- Only MyGo native primitives: colour blocks, rounded corners, circles, hairlines, text, bitmaps
- One memorable element per screen; everything else steps back

## Off limits (the refuse list)

| Not used | Why |
|---|---|
| `A · B · C` middle-dot strings | The most obvious machine tell |
| All-caps letter-spaced labels (CLOWDER / ADDRESS) | A cliché on every screen |
| Card sets sharing one radius and one shadow | SaaS card kit |
| Neon outlines, outer glows, gradient text | Oversaturated |
| Monospace as decoration (ports, labels) | Monospace only for the address itself, because that is real data |
| Numbered step bars (01 / 02) | This is not a flow |
| Centred hero plus dark grid gradient | The default option |

## The three candidates

### A. Poster

Typography is the visual. The address becomes the body copy, sized close
to a poster headline; the QR code shrinks to stamp size and retreats to
the bottom right.

- Colours: ink `#101113`, paper `#FAFAF7`, vermilion `#C03B25` (not
  Anthropic's terracotta)
- Type: body Noto Sans CJK SC; address Noto Sans Mono CJK SC
- Layout: left-aligned 44px title → three lines of address → 120px QR
  bottom right with a vermilion hairline
- Principle: put all the boldness into size contrast, keep the corners
  empty, add no ornament

### B. Diagram

Draw the tunnel: two nodes, one line, the address hanging on the line.

- Colours: ground `#F4F5F3`, ink `#15181A`, teal `#0F6E63`
- Layout: a horizontal axis joining two dots, "this machine" on the left
  and "peer" on the right, the QR code above the left node, the address
  along the line
- Principle: geometry first, type second; one accent colour, used once

### C. Ticket (visual metaphor only)

The address is a ticket you can hand over. Only the shape is borrowed;
the copy never uses ticket-office vocabulary.

- Colours: deep green `#12221E`, cream `#F2EDE3`, gold `#C9A227`
- Layout: a vertical ticket in the centre, QR code in the upper half, a
  dashed tear line, the address below; a small "ephemeral" stamp in the
  bottom right
- Principle: material and metaphor carry the character; no stacked shadows

## Acceptance

After rendering, check these four:

1. Squint: can you say what the main subject is at a glance?
2. Is anything purely decorative?
3. Would this interface still work under another product name? (If yes,
   the character is missing.)
4. Did anything from the refuse list sneak in?
5. Does anything explain the interface instead of showing it? Remove it:
   no helper sentences, no instructions, no reassurance.

## Rules

[`docs/UI.md`](UI.md) holds the binding rules the interface is built and
reviewed against. The most important one:

> The interface does not explain itself.

Labels name things and actions; state is a value; there are no helper
sentences, no instructions, and no reassurance. Icons are SVG files, never
emoji or typographic glyphs.

## Current implementation

The shipped interface uses candidate C's palette (deep green, cream,
gold), now stepped into three neutral surfaces with hairlines and a soft
shadow instead of one flat plane. The structure is a desktop tool's:

- a 252 px sidebar: a gold mark and wordmark, `Machines` and its rows with
  a status dot, then the create and settings actions;
- a detail page that starts with a 44 px icon tile, an editable machine
  name and a status chip, with the page's primary action on the right;
- grouped cards for ports, options, mapping, files and the terminal, each
  with a single raised surface and a hairline between rows;
- one gold accent for the primary action and status dots; green and red only
  for state.

All icons live in `ui/icons/` and are drawn with `ui.Icon`, in the colour of
the text beside them, so they stay sharp at any scale and work on every
surface.
