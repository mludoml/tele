package config

import (
	"github.com/spf13/viper"

	"github.com/sorokin-vladimir/tele/internal/proxy"
	"github.com/sorokin-vladimir/tele/internal/translation"
)

// defaultValues is what a setting is worth when the file does not name it, and
// what an illegal value in the file is repaired to. One map rather than a run of
// SetDefault calls, because repair needs to read a default back after the file
// has been loaded over it.
//
// ui.date_format is deliberately absent: nothing reads it, so a default for it
// would be a promise the app does not keep. See #238 and the registry
// exclusions.
func defaultValues() map[string]any {
	return map[string]any{
		// auto is the file saying nothing about the route, which is not the
		// same as saying direct: it leaves ALL_PROXY in charge, the way tele
		// has since v1.3.0. The port has no default that could be right - any
		// number here would be a port nobody chose, silently dialled - so zero
		// stands for "not set" and proxy.Parse refuses it when the type needs
		// one.
[~/Projects/tele-cli-v2/internal/config/defaults.go#9EC7]
27:		"proxy.type":                 proxy.TypeAuto,
28:		"proxy.port":                 0,
29:		"ui.history_limit":           50,
30:		"ui.confirm_quit":            false,
31:		"ui.notifications.desktop":   true,
32:		"ui.notifications.toast":     true,
33:		"ui.notifications.preview":   true,
34:		"ui.toasts.error_zone":       "bottom-right",
35:		"ui.toasts.notify_zone":      "top-right",
36:		"ui.toasts.max_visible":      3,
37:		"photos.eager_full_quality":  true,
38:		"photos.mode":                "auto",
39:		"photos.kitty_placement_cap": 16,
40:		"photos.max_long_side_px":    800,
41:		"photos.disk_cache_size":     int64(256 * 1024 * 1024),
42:		"avatars.disk_cache_size":    int64(16 * 1024 * 1024),
43:		"cli.name_width":             20,
44:		"cli.account_commands":       "all",
		"translation.target_language": translation.Default,
	}
}

func setDefaults(v *viper.Viper) {
	for key, value := range defaultValues() {
		v.SetDefault(key, value)
	}
}
