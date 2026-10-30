package store

import "slices"

var profileAvatarIDs = []string{
	"adventurers-cat-pilot",
	"adventurers-dragon-ember",
	"adventurers-fox-rogue",
	"adventurers-frog-adventurer",
	"adventurers-owl-guardian",
	"adventurers-rabbit-scholar",
	"adventurers-wolf-shadow",
	"approved-samples-cloud-companion",
	"approved-samples-sparrow-explorer",
	"forest-badger-warrior",
	"forest-fox-rogue",
	"forest-frog-adventurer",
	"forest-lynx-shadow",
	"forest-owl-sage",
	"forest-squirrel-maker",
	"forest-wolf-guardian",
	"magical-dragon",
	"magical-forest-spirit",
	"magical-moon-spirit",
	"magical-phoenix",
	"magical-shadow-cat",
	"magical-storm-spirit",
	"magical-sun-spirit",
	"nerdy-dragon-alchemist",
	"nerdy-fox-librarian",
	"nerdy-owl-astronomer",
	"nerdy-raccoon-coder",
	"nerdy-sparrow-inventor",
	"sparrows-sparrow-01",
	"sparrows-sparrow-02",
	"sparrows-sparrow-03",
	"sparrows-sparrow-04",
	"sparrows-sparrow-05",
	"sparrows-sparrow-06",
	"sparrows-sparrow-07",
}

// ProfileAvatarIDs returns the built-in avatar identifiers available to human profiles.
func ProfileAvatarIDs() []string {
	return slices.Clone(profileAvatarIDs)
}
