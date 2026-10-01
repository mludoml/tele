package tg

import (
	"context"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"rsc.io/qr"
)

// renderQR draws a QR code as half-block ANSI art: two code rows share one
// terminal row (▀/▄/█/space), so the printed art keeps the code's true
// aspect ratio instead of coming out twice as tall as it is wide. Colors are
// pinned to black-on-white regardless of the terminal's theme, since a QR
// scanner needs reliable contrast more than it needs to match the palette.
func renderQR(url string) (string, error) {
	code, err := qr.Encode(url, qr.M)
	if err != nil {
		return "", err
	}

	const quiet = 2 // modules of white margin, so phone cameras can find the code
	var b strings.Builder
	for y := -quiet; y < code.Size+quiet; y += 2 {
		b.WriteString("\x1b[30;47m")
		for x := -quiet; x < code.Size+quiet; x++ {
			top, bottom := code.Black(x, y), code.Black(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// qrLogin drives Telegram's QR login flow in place of the phone/code one:
// it exports a login token, hands each rendering to af.QRFrames for the UI to
// display, and blocks until a device that is already signed in scans and
// accepts it (refreshing the token on every ~30s expiry in the meantime).
// If the account has 2FA enabled, it completes the same Password() prompt the
// phone flow uses, so the UI needs no separate handling for that case.
func qrLogin(ctx context.Context, tc *telegram.Client, af *AuthFlow, loggedIn qrlogin.LoggedIn) (*tg.AuthAuthorization, error) {
	authorization, err := tc.QR().Auth(ctx, loggedIn, func(ctx context.Context, token qrlogin.Token) error {
		rendered, rerr := renderQR(token.URL())
		if rerr != nil {
			return rerr
		}
		select {
		case af.QRFrames <- rendered:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
		password, perr := af.Password(ctx)
		if perr != nil {
			return nil, perr
		}
		return tc.Auth().Password(ctx, password)
	}
	return authorization, err
}
