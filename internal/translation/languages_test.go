package translation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A duplicated code would key two rows of the settings choice list to the same
// stored value, so choosing either would land on the other. The catalog is a
// table, and a table's one invariant is that its key column is a key.
func TestLanguages_HaveUniqueCodesAndNames(t *testing.T) {
	seenCode := make(map[string]int, len(languages))
	seenName := make(map[string]int, len(languages))
	for _, l := range languages {
		require.Len(t, l.Code, 2, "%s (%q) is not an ISO 639-1 code", l.Name, l.Code)
		assert.Equal(t, l.Code, lower(l.Code), "%q must be written in lower case, the way the RPC takes it")
		assert.NotContains(t, seenCode, l.Code, "%q appears twice", l.Code)
		assert.NotContains(t, seenName, l.Name, "%q names two codes", l.Name)
		assert.NotEmpty(t, l.Name, "%q has no English name", l.Code)
		seenCode[l.Code] = 1
		seenName[l.Name] = 1
	}
	assert.NotEmpty(t, languages, "the catalog must not be empty")
}

// The list is offered to a person, so it reads as a list: by English name.
func TestLanguages_AreOrderedByEnglishName(t *testing.T) {
	for i := 1; i < len(languages); i++ {
		require.LessOrEqual(t, languages[i-1].Name, languages[i].Name,
			"%q comes before %q", languages[i-1].Name, languages[i].Name)
	}
}

// Every helper describes the same table: a code that Languages offers is a code
// Codes accepts, a code Labels can label, and a code Name names.
func TestHelpers_DescribeTheSameCatalog(t *testing.T) {
	all := Languages()
	codes := Codes()
	labels := Labels()

	require.Len(t, codes, len(all))
	require.Len(t, labels, len(all))

	for i, l := range all {
		assert.Equal(t, l.Code, codes[i], "Codes follows Languages")
		assert.Equal(t, l.Name+" ("+l.Code+")", labels[l.Code])
		assert.Equal(t, l.Name, Name(l.Code))
	}
}

// A code the catalog does not carry is returned as itself: the person can then
// see which code is in force, rather than a name invented for it.
func TestName_UnknownCodeIsItsOwnName(t *testing.T) {
	assert.Equal(t, "xx", Name("xx"))
	assert.Equal(t, "", Name(""))
}

// The copy-returning helpers must not hand out the catalog itself: a caller
// sorting or trimming what it was given would be editing what every other caller
// reads.
func TestHelpers_ReturnCopies(t *testing.T) {
	first := Languages()
	require.NotEmpty(t, first)
	original := first[0]
	first[0] = Language{Code: "zz", Name: "Zzz"}
	assert.Equal(t, original, Languages()[0], "Languages handed out the catalog")

	names := Codes()
	require.NotEmpty(t, names)
	names[0] = "zz"
	assert.NotEqual(t, names[0], Codes()[0], "Codes handed out the catalog")

	byCode := Labels()
	require.NotEmpty(t, byCode)
	key := Codes()[0]
	byCode[key] = "vandalised"
	assert.NotEqual(t, "vandalised", Labels()[key], "Labels handed out the map it keeps")
}

// The default is a language, and one of the ones offered: a default the choice
// list does not carry would be refused by the setting that stores it.
func TestDefault_IsOneOfTheLanguages(t *testing.T) {
	assert.Equal(t, "pl", Default)
	assert.Contains(t, Codes(), Default)
	assert.Equal(t, "Polish", Name(Default))
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}
	return string(out)
}
