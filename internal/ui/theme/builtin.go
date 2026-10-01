package theme

import (
	"fmt"
	"image/color"
)

// The built-in themes. They are named after the app rather than called
// "default", because a theme name is a stable reference: base: tele-dark means
// this palette forever, whereas base: default-dark would have meant "whatever is
// currently default" and would have repainted the themes built on it the day
// that changed. "Default" and "fallback" describe the role these two play at
// the root of every inheritance chain, and are not names anything resolves by.
//
// The trailing comment on each value is the xterm index it came from. Indexes
// 16-255 convert losslessly; 0-15 were hand-picked, because the literal xterm
// values for those (#00ff00, #0000ff) are harsher than any real terminal
// palette renders them.

// builtins is the registry every name is resolved against before the themes
// directory is consulted.
var builtins = map[string]Theme{
	normalize("tele-dark"):  TeleDark,
	normalize("tele-light"): TeleLight,
}

// Builtin returns the compiled-in theme of that name.
func Builtin(name string) (Theme, bool) {
	t, ok := builtins[normalize(name)]
	return t, ok
}

// BuiltinNames lists the compiled-in theme names, for diagnostics.
func BuiltinNames() []string { return []string{"tele-dark", "tele-light"} }

// mustColor parses a literal written in this file. A malformed one is a
// programmer error and there is no sane value to continue with, so it fails at
// init rather than rendering as an invisible token.
func mustColor(s string) color.Color {
	c, err := ParseColor(s)
	if err != nil {
		panic(fmt.Sprintf("theme: built-in color %q: %v", s, err))
	}
	return c
}

func hex(s string) color.Color { return mustColor(s) }

var TeleDark = Theme{
	Name: "tele-dark",

	Background: hex("none"), // the terminal's own; see Theme.Background

	SurfaceOverlay:     hex("#262626"), // 235
	SurfaceHelp:        hex("#262626"), // 235
	SurfaceToast:       hex("#3a3a3a"), // 237
	SurfaceStatusBar:   hex("#303030"), // 236
	SurfaceSelected:    hex("#5f5fff"), // 63
	SurfaceSelfMention: hex("#ff87ff"), // 213
	SurfaceCode:        hex("#303030"), // 236

	Text:                hex("none"),    // the terminal's own foreground
	TextDim:             hex("#8a8a8a"), // 245
	TextMuted:           hex("#808080"), // 244
	TextFaint:           hex("#808080"), // 8
	TextSubtle:          hex("#8a8a8a"), // 245; 240 on the toast fill came to 1.6:1
	TextOnSurface:       hex("#bcbcbc"), // 250
	TextStatusBar:       hex("#eeeeee"),
	TextOnSelected:      hex("#ffffff"),
	TextOnSelectedMuted: hex("#e4e4e4"), // 254
	TextOnToast:         hex("#d0d0d0"), // 252
	TextModeLabel:       hex("#ffffff"), // 231
	TextCode:            hex("#d0d0d0"), // 252

	Accent:           hex("#00afff"), // 39
	AccentOnSurface:  hex("#00afff"), // 39
	AccentStatusBar:  hex("#00afff"), // 39
	AccentInsert:     hex("#00d700"), // 40
	AccentModeNormal: hex("#0087ff"), // 33
	AccentModeInsert: hex("#00a05f"), // 35 was too light to carry the white label

	StatusError:     hex("#ff5f5f"), // 203
	StatusWarning:   hex("#ffaf00"), // 214
	StatusInfo:      hex("#5fafff"), // 75
	StatusOnline:    hex("#5fd75f"), // 10
	TickSent:        hex("#8a8a8a"), // 245
	TickOutbox:      hex("#8a8a8a"), // 245
	TickRead:        hex("#5f87ff"), // 12
	TickFailed:      hex("#ff5f5f"), // 9
	NameIncoming:    hex("#5fd75f"), // 10
	NameEditing:     hex("#ffd75f"), // 11
	Indicator:       hex("#00afaf"), // 6
	UnreadSeparator: hex("#5f87ff"), // 12
	WaveformPlayed:  hex("#5f87ff"), // 12
	ReactionChosen:  hex("#5fd75f"), // 10
	UnreadReaction:  hex("#ff5faf"), // 205
	UnreadMention:   hex("#00afff"), // 39

	BorderPane:            hex("#444444"), // 238, recedes behind the content
	BorderPaneActive:      hex("#5fd75f"), // 10
	BorderBubbleIn:        hex("#444444"), // 238
	BorderBubbleOut:       hex("#005faf"), // 25
	BorderOverlay:         hex("#585858"), // 240
	BorderComposerFocused: hex("#00d700"), // 40
	BorderComposerFlash:   hex("#ff5f5f"), // 203
	BorderStatusSep:       hex("#585858"), // 240

	MarkupLink:          hex("#00d7ff"), // 45
	MarkupRef:           hex("#ff87ff"), // 213
	MarkupSelfMentionFg: hex("#000000"), // 0

	HighlightAccent:     hex("#ffaf00"),
	HighlightError:      hex("#ff5f5f"),
	HighlightBaseChat:   hex("#bcbcbc"), // 250
	HighlightBaseBubble: hex("#444444"), // 238
	OverlayDim:          hex("#585858"), // 240

	// The quietest things on screen, but still on screen: 240 on black came to
	// 2.95:1, under the floor the contrast test holds foregrounds to.
	ComposerCounterDim: hex("#626262"),
	ComposerGlyphIdle:  hex("#626262"),
	ComposerGlyphReady: hex("#00afff"), // 39

	SenderPalette: []color.Color{
		hex("#ff5f5f"), // 9
		hex("#5fd75f"), // 10
		hex("#5f87ff"), // 12
		hex("#ff5fff"), // 13
		hex("#5fffff"), // 14
		hex("#ff8700"), // 208
		hex("#af87ff"), // 141
		hex("#87ff00"), // 118
	},

	LogoGradient: []GradientStop{
		{Pos: 0.00, Color: hex("#3c5aa0")},
		{Pos: 0.30, Color: hex("#5a87d2")},
		{Pos: 0.60, Color: hex("#82aae6")},
		{Pos: 0.85, Color: hex("#afd2f8")},
		{Pos: 1.00, Color: hex("#d7eeff")},
	},
}

// TeleLight is tuned against a light terminal background. The rule it follows:
// a token that carries text is dark enough to read on white, and a token that
// only has to be noticed (a border, a fill) is not pushed further than it needs
// to be. Pale tints that look right on black — the greens, the light blues —
// vanish on white and are replaced rather than reused.
//
// The status bar stays dark on purpose. It is not an untuned leftover: a solid
// dark bar anchors the bottom of the screen against either background, and its
// text and separators are already chosen for it.
var TeleLight = Theme{
	Name: "tele-light",

	Background: hex("none"), // the terminal's own; see Theme.Background

	SurfaceOverlay:     hex("#d0d0d0"), // 252
	SurfaceHelp:        hex("#e4e4e4"), // 254
	SurfaceToast:       hex("#e4e4e4"), // 254
	SurfaceStatusBar:   hex("#303030"), // 236, dark on purpose
	SurfaceSelected:    hex("#5f5fff"), // 63, a saturated fill reads on either background
	SurfaceSelfMention: hex("#ff87ff"), // 213, carries black text
	SurfaceCode:        hex("#e4e4e4"), // 254

	Text:                hex("none"),    // the terminal's own foreground
	TextDim:             hex("#6c6c6c"), // timestamps have to stay readable on white
	TextMuted:           hex("#767676"),
	TextFaint:           hex("#878787"), // fainter than dim, still legible
	TextSubtle:          hex("#585858"), // 240, sits on the light toast surface
	TextOnSurface:       hex("#444444"), // 238
	TextStatusBar:       hex("#eeeeee"), // on the dark bar
	TextOnSelected:      hex("#ffffff"),
	TextOnSelectedMuted: hex("#e4e4e4"), // 254, over the blue fill
	TextOnToast:         hex("#303030"), // 236
	TextModeLabel:       hex("#ffffff"), // 231, over the mode fills
	TextCode:            hex("#303030"), // 236

	Accent:          hex("#005faf"), // 25, on the light terminal background
	AccentOnSurface: hex("#2e7bff"), // between the two failures either side of it:
	// muted, and it becomes a second shade of the panel's dark body text, which
	// the eye reads as one color; pale, and it washes out against the panel.
	//
	// The status bar is dark in this theme too, so its two accents are the same
	// bright ones the dark theme uses. Darkening them for a light terminal put
	// dark blue on a dark bar.
	AccentStatusBar:  hex("#00afff"), // 39
	AccentInsert:     hex("#00d700"), // 40
	AccentModeNormal: hex("#005fd7"), // 26, dark enough to carry white
	AccentModeInsert: hex("#00875f"), // 29

	StatusError:     hex("#d70000"), // 160
	StatusWarning:   hex("#af5f00"), // 130
	StatusInfo:      hex("#0087d7"), // 32
	StatusOnline:    hex("#008700"), // 28
	TickSent:        hex("#6c6c6c"),
	TickOutbox:      hex("#6c6c6c"),
	TickRead:        hex("#0050d7"),
	TickFailed:      hex("#d70000"), // 160, one red with StatusError
	NameIncoming:    hex("#008700"), // 28
	NameEditing:     hex("#875f00"), // 94; pale yellow is invisible on white
	Indicator:       hex("#007878"),
	UnreadSeparator: hex("#0050d7"),
	WaveformPlayed:  hex("#0050d7"),
	ReactionChosen:  hex("#008700"), // 28
	UnreadReaction:  hex("#d70087"), // 162
	UnreadMention:   hex("#005faf"), // 25, one tone with Accent

	BorderPane:            hex("#c6c6c6"), // 251, recedes behind the content
	BorderPaneActive:      hex("#005f00"), // 22
	BorderBubbleIn:        hex("#9e9e9e"), // visible without weighing the bubble down
	BorderBubbleOut:       hex("#005faf"), // 25
	BorderOverlay:         hex("#8a8a8a"), // 245
	BorderComposerFocused: hex("#008700"), // 28
	BorderComposerFlash:   hex("#d70000"), // 160
	BorderStatusSep:       hex("#585858"), // 240, on the dark bar

	MarkupLink:          hex("#005faf"), // 25
	MarkupRef:           hex("#870087"), // 90
	MarkupSelfMentionFg: hex("#000000"), // 0, on the pink fill

	HighlightAccent: hex("#ff8c00"),
	HighlightError:  hex("#d70000"),
	// The two bases are the tones the highlights fade back to, so each must be
	// what the thing normally looks like: chat-row text, and the incoming
	// bubble border.
	HighlightBaseChat:   hex("#303030"), // 236
	HighlightBaseBubble: hex("#9e9e9e"),
	OverlayDim:          hex("#bcbcbc"), // 250

	ComposerCounterDim: hex("#444444"), // 238
	ComposerGlyphIdle:  hex("#767676"),
	ComposerGlyphReady: hex("#005fff"), // 27

	SenderPalette: []color.Color{
		hex("#af0000"), // 1
		hex("#008700"), // 2
		hex("#0000af"), // 4
		hex("#8700af"), // 5
		hex("#007878"), // 6 was too pale on white: every 8th sender at 2.7:1
		hex("#af5f00"), // 130
		hex("#8700d7"), // 92
		hex("#005f00"), // 28
	},

	LogoGradient: []GradientStop{
		{Pos: 0.00, Color: hex("#0e1228")},
		{Pos: 0.30, Color: hex("#264181")},
		{Pos: 0.60, Color: hex("#5888d0")},
		{Pos: 0.85, Color: hex("#94c0f2")},
		{Pos: 1.00, Color: hex("#c6e4ff")},
	},
}
