package checklist

import (
	"slices"
	"testing"
)

const plan = "## Checks\n\n- [ ] Add the migration\n* [x] Write the store\n1. [X] Numbered\n\n```md\n- [ ] Not a task\n```\n\n  - [ ] Nested task\n- [] not a box\n"

func TestItemsSkipsCodeBlocks(t *testing.T) {
	items := Items(plan)

	want := []Item{
		{Index: 0, Text: "Add the migration"},
		{Index: 1, Text: "Write the store", Checked: true},
		{Index: 2, Text: "Numbered", Checked: true},
		{Index: 3, Text: "Nested task"},
	}
	if !slices.Equal(items, want) {
		t.Fatalf("Items = %+v, want %+v", items, want)
	}
}

func TestSetChangesOnlyTheNamedItems(t *testing.T) {
	updated, err := Set(plan, map[int]bool{0: true, 1: false, 3: true})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := "## Checks\n\n- [x] Add the migration\n* [ ] Write the store\n1. [X] Numbered\n\n```md\n- [ ] Not a task\n```\n\n  - [x] Nested task\n- [] not a box\n"
	if updated != want {
		t.Fatalf("Set =\n%q\nwant\n%q", updated, want)
	}
}

func TestSetRefusesAMissingItem(t *testing.T) {
	if _, err := Set(plan, map[int]bool{4: true}); err == nil {
		t.Fatal("Set accepted item 4 of 4")
	}
}
