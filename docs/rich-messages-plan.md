# Plan: pełna obsługa Rich Messages (feat/rich-messages)

Status: PLAN — jeszcze nie zaimplementowane. Referencje projektowe: `reference/rich-messages-spec.md`,
`skills/tg-rich-messages/SKILL.md` (kanoniczna specyfikacja Bot API 10.3, strona *wysyłania* przez boty —
używać wyłącznie jako checklistę typów bloków/limitów, NIE jako źródła kodu: `tele` odbiera te wiadomości
przez MTProto, inną reprezentację niż JSON Bot API).

## Grounding (zweryfikowane w `gotd/td@v0.161.0`, moduł już w `go.mod`)

- `tg.Message.RichMessage RichMessage` (opcjonalne pole, `richMessage#baf39d8b`) z `Blocks []PageBlockClass`,
  `Photos []PhotoClass`, `Documents []DocumentClass`, `Rtl bool`.
- `tg.PageBlockClass` — pełna rodzina typów Instant View, już zaimplementowana w gotd: `PageBlockParagraph`,
  `PageBlockHeading1..6` (MTProto dzieli nagłówek na 6 osobnych typów, inaczej niż Bot API JSON
  `{"type":"heading","size":1..6}`), `PageBlockTable`, `PageBlockList`, `PageBlockOrderedList`,
  `PageBlockBlockquote`, `PageBlockBlockquoteBlocks`, `PageBlockPullquote`, `PageBlockCollage`,
  `PageBlockSlideshow`, `PageBlockMap`, `PageBlockDetails`, `PageBlockAudio`, `PageBlockPhoto`,
  `PageBlockVideo`, `PageBlockDivider`, `PageBlockAnchor`, `PageBlockKicker`, `PageBlockCover`,
  `PageBlockMath`, `PageBlockThinking` (draft-only), `PageBlockPreformatted`, `PageBlockFooter`,
  `PageBlockSubtitle`, `PageBlockSubheader`, `PageBlockAuthorDate`, `PageBlockEmbed(Post)`,
  `PageBlockRelatedArticles`, `PageBlockChannel`, `PageBlockTitle`, `PageBlockUnsupported`.
- Przyciski: `tg.Message.ReplyMarkup ReplyMarkupClass`, konkretnie `tg.ReplyInlineMarkup{Rows []KeyboardButtonRow}`.
  `tg.KeyboardButtonClass` warianty istotne dla rich messages: `KeyboardButtonCallback{Style, Text, Data}`,
  `KeyboardButtonURL{Style, Text, URL}`. `tg.KeyboardButtonStyle{BgPrimary, BgDanger, BgSuccess, Icon}` —
  to jest natywna reprezentacja stylów `primary`/`success`/`danger` z Bot API 10.3.
  Callback wysyła się przez `tg.MessagesGetBotCallbackAnswerRequest{Peer, MsgID, Data}` →
  `tg.MessagesBotCallbackAnswer{Alert, Message, HasURL, URL, CacheTime}`.
- Streaming draft (`sendRichMessageDraft`, blok `thinking`) NIE dociera jako `tg.Message` — to osobny
  strumień: `tg.UpdateNewEphemeralMessage` / `UpdateEditEphemeralMessage` / `UpdateDeleteEphemeralMessages`
  niosące `tg.EphemeralMessage` (efemeryczne, nie trafiają do historii/store).
- Zweryfikowano `grep`: dziś **nic** w `internal/` nie dotyka `RichMessage`, `PageBlock`, `ReplyMarkup`,
  `KeyboardButton` — to całkowicie nowa funkcjonalność.
- Precedensy architektoniczne w tym repo (na gałęzi `main`, brak `feat/message-translation` na tej gałęzi):
  - `photos.eager_full_quality` (`internal/config/registry.go:205-211`, `config.go:79-81`, `defaults.go:34-36`)
    — wzorzec flagi konfiguracyjnej Toggle bramkującej zachowanie w rdzeniu/UI.
  - Reactions (`internal/store/store.go`, `internal/core/state/messages.go` `ApplyIncoming`/`ApplyEdit`)
    — wzorzec "edit niesie cały aktualny stan wiadomości", łącznie z invalidacją cache wysokości.
  - `SendReaction`/`EditMessage` w `internal/core/cmd_message.go` — wzorzec optymistycznej komendy
    UI→RPC→rollback-on-refusal, do naśladowania dla `PressCallbackButton`.
  - `messagelist_mosaic.go` (album/grid) — gotowy scaffold do reużycia dla `collage`/`slideshow`.
  - `messagelist_height.go`'s `wrappedLineCount` — wzorzec "mierz przez faktyczne renderowanie", żeby
    uniknąć dryfu render/height (plik sam ostrzega o tym w komentarzach, issue #115/#193/#231).

## Zakres i świadome wyłączenia

W zakresie: pełny odbiór i renderowanie `RichMessage` (wszystkie typy `PageBlockClass` powyżej — dla tych
niemożliwych do sensownego oddania w terminalu, jawny placeholder, nie ciche pomijanie), natywne przyciski
inline (style, callback, URL) z nawigacją klawiaturą, streaming draft (ephemeral).

Świadomie POZA zakresem (udokumentować w `docs/rich-messages.md`, nie cichy dług):
- Prawdziwe renderowanie LaTeX (`RichBlockMath`) — tylko surowe wyrażenie jako preformatted.
- Interaktywna mapa (`PageBlockMap`) — tylko współrzędne + link "otwórz w przeglądarce map".
- Przełączanie slajdów w `slideshow` — renderowane jak `collage` (siatka), bez paginacji swipe.
- `KeyboardButtonRequestPhone/RequestGeoLocation/RequestPoll/SwitchInline/Game/Buy/WebView/RequestPeer/Copy`
  — wymagają integracji OS (kontakty, GPS, in-app browser, płatności), których terminal nie ma; renderowane
  jako widocznie wyłączone przyciski z jednolinijkową adnotacją, nigdy ciche zniknięcie.
- `PageBlockEmbed/EmbedPost/RelatedArticles/Channel` — rzadkie w praktyce boty rzadko ich używają w
  rich message (są głównie z Instant View artykułów) → `PageBlockUnsupported`-style fallback.

## Fazy

### Faza 0 — Fundament: config + model domenowy
- `internal/config/registry.go`: nowy `Entry` `rich_messages.enabled` (Widget: `settings.Toggle`,
  Applies: `settings.Immediate`), wzorzec identyczny do `photos.eager_full_quality`.
- `internal/config/config.go`: `RichMessagesConfig{Enabled bool}` + tag `mapstructure`, wzorzec
  `PhotosConfig.EagerFullQuality`.
- `internal/config/defaults.go`: domyślnie `true` (parsowanie zawsze się odbywa; flaga steruje wyłącznie
  renderowaniem — patrz Faza 1, dane trzymamy zawsze, żeby wyłączenie flagi nie gubiło informacji przy
  ponownym włączeniu).
- `internal/domain/model.go`:
  - `BlockKind` enum (wzorzec `MediaKind`, `BlockKindCount` sentinel) pokrywający wszystkie obsługiwane
    warianty `PageBlockClass` + `BlockKindUnsupported`.
  - `PageBlock` struct — drzewo: `Kind BlockKind`, `Text *RichText` (własna reprezentacja: string + zakres
    encji analogiczny do istniejącego `MessageEntity`, NIE HTML), `Level int` (nagłówki/listy zagnieżdżone),
    `Children []PageBlock` (blockquote/details/collage/slideshow/list items), `Table *TableBlock`
    (`Rows [][]TableCell`, `TableCell{Text RichText, IsHeader bool, Colspan, Rowspan int, Align, VAlign string}`),
    `Media *MediaRef` (photo/video/audio/animation/voice_note — reużyj istniejący `MediaRef`),
    `Caption *RichText`, `Credit *RichText`, `Map *MapBlock{Lat, Long float64, Zoom int}`,
    `DetailsOpen bool` (domyślny stan rozwinięcia z serwera; stan *lokalny* UI trzymany osobno, patrz Faza 8),
    `Language string` (dla `pre`).
  - `ButtonStyle` enum (`ButtonStyleDefault/Primary/Success/Danger`).
  - `KeyboardButton{Text string, Style ButtonStyle, Action ButtonAction}`, `ButtonAction` — closed union:
    `ButtonActionCallback{Data []byte}`, `ButtonActionURL{URL string}`, `ButtonActionUnsupported{Reason string}`.
  - `ReplyMarkup{Rows [][]KeyboardButton}`.
  - `Message`: dodać `RichBlocks []PageBlock` i `ReplyMarkup *ReplyMarkup`, nil-by-default, komentarz w stylu
    `Forward`/`LocalMedia` (linie 327-334) opisujący kiedy jest ustawione.
- Testy: zero-value/`Zero()`-style testy nowych typów w `internal/domain`.

### Faza 1 — Ingestion: MTProto → domena
- `internal/tg/parse.go`: `convertMessage` (linia 261) — po dzisiejszym `Media`/`Entities` dodać:
  - `convertRichBlocks(blocks []tg.PageBlockClass) []domain.PageBlock` — `switch` po każdym wariancie
    `PageBlockClass` (wzorzec `classifyMedia`, linie ~183-198), rekurencyjnie dla blocków-kontenerów.
    Nierozpoznany/nieobsługiwany wariant → `BlockKindUnsupported` z krótkim opisem (NIE panic, NIE ciche
    pominięcie).
  - `convertReplyMarkup(rm tg.ReplyMarkupClass) *domain.ReplyMarkup` — obsłuż `*tg.ReplyInlineMarkup`;
    inne warianty (`ReplyKeyboardMarkup`, `ReplyKeyboardForceReply`, ...) zwracają `nil` (to nie są rich
    message buttons, poza zakresem tego feature).
  - Dla każdego `KeyboardButtonClass`: `KeyboardButtonCallback`→`ButtonActionCallback{Data}`,
    `KeyboardButtonURL`→`ButtonActionURL{URL}`, wszystko inne→`ButtonActionUnsupported{Reason: "<nazwa typu>"}`.
    Styl z `btn.GetStyle()` → `BgPrimary`/`BgSuccess`/`BgDanger` → `ButtonStyle*` (pierwszy `true` wygrywa,
    kolejność primary>success>danger, żaden `true`→`ButtonStyleDefault`).
- `internal/tg/messages.go`: `sentMessage` (linia 650) — bez zmian (`tele` nie jest botem, nigdy nie
  wysyła `RichMessage`/`ReplyMarkup` we własnych wiadomościach; pola zostają zerowe).
- Testy: fixture `*tg.Message` z ręcznie złożonym `RichMessage`/`ReplyMarkup` per typ bloku/przycisku →
  asercja dokładnego `domain.PageBlock`/`domain.ReplyMarkup`.

### Faza 2 — Persystencja
- Bez migracji schematu — `internal/store/sqlite_messages.go` serializuje cały `domain.Message` do kolumny
  JSON (`snapshotMessageWritesLocked`/`flushMessageRows`/`LoadMessages`). Dodać test regresyjny: zapis→odczyt
  wiadomości z `RichBlocks`+`ReplyMarkup` daje identyczną strukturę (round-trip).

### Faza 3 — Core: edycja + komenda kliknięcia przycisku
- `internal/core/state/messages.go`: `ApplyEdit` już podmienia cały `domain.Message` — zweryfikować testem,
  że `RichBlocks`/`ReplyMarkup` też są zamieniane 1:1 przy edycji (bot może dosłać/zmienić przyciski po
  callbacku — to jest zwykły `messages.editMessage`, dociera normalnym `UpdateEditMessage`, ZERO nowego
  kodu ingestion, tylko test potwierdzający).
- `internal/tg/messages.go`: nowa metoda `GetBotCallbackAnswer(ctx context.Context, peer tg.InputPeerClass,
  msgID int, data []byte) (tg.MessagesBotCallbackAnswer, error)` opakowująca
  `tg.MessagesGetBotCallbackAnswerRequest` — obok istniejących `SendMessage`/`EditMessage`.
- `internal/core/cmd_message.go`: `Owner.PressCallbackButton(ctx, chatID int64, msgID int, data []byte)
  (alert string, url string, err error)` — wzorzec `SendReaction`/`EditMessage` (resolve peer, RPC, brak
  potrzeby optymistycznej zmiany UI bo callback nie zmienia treści wiadomości samodzielnie — ewentualną
  zmianę przynosi późniejszy, oddzielny `editMessage` update przez istniejący pipeline).
- Testy: `cmd_message_test.go`-owy fake-client test dla `PressCallbackButton` (sukces, `HasURL`, `Alert`,
  błąd RPC).

### Faza 4 — Rendering: bloki tekstowe
Plik `internal/ui/components/messagelist_richblocks.go` (nowy): `renderRichBlocks(blocks []domain.PageBlock,
width int) []string` + `richBlocksHeight(blocks []domain.PageBlock, width int) int` — **obie funkcje przez
wspólny helper mierzący realny wrap** (rozszerzenie wzorca `wrappedLineCount`), żeby nie powielać ryzyka
dryfu, które plik `messagelist_height.go` już ostrzega. Obsłużyć: `paragraph`, `heading1..6` (rozmiar
oddany przez bold/underline/kolor - terminal nie ma rozmiaru fontu, 1-2 bold+underline, 3-4 bold, 5-6
bold+dim), `footer`(dim), `divider`(pozioma linia), `anchor`(no-op, zero wysokości), `kicker`(dim caps),
`cover`(bold tytuł), `pre`(monospace/code-block styl, z `language` jako etykietą), `blockquote`/
`blockquoteBlocks`(lewy pasek + wcięcie + `credit`), `pullquote`(wyśrodkowany, kursywa), `list`/
`orderedList`(wcięcie, znaczniki `-`/`1.`/`a.`/`i.` wg `type`, checkbox `[ ]`/`[x]` jeśli `has_checkbox`).
Podpięcie w `messagelist_render.go`: `bubbleContentLines` (linia 291) — jeśli `msg.RichBlocks != nil` i
`cfg.RichMessages.Enabled`, renderuj bloki zamiast zwykłego tekstu z encjami; w przeciwnym razie dotychczasowa
ścieżka bez zmian. Analogiczna gałąź w `msgHeight` (`messagelist_height.go`).

### Faza 5 — Rendering: tabele
`RichBlockTable`/`domain.TableBlock` → siatka ASCII/lipgloss z podziałem szerokości bąbelka na kolumny,
zawijaniem długich komórek, `align`/`valign`, `is_header` (bold + separator pod wierszem nagłówkowym),
`is_compact` (mniejszy padding). `colspan`/`rowspan` > 1: uproszczone scalanie wizualne (powielenie/łączenie
komórek), udokumentować jako uproszczenie w `docs/rich-messages.md`, nie próbować pełnej algebry spanów.
Wysokość = suma wysokości wierszy (wiersz = max zawiniętych linii wśród jego komórek).

### Faza 6 — Rendering: bloki medialne, collage/slideshow, mapa
- Photo/video/audio/voice_note jako rich-block: reużyć istniejącą ścieżkę `messagelist_media.go`
  (ten sam `MediaRef`) + `caption`/`credit` pod spodem.
- Collage/slideshow: adapter zamieniający `[]domain.PageBlock` (dzieci typu media) na wejście, którego
  dziś oczekuje `renderMosaic`/`mosaicHeight` (`messagelist_mosaic.go`) dla albumów po `GroupedID` — bez
  duplikowania matematyki układu siatki. Slideshow renderowany identycznie jak collage (bez swipe/paginacji
  — patrz "Świadome wyłączenia").
- Map: brak renderowania kafli w terminalu — blok tekstowy: współrzędne, `zoom`, `caption`, plus przycisk-
  link (reużywa mechanizm z Fazy 7: `ButtonActionURL` do `https://maps.google.com/?q=<lat>,<long>`).

### Faza 7 — Rendering + interakcja: przyciski inline
- Render: wiersze przycisków pod bąbelkiem, kolor tła wg `ButtonStyle` (lipgloss: primary=niebieski,
  success=zielony, danger=czerwony, default=neutralny, wg palety z `internal/ui/theme`).
  `ButtonActionUnsupported` → przycisk wyraźnie wyszarzony + końcowa adnotacja jednolinijkowa (typ akcji),
  NIE znika.
- Nawigacja klawiaturą: gdy zaznaczona wiadomość ma `ReplyMarkup`, wejście w "tryb przycisków" (nowy
  keybinding w `internal/ui/keys/keys.go`, wpis w `docs/keybindings.md`, wzorzec istniejących bindów)
  pozwala strzałkami poruszać się między przyciskami w obrębie wiadomości, Enter/Space aktywuje:
  - `ButtonActionURL` → otwarcie w przeglądarce systemowej (sprawdzić czy istnieje już helper otwierania
    linków w tekście wiadomości; jeśli nie — `xdg-open`/odpowiednik, minimalna funkcja w `internal/ui`
    lub nowym `internal/openurl` wg istniejących konwencji nazewnictwa pakietów).
  - `ButtonActionCallback` → `Owner.PressCallbackButton`; wynikowy `alert`/`message` pokazać przez
    istniejący mechanizm powiadomień/statusbar (sprawdzić `internal/notices` lub statusbar component —
    NIE tworzyć nowego systemu toastów, jeśli któryś już istnieje).
- Testy: `keys/matcher_test.go`-wzorcowy test nowego bindu, render/height testy dla wierszy przycisków,
  test integracyjny w `internal/ui` symulujący nawigację+aktywację na fake ownerze.

### Faza 8 — Rendering + interakcja: `details` (collapsible)
Stan rozwinięcia jest lokalny dla UI (nie z serwera poza wartością domyślną `is_open`): mapa
`map[messageBlockKey]bool` w modelu `messagelist` (klucz: `chatID/msgID/ścieżka-bloku`), reset przy
restarcie — to nieszkodliwe uproszczenie, udokumentować. Nowy bind (jak w Fazie 7, ten sam plik/wzorzec)
"rozwiń/zwiń szczegóły pod zaznaczeniem". Toggle wywołuje `invalidateHeights()` dla tej pozycji.

### Faza 9 — Rendering: `mathematical_expression`
Brak realnego LaTeX w terminalu: renderować surowe wyrażenie jako `pre`-podobny blok z prefiksem "∑ " i
jawnym komentarzem w kodzie, że to świadome uproszczenie (nie TODO).

### Faza 10 — Ephemeral rich-message drafts (streaming)
Osobna, lekka ścieżka NIEPRZECHODZĄCA przez `domain.Message`/store (efemeryczne, nie trafiają do historii):
- `internal/tg/dispatcher.go`: nowe hooki na `tg.UpdateNewEphemeralMessage`/`UpdateEditEphemeralMessage`/
  `UpdateDeleteEphemeralMessages`, budujące minimalny transient event (nowy `store.EventKind` np.
  `EventEphemeralMessage`/`EventEphemeralDelete` z payloadem `chatID`, `blocks []domain.PageBlock`, `date`)
  — NIE zapisywany przez `sqlite_messages.go`.
  - `internal/core`: krótkotrwały slice/mapa "aktywne drafty per chat" w stanie w pamięci, bez persystencji,
    z TTL 30s (timer czyszczący, zgodnie z limitem protokołu) i czyszczeniem na `UpdateDeleteEphemeralMessages`.
  - `internal/ui`: overlay pod historią czatu (osobny komponent, nie wiersz `messagelist`) renderujący
    aktualne bloki draftu (przez `renderRichBlocks` z Fazy 4) z wizualnym wskaźnikiem "generowanie...".
- To najbardziej architektonicznie nowy i najmniej wartościowy dla trwałej historii czatu fragment —
  jeśli czas/jakość implementacji tego wymaga, może zostać ograniczony do najprostszej wersji (statyczny
  napis "bot pisze rich message..." bez live-streamowania treści blok-po-bloku) BEZ obniżania pozostałych
  faz, i musi być jawnie opisane w `docs/rich-messages.md`, a nie po cichu pominięte.

### Faza 11 — Dokumentacja
`docs/rich-messages.md`: tabela pokrycia (每 `PageBlockKind` → obsłużony/uproszczony/nieobsługiwany +
uzasadnienie), opis flagi `rich_messages.enabled`, opis bindów, lista świadomych wyłączeń z tej sekcji
planu. `CHANGELOG.md`: wpis feature. Zachować `reference/rich-messages-spec.md` i
`skills/tg-rich-messages/SKILL.md` bez zmian (spec referencyjny).

## Weryfikacja (per faza, nie tylko na końcu)
- Po każdej fazie: `go build ./...`, `go vet ./...`, testy pakietów które faza dotknęła.
- Na końcu całości: `mise run check` (vet + lint + test) musi przejść czysto — zero nowych ostrzeżeń
  `golangci-lint`/`staticcheck`, zero nieużywanego kodu.
- Ręczna weryfikacja end-to-end wymaga prawdziwego bota wysyłającego rich message (np. publiczny
  `@RichTextDemoBot` wymieniony w README `serejaris/telegram-skills`) — opisać jako manualny krok QA,
  niemożliwy do zautomatyzowania bez działającego bota/tokenu; nie blokuje merge'a testów jednostkowych.
