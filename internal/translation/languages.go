// Package translation holds the languages a message may be translated into.
//
// It is a catalog and nothing else: no client, no config and no UI. Telegram's
// messages.translateText takes an ISO 639-1 code and publishes no narrower list
// of targets, so the set offered here is the standardized two-letter set, and a
// code that turns out to be unsupported is refused by the server and reported to
// the person rather than quietly replaced with another language.
package translation

import "sort"

// Language is one target language: the code Telegram is given and the English
// name a person picks it by.
type Language struct {
	Code string
	Name string
}

// catalog is the standardized ISO 639-1 set, one entry per code. The
// three-letter codes, the codes withdrawn from the standard (in, iw, ji, jw, mo,
// sh) and the regional variants are deliberately absent: the RPC takes a
// two-letter code, and offering something it will refuse is not a choice.
var catalog = []Language{
	{Code: "aa", Name: "Afar"},
	{Code: "ab", Name: "Abkhazian"},
	{Code: "af", Name: "Afrikaans"},
	{Code: "ak", Name: "Akan"},
	{Code: "sq", Name: "Albanian"},
	{Code: "am", Name: "Amharic"},
	{Code: "ar", Name: "Arabic"},
	{Code: "an", Name: "Aragonese"},
	{Code: "hy", Name: "Armenian"},
	{Code: "as", Name: "Assamese"},
	{Code: "av", Name: "Avaric"},
	{Code: "ae", Name: "Avestan"},
	{Code: "ay", Name: "Aymara"},
	{Code: "az", Name: "Azerbaijani"},
	{Code: "bm", Name: "Bambara"},
	{Code: "ba", Name: "Bashkir"},
	{Code: "eu", Name: "Basque"},
	{Code: "be", Name: "Belarusian"},
	{Code: "bn", Name: "Bengali"},
	{Code: "bh", Name: "Bihari languages"},
	{Code: "bi", Name: "Bislama"},
	{Code: "bs", Name: "Bosnian"},
	{Code: "br", Name: "Breton"},
	{Code: "bg", Name: "Bulgarian"},
	{Code: "my", Name: "Burmese"},
	{Code: "ca", Name: "Catalan"},
	{Code: "km", Name: "Central Khmer"},
	{Code: "ch", Name: "Chamorro"},
	{Code: "ce", Name: "Chechen"},
	{Code: "ny", Name: "Chichewa"},
	{Code: "zh", Name: "Chinese"},
	{Code: "cu", Name: "Church Slavic"},
	{Code: "cv", Name: "Chuvash"},
	{Code: "kw", Name: "Cornish"},
	{Code: "co", Name: "Corsican"},
	{Code: "cr", Name: "Cree"},
	{Code: "hr", Name: "Croatian"},
	{Code: "cs", Name: "Czech"},
	{Code: "da", Name: "Danish"},
	{Code: "dv", Name: "Divehi"},
	{Code: "nl", Name: "Dutch"},
	{Code: "dz", Name: "Dzongkha"},
	{Code: "en", Name: "English"},
	{Code: "eo", Name: "Esperanto"},
	{Code: "et", Name: "Estonian"},
	{Code: "ee", Name: "Ewe"},
	{Code: "fo", Name: "Faroese"},
	{Code: "fj", Name: "Fijian"},
	{Code: "fi", Name: "Finnish"},
	{Code: "fr", Name: "French"},
	{Code: "ff", Name: "Fulah"},
	{Code: "gd", Name: "Gaelic"},
	{Code: "gl", Name: "Galician"},
	{Code: "lg", Name: "Ganda"},
	{Code: "ka", Name: "Georgian"},
	{Code: "de", Name: "German"},
	{Code: "el", Name: "Greek"},
	{Code: "gn", Name: "Guarani"},
	{Code: "gu", Name: "Gujarati"},
	{Code: "ht", Name: "Haitian"},
	{Code: "ha", Name: "Hausa"},
	{Code: "he", Name: "Hebrew"},
	{Code: "hz", Name: "Herero"},
	{Code: "hi", Name: "Hindi"},
	{Code: "ho", Name: "Hiri Motu"},
	{Code: "hu", Name: "Hungarian"},
	{Code: "is", Name: "Icelandic"},
	{Code: "io", Name: "Ido"},
	{Code: "ig", Name: "Igbo"},
	{Code: "id", Name: "Indonesian"},
	{Code: "ia", Name: "Interlingua"},
	{Code: "ie", Name: "Interlingue"},
	{Code: "iu", Name: "Inuktitut"},
	{Code: "ik", Name: "Inupiaq"},
	{Code: "ga", Name: "Irish"},
	{Code: "it", Name: "Italian"},
	{Code: "ja", Name: "Japanese"},
	{Code: "jv", Name: "Javanese"},
	{Code: "kl", Name: "Kalaallisut"},
	{Code: "kn", Name: "Kannada"},
	{Code: "kr", Name: "Kanuri"},
	{Code: "ks", Name: "Kashmiri"},
	{Code: "kk", Name: "Kazakh"},
	{Code: "ki", Name: "Kikuyu"},
	{Code: "rw", Name: "Kinyarwanda"},
	{Code: "ky", Name: "Kirghiz"},
	{Code: "kv", Name: "Komi"},
	{Code: "kg", Name: "Kongo"},
	{Code: "ko", Name: "Korean"},
	{Code: "kj", Name: "Kuanyama"},
	{Code: "ku", Name: "Kurdish"},
	{Code: "lo", Name: "Lao"},
	{Code: "la", Name: "Latin"},
	{Code: "lv", Name: "Latvian"},
	{Code: "li", Name: "Limburgan"},
	{Code: "ln", Name: "Lingala"},
	{Code: "lt", Name: "Lithuanian"},
	{Code: "lu", Name: "Luba-Katanga"},
	{Code: "lb", Name: "Luxembourgish"},
	{Code: "mk", Name: "Macedonian"},
	{Code: "mg", Name: "Malagasy"},
	{Code: "ms", Name: "Malay"},
	{Code: "ml", Name: "Malayalam"},
	{Code: "mt", Name: "Maltese"},
	{Code: "gv", Name: "Manx"},
	{Code: "mi", Name: "Maori"},
	{Code: "mr", Name: "Marathi"},
	{Code: "mh", Name: "Marshallese"},
	{Code: "mn", Name: "Mongolian"},
	{Code: "na", Name: "Nauru"},
	{Code: "nv", Name: "Navajo"},
	{Code: "nd", Name: "North Ndebele"},
	{Code: "nr", Name: "South Ndebele"},
	{Code: "ng", Name: "Ndonga"},
	{Code: "ne", Name: "Nepali"},
	{Code: "se", Name: "Northern Sami"},
	{Code: "no", Name: "Norwegian"},
	{Code: "nb", Name: "Norwegian Bokmål"},
	{Code: "nn", Name: "Norwegian Nynorsk"},
	{Code: "oc", Name: "Occitan"},
	{Code: "oj", Name: "Ojibwa"},
	{Code: "or", Name: "Oriya"},
	{Code: "om", Name: "Oromo"},
	{Code: "os", Name: "Ossetian"},
	{Code: "pi", Name: "Pali"},
	{Code: "pa", Name: "Panjabi"},
	{Code: "fa", Name: "Persian"},
	{Code: "pl", Name: "Polish"},
	{Code: "pt", Name: "Portuguese"},
	{Code: "ps", Name: "Pushto"},
	{Code: "qu", Name: "Quechua"},
	{Code: "rm", Name: "Romansh"},
	{Code: "ro", Name: "Romanian"},
	{Code: "rn", Name: "Rundi"},
	{Code: "ru", Name: "Russian"},
	{Code: "sm", Name: "Samoan"},
	{Code: "sg", Name: "Sango"},
	{Code: "sa", Name: "Sanskrit"},
	{Code: "sc", Name: "Sardinian"},
	{Code: "sr", Name: "Serbian"},
	{Code: "sn", Name: "Shona"},
	{Code: "ii", Name: "Sichuan Yi"},
	{Code: "sd", Name: "Sindhi"},
	{Code: "si", Name: "Sinhala"},
	{Code: "sk", Name: "Slovak"},
	{Code: "sl", Name: "Slovenian"},
	{Code: "so", Name: "Somali"},
	{Code: "st", Name: "Southern Sotho"},
	{Code: "es", Name: "Spanish"},
	{Code: "su", Name: "Sundanese"},
	{Code: "sw", Name: "Swahili"},
	{Code: "ss", Name: "Swati"},
	{Code: "sv", Name: "Swedish"},
	{Code: "tl", Name: "Tagalog"},
	{Code: "ty", Name: "Tahitian"},
	{Code: "tg", Name: "Tajik"},
	{Code: "ta", Name: "Tamil"},
	{Code: "tt", Name: "Tatar"},
	{Code: "te", Name: "Telugu"},
	{Code: "th", Name: "Thai"},
	{Code: "bo", Name: "Tibetan"},
	{Code: "ti", Name: "Tigrinya"},
	{Code: "to", Name: "Tonga"},
	{Code: "ts", Name: "Tsonga"},
	{Code: "tn", Name: "Tswana"},
	{Code: "tr", Name: "Turkish"},
	{Code: "tk", Name: "Turkmen"},
	{Code: "tw", Name: "Twi"},
	{Code: "ug", Name: "Uighur"},
	{Code: "uk", Name: "Ukrainian"},
	{Code: "ur", Name: "Urdu"},
	{Code: "uz", Name: "Uzbek"},
	{Code: "ve", Name: "Venda"},
	{Code: "vi", Name: "Vietnamese"},
	{Code: "vo", Name: "Volapük"},
	{Code: "wa", Name: "Walloon"},
	{Code: "cy", Name: "Welsh"},
	{Code: "fy", Name: "Western Frisian"},
	{Code: "wo", Name: "Wolof"},
	{Code: "xh", Name: "Xhosa"},
	{Code: "yi", Name: "Yiddish"},
	{Code: "yo", Name: "Yoruba"},
	{Code: "za", Name: "Zhuang"},
	{Code: "zu", Name: "Zulu"},
}

// languages is the catalog in the order a person meets it: by English name, so
// the settings overlay's list reads as a list rather than as a run of codes.
var languages = sortedByName(catalog)

// names maps a code to its English name, for the marker a translated bubble
// carries and for anything else that has only the stored code in hand.
var names = indexNames(languages)

func sortedByName(in []Language) []Language {
	out := append([]Language(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func indexNames(in []Language) map[string]string {
	out := make(map[string]string, len(in))
	for _, l := range in {
		out[l.Code] = l.Name
	}
	return out
}

// Languages returns the catalog, ordered by English name. The slice is a copy:
// a caller that sorts or trims it is not editing the catalog.
func Languages() []Language { return append([]Language(nil), languages...) }

// Codes returns the ISO 639-1 codes, in the same order as Languages. This is
// what a setting stores and what Telegram is given.
func Codes() []string {
	out := make([]string, 0, len(languages))
	for _, l := range languages {
		out = append(out, l.Code)
	}
	return out
}

// Labels maps each code to how it is shown where a code alone would be a
// puzzle: the English name with the code beside it, so a person who knows the
// language by either one can find it.
func Labels() map[string]string {
	out := make(map[string]string, len(languages))
	for _, l := range languages {
		out[l.Code] = label(l)
	}
	return out
}

// Name returns the bare English name of a code, and the code itself when the
// catalog has no such language. The fallback is deliberate: a config naming a
// code this build does not know is repairable, and a wrong name invented for it
// would hide which code was actually in force.
func Name(code string) string {
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

// Default is the language translation starts in when nothing has chosen one.
const Default = "pl"

func label(l Language) string { return l.Name + " (" + l.Code + ")" }
