package components_test

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func buildMixedRichMessages(n int) []domain.Message {
	now := time.Now()
	var msgs []domain.Message
	for i := 1; i <= n; i++ {
		m := domain.Message{ID: i, ChatID: 1, Date: now}
		switch i % 7 {
		case 0:
			m.Media = &domain.MediaRef{Kind: domain.MediaPhoto}
			m.Photo = &domain.PhotoRef{ID: int64(1000 + i)}
			m.Text = "a plain photo"
		case 1:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: int64(2000 + i)}},
				{Kind: domain.BlockKindHeading, Level: 3, Text: &domain.RichText{Text: fmt.Sprintf("Heading %d", i)}},
				{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "Some longer paragraph body that wraps across several lines depending on the viewport width available to it."}},
			}
		case 2:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
					Bordered: true,
					Rows: [][]domain.TableCell{
						{{Text: domain.RichText{Text: "A"}, IsHeader: true}, {Text: domain.RichText{Text: "B"}, IsHeader: true}},
						{{Text: domain.RichText{Text: "1"}}, {Text: domain.RichText{Text: "2"}}},
						{{Text: domain.RichText{Text: "3"}}, {Text: domain.RichText{Text: "4"}}},
					},
				}},
			}
		case 3:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "Choose an option:"}},
			}
			m.ReplyMarkup = &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
				{{Text: "Yes", Style: domain.ButtonStylePrimary}, {Text: "No"}},
				{{Text: "Maybe"}},
			}}
		case 4:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindCollage, Children: []domain.PageBlock{
					{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: int64(3000 + i)}},
					{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: int64(3100 + i)}},
					{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: int64(3200 + i)}},
				}},
			}
		case 5:
			m.RichBlocks = []domain.PageBlock{
				{Kind: domain.BlockKindDetails, Text: &domain.RichText{Text: "More info"}, Children: []domain.PageBlock{
					{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "Hidden detail body that only shows when expanded."}},
				}},
			}
		default:
			m.Text = fmt.Sprintf("plain msg %d", i)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// A randomized walk of line-scrolls and cursor keypresses over a chat mixing
// every rich-message shape (plain photos, photo+heading, tables, buttons,
// collages, details) across several viewport sizes, checked against the real
// rendered SelectedBubbleRect rather than the internal height estimate: the
// estimate can be off by a line from the actual render for some message
// shapes without that ever being visible, so this is the ground truth for
// "does the user actually see the selection clipped".
func TestMessageList_SelectedBubbleRect_StaysOnScreenAcrossMixedRichContent(t *testing.T) {
	for _, size := range []struct{ h, w int }{{15, 60}, {10, 40}, {25, 100}, {8, 30}} {
		ml := components.NewMessageList(size.h, size.w)
		ml.SetRichMessages(true)
		msgs := buildMixedRichMessages(40)
		ml.SetMessages(msgs)

		rng := rand.New(rand.NewPCG(uint64(size.h), uint64(size.w)))
		cursorActions := []func(){
			func() { ml.CursorUp() },
			func() { ml.CursorDown() },
		}
		cursorNames := []string{"CursorUp", "CursorDown"}
		scrollActions := []func(){
			func() { ml.ScrollUp() },
			func() { ml.ScrollDown() },
		}
		for i := 0; i < 400; i++ {
			for j := 0; j < rng.IntN(4); j++ {
				scrollActions[rng.IntN(len(scrollActions))]()
			}
			a := rng.IntN(len(cursorActions))
			cursorActions[a]()

			ml.View()
			rect, ok := ml.SelectedBubbleRect()
			if !ok || rect.Height > ml.ViewHeight() {
				continue
			}
			if rect.Top < 0 || rect.Top+rect.Height > ml.ViewHeight() {
				t.Errorf("size=%v %s-%d: msg=%d rect=%+v out of viewport (height=%d)",
					size, cursorNames[a], i, ml.SelectedMessageID(), rect, ml.ViewHeight())
			}
		}
	}
}
