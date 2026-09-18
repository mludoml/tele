package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rich message is serialised whole, like every other message, so the block
// tree and the inline keyboard must survive a restart. Without this a bot's
// document would come back as its flattened text and lose its buttons, which is
// only visible after reopening the chat.
func TestSQLite_MessageRichContent_PersistsSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	rich := domain.Message{
		ID: 1, ChatID: 6, Text: "fallback", Date: time.Unix(1000, 0),
		RichBlocks: []domain.PageBlock{
			{Kind: domain.BlockKindHeading, Level: 2, Text: &domain.RichText{Text: "Report"}},
			{
				Kind: domain.BlockKindDetails,
				Text: &domain.RichText{Text: "More"},
				Children: []domain.PageBlock{
					{Kind: domain.BlockKindParagraph, Text: &domain.RichText{
						Text:     "body",
						Entities: []domain.MessageEntity{{Type: "bold", Offset: 0, Length: 4}},
					}},
				},
				DetailsOpen: true,
			},
			{
				Kind: domain.BlockKindTable,
				Table: &domain.TableBlock{Bordered: true, Rows: [][]domain.TableCell{{
					{Text: domain.RichText{Text: "a"}, IsHeader: true, Align: "right", VAlign: "bottom", Colspan: 2, Rowspan: 2},
				}}},
				Caption:  &domain.RichText{Text: "cap"},
				Language: "go",
			},
			{
				Kind:   domain.BlockKindCollage,
				Media:  &domain.MediaRef{Kind: domain.MediaPhoto},
				Photo:  &domain.PhotoRef{ID: 4, AccessHash: 5, FileReference: []byte{1, 2}, DCID: 2, ThumbSize: "m"},
				Map:    &domain.MapBlock{Lat: 1.5, Long: -2.5, Zoom: 12},
				Credit: &domain.RichText{Text: "credit"},
			},
			{Kind: domain.BlockKindUnsupported, Label: "pageBlockEmbed"},
		},
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{
				{Text: "Yes", Style: domain.ButtonStylePrimary, Action: domain.ButtonAction{Kind: domain.ButtonActionCallback, Data: []byte{1, 2}}},
				{Text: "Docs", Action: domain.ButtonAction{Kind: domain.ButtonActionURL, URL: "https://example.com"}},
			},
			{{Text: "Phone", Action: domain.ButtonAction{Kind: domain.ButtonActionNone, Reason: "keyboardButtonRequestPhone"}}},
		}},
	}

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 6, Peer: domain.Peer{ID: 6, Type: domain.PeerUser}})
	s.SetMessages(6, []domain.Message{rich})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(6)
	got := s2.Messages(6)

	require.Len(t, got, 1)
	assert.Equal(t, rich.RichBlocks, got[0].RichBlocks)
	assert.Equal(t, rich.ReplyMarkup, got[0].ReplyMarkup)
}

// A message without rich content stays without it: nil is not an empty tree, and
// the renderer decides which path to take from exactly that.
func TestSQLite_PlainMessage_HasNoRichContentAfterReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.SetMessages(7, []domain.Message{{ID: 1, ChatID: 7, Text: "plain", Date: time.Unix(1000, 0)}})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(7)
	got := s2.Messages(7)

	require.Len(t, got, 1)
	assert.Nil(t, got[0].RichBlocks)
	assert.Nil(t, got[0].ReplyMarkup)
}
