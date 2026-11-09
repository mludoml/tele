package components

import "github.com/sorokin-vladimir/tele/internal/domain"

// This file holds which collapsible sections are open. The server sends a
// default (`details.is_open`) and the domain keeps it as `DetailsOpen`, but
// whether a section is open *now* is a view state: opening one changes nothing
// about the message, and nothing about it belongs in the store or on the wire.
//
// The state is keyed by (message, block path) rather than by block index alone,
// because a chat can hold two bots' messages with identically-shaped documents
// and a chat can be reopened on a different day. It is deliberately not
// persisted: reopening tele collapses every section back to what the bot sent,
// which is an honest default rather than a stale decision nobody remembers
// making. See docs/rich-messages.md.

// richDetailsState maps "msgID/path" to the reader's expansion choice.
type richDetailsState map[string]bool

// richDetailsKey is the map key for one collapsible block: its message and the
// index trail from the document root.
func richDetailsKey(msgID int, path string) string {
	return itoaKey(msgID) + "/" + path
}

// itoaKey renders a message id for the state key. It is deliberately decimal so
// a stale key in a log can be read back as the message it names.
func itoaKey(id int) string {
	if id == 0 {
		return "0"
	}
	neg := id < 0
	if neg {
		id = -id
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// richDetailsOpen reports whether a details block is expanded: the reader's
// choice when they have made one, the server's default otherwise.
func (ml *MessageList) richDetailsOpen(msgID int, path string, defaultOpen bool) bool {
	if ml.detailsOpen == nil {
		return defaultOpen
	}
	open, ok := ml.detailsOpen[richDetailsKey(msgID, path)]
	if !ok {
		return defaultOpen
	}
	return open
}

// ToggleDetails collapses or expands the collapsible section at a block path of
// the selected message, and reports whether a section was there to toggle.
//
// Only reachable sections are toggled: a details block inside a collapsed parent
// is not on screen, so a toggle key pressed while the cursor is on the parent
// must act on the parent rather than on something invisible.
func (ml *MessageList) ToggleDetails(msgID int, path string, defaultOpen bool) bool {
	if msgID == 0 {
		return false
	}
	if ml.detailsOpen == nil {
		ml.detailsOpen = make(richDetailsState)
	}
	key := richDetailsKey(msgID, path)
	ml.detailsOpen[key] = !ml.richDetailsOpen(msgID, path, defaultOpen)
	ml.invalidateHeights()
	return true
}

// SelectedMessageDetailsPath returns the block path of the first collapsible
// section in the selected message that is reachable (inside an open parent), and
// its server default. ok is false when the message has no details block, which
// is what makes the toggle key a no-op rather than a guess.
func (ml *MessageList) SelectedMessageDetailsPath() (int, string, bool, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil {
		return 0, "", false, false
	}
	path, defaultOpen, ok := ml.firstDetailsPath(msg.ID, msg.RichBlocks)
	if !ok {
		return 0, "", false, false
	}
	return msg.ID, path, defaultOpen, true
}

// firstDetailsPath walks a block tree and returns the path of the first
// reachable collapsible section, together with the server's default for it.
//
// "Reachable" is the whole subtlety: a section nested inside a collapsed one is
// not on screen, so it cannot be what a toggle key acts on. The walk therefore
// descends only through expanded containers, and the first section it meets is
// the outermost visible one.
func (ml *MessageList) firstDetailsPath(msgID int, blocks []domain.PageBlock) (path string, defaultOpen bool, ok bool) {
	return ml.walkDetails(msgID, "", blocks)
}

func (ml *MessageList) walkDetails(msgID int, prefix string, blocks []domain.PageBlock) (string, bool, bool) {
	for i, b := range blocks {
		path := itoaKey(i)
		if prefix != "" {
			path = prefix + "/" + path
		}
		if b.Kind == domain.BlockKindDetails {
			// A collapsed section is the target: toggling it is what reveals the
			// content the reader cannot see yet. An expanded one is still the
			// fallback - the key then closes it - but only after its own
			// contents have had their turn, so the innermost reachable section
			// wins while everything around it is open.
			if !ml.richDetailsOpen(msgID, path, b.DetailsOpen) {
				return path, b.DetailsOpen, true
			}
			if len(b.Children) > 0 {
				if p, d, ok := ml.walkDetails(msgID, path, b.Children); ok {
					return p, d, ok
				}
			}
			return path, b.DetailsOpen, true
		}
		if len(b.Children) > 0 {
			if p, d, ok := ml.walkDetails(msgID, path, b.Children); ok {
				return p, d, ok
			}
		}
	}
	return "", false, false
}
