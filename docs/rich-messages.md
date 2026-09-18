# Rich messages

Telegram lets a bot send a **rich message**: a block document — headings,
tables, collapsible sections, photo grids, maps, inline buttons — in place of
plain text. `tele` reads `richMessage` over MTProto and draws it as terminal
content. This page says what is drawn, what is deliberately simplified, and
which settings and keys are involved.

The reference material under `reference/rich-messages-spec.md` and
`skills/tg-rich-messages/SKILL.md` describes the **Bot API's** JSON schema, which
is the sending side over HTTP. What arrives here is a different wire format
(`PageBlock` constructors), so only the *set* of block types, button styles and
limits comes from there.

## Turning it on and off

`rich_messages.enabled` (default `true`) is a rendering switch and nothing more:

```yaml
rich_messages:
  enabled: false
```

Blocks and inline keyboards are parsed and stored either way. With the setting
off, a rich message is drawn from its flattened text — the plain rendering
Telegram sends alongside the document — and a message's inline keyboard is not
drawn. Switching it off and back on is a repaint: no re-fetch, nothing lost.
The switch is live, so it takes effect on the next frame after the config is
reloaded.

## Coverage

| Block (`PageBlock` constructor) | How it is drawn |
| ------------------------------- | --------------- |
| `pageBlockParagraph` | Body text, wrapped. |
| `pageBlockHeading1`…`6` | Bold, with the emphasis signalling the level: 1–2 bold+underline, 3–4 bold, 5–6 bold+dim. A terminal has no font size. |
| `pageBlockPreformatted` | The code surface, with the language as a ``` fence label. |
| `pageBlockFooter` | Dim text. |
| `pageBlockDivider` | A horizontal rule across the bubble. |
| `pageBlockAnchor` | **Nothing.** An anchor is a link target, not content; drawing a placeholder would put a line in the message nobody wrote. |
| `pageBlockKicker` | Dim, upper-cased (the "caps label" look). |
| `pageBlockTitle`, `pageBlockSubtitle` | The legacy masthead: title bold+underline, subtitle dim. |
| `pageBlockHeader`, `pageBlockSubheader` | The legacy headings, drawn as `heading` levels 2 and 3. |
| `pageBlockAuthorDate` | `Author · YYYY-MM-DD`. |
| `pageBlockBlockquote`, `pageBlockBlockquoteBlocks` | Left bar, indented body, `— credit` under it. Nested blocks hang under the same bar. |
| `pageBlockPullquote` | Centred, italic, `— credit` under it. |
| `pageBlockList` | Hanging `-` markers, `[ ]`/`[x]` when the item has a checkbox. An item's content may be text or a nested block tree. |
| `pageBlockOrderedList` | The server's own rendered marker (`3.`, `c.`) when it sent one; otherwise `1.`, `a.`, `A.`, `i.`, `I.` from `type` and `start`. |
| `pageBlockTable` | A text grid: columns sized to their content, cells wrapped, header row bold with a rule under it, `align`/`valign` honoured. |
| `pageBlockDetails` | `▸ summary` collapsed, `▾ summary` open, with the body indented below. |
| `pageBlockCollage`, `pageBlockSlideshow` | One grid of preview tiles, using the same tile geometry as an album, plus a caption. |
| `pageBlockPhoto`, `pageBlockVideo`, `pageBlockAudio` | The same inline image path a message's media uses (photo thumbnail, video/GIF thumbnail), with a placeholder before the bytes arrive, plus caption and credit. |
| `pageBlockMap` | Coordinates, zoom and caption, plus a link button to open the place in a browser. |
| `pageBlockMath` | The raw expression behind `∑ `, on the code surface. |
| `pageBlockThinking` | The streaming-draft placeholder (draft only). |
| `pageBlockUnsupported` and anything else | `[unsupported: <tl type name>]` — a **named placeholder**, never a silent gap. |

Inline text (`RichText`) is flattened to a string plus UTF-16 entities, the same
shape a plain message's body already has, so one renderer paints both. That
covers bold, italic, underline, strike, `code`, links, email/phone, mentions,
hashtags, cashtags and bot commands. **Subscript and superscript have no
terminal attribute**: their text is kept and the distinction is lost.
**Spoilers** are shown as ordinary text rather than hidden. Custom emoji are
drawn as their fallback glyph.

## Inline buttons

A message's inline keyboard is drawn inside its bubble under the content, with
the bot's own row structure: the arrangement is part of what the keyboard means,
so rows are never re-flowed. Button emphasis (`primary`, `success`, `danger`)
becomes a background fill in the theme's accent / success / danger colour; the
default is the selection colour.

| Action | What it does |
| ------ | ------------ |
| `callback` | Pressed through `messages.getBotCallbackAnswer`; the bot's answer is shown as a toast, and any keyboard or document it rewrites arrives later as an ordinary edit. |
| `url` | Opened in the system browser. Nothing travels to the bot. |

Keys, in the chat pane:

| Key | Action |
| --- | ------ |
| `B` | Focus the selected message's inline keyboard, or leave it again. Refused with a toast when the message has none. |
| `Tab` / `Shift+Tab` | Move to the next / previous button. Wraps at both ends: a keyboard is a small closed set, and stopping at the edge would make the last button a dead end. |
| `Enter` | Activate the focused button. |
| `Esc` | Leave the keyboard (the chat stays open). |
| `v` | Expand or collapse the selected message's collapsible section. |

While a keyboard has focus, only those keys are claimed: the message list keeps
`j`/`k`, scrolling and every other binding, because the mode is a cursor over
buttons rather than a second pane.

## Streaming drafts

A bot can stream a rich message while it writes, via a rich-message *draft*.
Telegram carries it as a typing action (`sendMessageRichMessageDraftAction`)
rather than as a message, which is why a draft has no history and why it expires
by itself: **30 seconds** after the last revision.

A draft is drawn as an overlay under the history of the open chat, on the toast
surface with a `· generating…` marker, bounded to a third of the pane. It is
never part of the message list, never stored and never addressable — it cannot
be selected, replied to or scrolled past, because it is not a message. A
revision replaces the one on screen; the stream ends when the bot cancels it
(the same 30-second expiry, or a `sendMessageCancelAction`). The finished
article arrives afterwards as an ordinary message.

A draft whose document is only the `thinking` block is shown as the placeholder,
which is what that block means.

## Deliberate simplifications

These are choices, not gaps. Each is where a terminal cannot honestly do what
the block says.

- **No LaTeX.** `pageBlockMath` shows the raw expression behind `∑ `. There is
  no LaTeX engine here, and rendering it as anything else would be inventing a
  reading of it.
- **Maps are coordinates.** A map needs tiles and a graphics protocol;
  `pageBlockMap` shows `lat, long · zoom N`, its caption, and a link button to
  `maps.google.com`. (The link is rendered like any other button but is not
  drawn as a real `ButtonActionURL` block — the block itself has no buttons.)
- **A slideshow is a grid.** `pageBlockSlideshow` draws exactly like
  `pageBlockCollage`. A terminal cannot swipe, and paginating a deck behind a
  key that exists only inside one block would be a mode nobody discovers.
- **`colspan` is merged, `rowspan` is not laid out.** A cell with `colspan` > 1
  is drawn as one cell as wide as the columns it covers. `rowspan` is carried
  (it round-trips through storage) but the cell occupies only its own row, whose
  height is already that of its tallest cell. A table wider than the pane
  shrinks its columns rather than overflowing.
- **At most 8 table columns.** More than that is unreadable at any terminal
  width; the extra columns are dropped and the fact is stated under the table.
- **An ordered list item's explicit `value` override is not read.** Telegram
  lets a bot jump an item to an arbitrary number (`value`) independently of
  the server-rendered marker string (`num`). `tele` only reads `num`; an item
  that sets `value` without also sending a pre-rendered `num` falls back to
  the running counter instead of jumping. Rare in practice — bots that use
  `value` typically also send `num` — but a real, known gap rather than a
  choice, unlike the rest of this section.
- **A terminal has no font size**, so heading levels are conveyed by attributes
  (see the coverage table).
- **An inline image inside a paragraph** is drawn as `[image]`, and a custom
  emoji as its fallback glyph.
- **Collapsible sections reset on restart.** Expansion is view state keyed by
  message and block path, kept in memory only: reopening `tele` returns every
  section to the server's `is_open`. Persisting it would mean storing a decision
  nobody remembers making, for a document that may have been edited since.
- **`alert` answers are toasts.** Telegram's own clients put a callback answer
  with `show_alert` in a modal the reader dismisses. There is no modal in this
  client, so it is shown as a warning toast (longer-lived than an informational
  one), which is the closest thing available.

## Unsupported button actions

Telegram offers buttons that need things a terminal has not got: the phone's
contact list, GPS, an in-app browser, a payment sheet, an inline-mode switch.
These are **drawn, disabled, with a one-line explanation under the keyboard**
(`⚠ Send phone: asks for your phone number`), never dropped: a keyboard with a
hole in it misdescribes what the bot offered, and a press on one reports the
same reason in a toast.

Covered here: `keyboardButtonRequestPhone`, `RequestGeoLocation`, `RequestPoll`,
`Game`, `Buy`, `SwitchInline`, `WebView`, `SimpleWebView`, `URLAuth`,
`RequestPeer`, `UserProfile`, `Copy`.

`ReplyKeyboardMarkup`, `ReplyKeyboardForceReply` and `ReplyKeyboardHide` — a bot
replacing the phone's own keyboard — are not read at all: they are not a rich
message's buttons and this client draws no such keyboard.

## Not drawn from Instant View articles

`pageBlockEmbed`, `pageBlockEmbedPost`, `pageBlockRelatedArticles` and
`pageBlockChannel` come from Instant View article markup rather than from a chat
message. They are reported as `[unsupported: <type>]` like any other
constructor this client has no rendering for.

## Manual QA

Verifying the whole path end to end needs a bot that actually sends a rich
message (the public `@RichTextDemoBot` mentioned in the `serejaris/telegram-skills`
README is one). That cannot be automated without a bot token, so it is a manual
step: open the bot's chat and check the headings, a table, a collage, a details
block, the inline buttons and a streamed draft. Unit tests cover everything up
to the wire.

## Where it lives

| Concern | Package |
| ------- | ------- |
| `richMessage` → domain blocks, inline keyboard | `internal/tg/rich.go` |
| The data model (`PageBlock`, `KeyboardButton`, `EphemeralDraft`) | `internal/domain/model.go` |
| Pressing a button (`getBotCallbackAnswer`) | `internal/core/cmd_message.go`, `internal/tg/messages.go` |
| Editing a message's blocks and keyboard | `internal/core/state/messages.go` |
| Streaming drafts (TTL, per-chat stream) | `internal/core/ephemeral.go` |
| Block rendering | `internal/ui/components/messagelist_richblocks.go` |
| Tables | `internal/ui/components/messagelist_richtable.go` |
| Inline buttons, keyboard focus mode | `internal/ui/components/messagelist_buttons.go` |
| Collapsible-section state | `internal/ui/components/messagelist_details.go` |
| The streaming-draft overlay | `internal/ui/components/ephemeral.go` |
| Key handling | `internal/ui/root_keys.go`, `internal/ui/root_msg_ops.go` |
