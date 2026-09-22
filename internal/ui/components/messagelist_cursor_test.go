package components_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageList_CursorStartsAtNewest(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	assert.Equal(t, 5, ml.SelectedMessageID())
}

func TestMessageList_CursorUp_SelectsOlderMessage(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	ml.CursorUp()
	assert.Equal(t, 4, ml.SelectedMessageID())
	ml.CursorUp()
	assert.Equal(t, 3, ml.SelectedMessageID())
}

func TestMessageList_CursorDown_SelectsNewerMessage(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	ml.CursorUp()
	ml.CursorUp() // cursor on msg 3
	ml.CursorDown()
	assert.Equal(t, 4, ml.SelectedMessageID())
}

func TestMessageList_CursorUp_StopsAtOldest(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	ml.CursorUp()
	ml.CursorUp()
	atOldest := ml.CursorUp() // already on the oldest message
	assert.Equal(t, 1, ml.SelectedMessageID())
	assert.True(t, atOldest)
}

// #124 case 1: history shorter than the viewport — the top message must be
// reachable even though nothing scrolls.
func TestMessageList_CursorUp_SelectsTopInShortChat(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	ml.CursorUp() // msg 2
	ml.CursorUp() // msg 1
	assert.Equal(t, 1, ml.SelectedMessageID())
	assert.Contains(t, ml.View(), "msg 1")
}

// Cursor stepping into older history pulls the viewport so the active bubble
// stays on screen.
func TestMessageList_CursorUp_ScrollsCursorIntoView(t *testing.T) {
	ml := components.NewMessageList(9, 40) // ~3 bubbles tall
	ml.SetMessages(makeMessages(20))
	for i := 0; i < 6; i++ {
		ml.CursorUp()
	}
	ml.View()
	_, ok := ml.SelectedBubbleRect()
	assert.True(t, ok, "cursor bubble must be visible after stepping up")
}

// Line-scrolling up (j/k) past the cursor must drag the cursor along so it never
// leaves the viewport.
func TestMessageList_LineScrollUp_KeepsCursorInViewport(t *testing.T) {
	ml := components.NewMessageList(3, 40) // ~1 bubble tall
	ml.SetMessages(makeMessages(10))       // cursor starts on newest (msg 10)
	ml.ScrollUpBy(6)                       // pan toward older history
	ml.View()
	_, ok := ml.SelectedBubbleRect()
	require.True(t, ok, "cursor must stay visible while line-scrolling")
	assert.NotEqual(t, 10, ml.SelectedMessageID(), "cursor dragged off the newest bubble")
}

// Line-scrolling down past the cursor must drag it down to stay in view.
func TestMessageList_LineScrollDown_KeepsCursorInViewport(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(10))
	for i := 0; i < 5; i++ {
		ml.CursorUp() // move cursor up into history (msg 5)
	}
	ml.ScrollDownBy(30) // scroll all the way back to the bottom
	ml.View()
	_, ok := ml.SelectedBubbleRect()
	require.True(t, ok, "cursor must stay visible while line-scrolling")
	assert.NotEqual(t, 5, ml.SelectedMessageID(), "cursor dragged toward the bottom")
}

// The reported bug: after a line scroll the cursor sits at a viewport edge; the
// first ctrl+k must step it within the viewport, not teleport-center the chat.
func TestMessageList_CursorUp_NoJumpAfterLineScroll(t *testing.T) {
	ml := components.NewMessageList(15, 40)
	ml.SetMessages(makeMessages(30))
	ml.ScrollUpBy(20) // pan into history; cursor clamps to the bottom edge
	off0 := ml.ScrollInfo().Offset
	ml.CursorUp()
	assert.Equal(t, off0, ml.ScrollInfo().Offset,
		"first ctrl+k after a line scroll must not jump-center the chat")
}

// Once the cursor reaches the middle, further steps scroll the chat.
func TestMessageList_CursorUp_ScrollsOnceCentered(t *testing.T) {
	ml := components.NewMessageList(15, 40)
	ml.SetMessages(makeMessages(30))
	off0 := ml.ScrollInfo().Offset
	for i := 0; i < 12; i++ {
		ml.CursorUp()
	}
	assert.Less(t, ml.ScrollInfo().Offset, off0,
		"after the cursor reaches the middle the chat scrolls toward older history")
}

// Symmetric: stepping down walks the cursor toward the bottom before scrolling.
func TestMessageList_CursorDown_StepsWithoutScrollingUntilBottom(t *testing.T) {
	ml := components.NewMessageList(15, 40)
	ml.SetMessages(makeMessages(30))
	for i := 0; i < 12; i++ {
		ml.CursorUp() // cursor centered up in history; viewport scrolled
	}
	off0 := ml.ScrollInfo().Offset
	ml.CursorDown()
	assert.Equal(t, off0, ml.ScrollInfo().Offset,
		"cursor descends within the viewport; the chat must not scroll yet")
}

// In the middle region the cursor stays vertically centered while the viewport
// scrolls underneath it.
func TestMessageList_CursorUp_KeepsCursorCentered(t *testing.T) {
	ml := components.NewMessageList(15, 40)
	ml.SetMessages(makeMessages(20))
	for i := 0; i < 10; i++ {
		ml.CursorUp() // msg 20 -> msg 10
	}
	ml.View()
	rect1, ok := ml.SelectedBubbleRect()
	require.True(t, ok)
	top1 := rect1.Top

	ml.CursorUp() // msg 10 -> msg 9
	ml.View()
	rect2, ok := ml.SelectedBubbleRect()
	require.True(t, ok)

	assert.Equal(t, 9, ml.SelectedMessageID())
	assert.Equal(t, top1, rect2.Top, "cursor stays put; the viewport scrolls instead")
	assert.InDelta(t, 15/2, top1, 1, "cursor sits near the vertical middle")
}

// A bubble taller than half the viewport (a photo's reserved pre-load
// footprint, or a rich media block) must still land fully on screen when
// CursorUp centers it: naively placing its top at the vertical middle pushed
// its bottom off the bottom edge even though the whole bubble fit the
// viewport (reported live: selection focus stuck below the visible window).
func TestMessageList_CursorUp_TallBubbleFullyOnScreen(t *testing.T) {
	ml := components.NewMessageList(15, 60)
	ml.SetRichMessages(true)
	now := time.Now()
	var msgs []domain.Message
	for i := 1; i <= 20; i++ {
		m := domain.Message{ID: i, ChatID: 1, Date: now}
		switch i % 4 {
		case 0:
			m.Media = &domain.MediaRef{Kind: domain.MediaPhoto}
			m.Photo = &domain.PhotoRef{ID: int64(1000 + i)}
		case 1:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: int64(2000 + i)}},
				{Kind: domain.BlockKindHeading, Level: 3, Text: &domain.RichText{Text: fmt.Sprintf("Heading %d", i)}},
			}
		default:
			m.Text = fmt.Sprintf("msg %d", i)
		}
		msgs = append(msgs, m)
	}
	ml.SetMessages(msgs)

	for i := 0; i < 20; i++ {
		ml.CursorUp()
		ml.View()
		rect, ok := ml.SelectedBubbleRect()
		require.True(t, ok)
		if rect.Height > ml.ViewHeight() {
			continue // a bubble taller than the whole viewport cannot fit either way
		}
		assert.GreaterOrEqualf(t, rect.Top, 0, "msg %d: bubble top above the viewport", ml.SelectedMessageID())
		assert.LessOrEqualf(t, rect.Top+rect.Height, ml.ViewHeight(),
			"msg %d: bubble bottom (top=%d height=%d) clipped below the viewport (height=%d)",
			ml.SelectedMessageID(), rect.Top, rect.Height, ml.ViewHeight())
	}
}
