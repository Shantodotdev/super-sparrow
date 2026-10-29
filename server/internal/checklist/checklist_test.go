package checklist

import (
	"errors"
	"slices"
	"strings"
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
	updated, err := Set(plan, map[int]Change{
		0: {Checked: true, Text: "Add the migration"},
		1: {Checked: false, Text: "Write the store"},
		3: {Checked: true, Text: " Nested task "},
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := "## Checks\n\n- [x] Add the migration\n* [ ] Write the store\n1. [X] Numbered\n\n```md\n- [ ] Not a task\n```\n\n  - [x] Nested task\n- [] not a box\n"
	if updated != want {
		t.Fatalf("Set =\n%q\nwant\n%q", updated, want)
	}
}

func TestSetRefusesAMissingItem(t *testing.T) {
	if _, err := Set(plan, map[int]Change{4: {Checked: true, Text: "Anything"}}); err == nil {
		t.Fatal("Set accepted item 4 of 4")
	}
}

func isItemChanged(err error) bool {
	_, ok := errors.AsType[*ItemChangedError](err)
	return ok
}

func TestSetRefusesAnItemThatMoved(t *testing.T) {
	// Someone added an item at the top after the caller read the plan.
	edited := "- [ ] Write the plan\n" + plan

	_, err := Set(edited, map[int]Change{0: {Checked: true, Text: "Add the migration"}})
	if !isItemChanged(err) || !strings.Contains(err.Error(), "it is item 1 now") {
		t.Fatalf("Set on a moved item = %v", err)
	}

	_, err = Set(plan, map[int]Change{0: {Checked: true, Text: "Removed item"}})
	if !isItemChanged(err) || !strings.Contains(err.Error(), "no longer in the plan") {
		t.Fatalf("Set on a removed item = %v", err)
	}

	// One refused change applies none of the others.
	if _, err := Set(edited, map[int]Change{1: {Checked: true, Text: "Add the migration"}, 2: {Text: "Wrong"}}); err == nil {
		t.Fatal("Set applied a batch with a moved item")
	}
}
