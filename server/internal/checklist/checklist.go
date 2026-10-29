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

// Set returns the plan with the items at the given indexes ticked or unticked, leaving every
// other line exactly as it was. It fails when an index has no item.
func Set(plan string, changes map[int]bool) (string, error) {
	lines := strings.Split(plan, "\n")
	found := map[int]bool{}

	index := 0
	forEachItem(plan, func(line int, match []string) {
		if checked, ok := changes[index]; ok {
			box := " "
			if checked {
				box = "x"
			}
			lines[line] = match[1] + box + match[3] + match[4]
			found[index] = true
		}
		index++
	})

	for wanted := range changes {
		if !found[wanted] {
			return "", fmt.Errorf("the plan has no checklist item %d; it has %d", wanted, index)
		}
	}
	return strings.Join(lines, "\n"), nil
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
