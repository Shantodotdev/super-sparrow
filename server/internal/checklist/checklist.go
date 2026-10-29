// Package checklist reads and ticks the task-list items in a markdown plan, such as
// "- [ ] Add the migration".
package checklist

import (
	"fmt"
	"regexp"
	"strings"
)

// itemPattern matches a task-list item: a bullet or number, a box, and the item's text.
var itemPattern = regexp.MustCompile(`^(\s*(?:[-*+]|\d{1,9}[.)])\s+\[)([ xX])(\]\s+)(.*)$`)

// Item is one checklist item. Index counts items from 0 in the order they appear.
type Item struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// Items returns the plan's checklist items. Items inside fenced code blocks are examples, not
// tasks, so they are skipped.
func Items(plan string) []Item {
	items := []Item{}
	forEachItem(plan, func(_ int, match []string) {
		items = append(items, Item{Index: len(items), Text: strings.TrimSpace(match[4]), Checked: match[2] != " "})
	})
	return items
}

// Change ticks or unticks one item. Text is the item's text as the caller last read it, which
// must still be the text at that index, so a change never lands on another item after the plan
// was edited.
type Change struct {
	Checked bool
	Text    string
}

// ItemChangedError is returned by Set when the item at an index no longer has the text the caller
// read, because the plan was edited in between. Its message says where the item is now.
type ItemChangedError struct {
	Message string
}

func (e *ItemChangedError) Error() string { return e.Message }

// Set returns the plan with the items at the given indexes ticked or unticked, leaving every
// other line exactly as it was. It fails when an index has no item, and with an *ItemChangedError
// when an item's text is not the text the change names, saying where the item is now if it is
// still in the plan.
func Set(plan string, changes map[int]Change) (string, error) {
	lines := strings.Split(plan, "\n")
	items := map[int]Item{}
	itemLines := map[int]int{}
	matches := map[int][]string{}

	index := 0
	forEachItem(plan, func(line int, match []string) {
		items[index] = Item{Index: index, Text: strings.TrimSpace(match[4]), Checked: match[2] != " "}
		itemLines[index], matches[index] = line, match
		index++
	})

	// Check every change before applying any, so a refused change leaves the plan untouched.
	for wanted, change := range changes {
		item, ok := items[wanted]
		if !ok {
			return "", fmt.Errorf("the plan has no checklist item %d; it has %d", wanted, index)
		}
		if text := strings.TrimSpace(change.Text); item.Text != text {
			return "", movedError(wanted, text, item.Text, items)
		}
	}

	for wanted, change := range changes {
		box := " "
		if change.Checked {
			box = "x"
		}
		match := matches[wanted]
		lines[itemLines[wanted]] = match[1] + box + match[3] + match[4]
	}
	return strings.Join(lines, "\n"), nil
}

// movedError explains that item index is no longer the item with text, and where that item is
// now when exactly one item still has its text.
func movedError(index int, text, current string, items map[int]Item) error {
	var found []int
	for _, item := range items {
		if item.Text == text {
			found = append(found, item.Index)
		}
	}

	where := "it is no longer in the plan"
	if len(found) == 1 {
		where = fmt.Sprintf("it is item %d now", found[0])
	}
	return &ItemChangedError{Message: fmt.Sprintf("item %d is %q, not %q; %s, so read the plan again", index, current, text, where)}
}

// forEachItem calls fn with the line number and pattern match of every item outside fenced
// code blocks.
func forEachItem(plan string, fn func(line int, match []string)) {
	fence := ""
	for number, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
			continue
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
			continue
		}

		if match := itemPattern.FindStringSubmatch(strings.TrimRight(line, "\r")); match != nil {
			fn(number, match)
		}
	}
}
