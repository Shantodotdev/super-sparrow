package keyboard

import "testing"

func TestKeyboardDefaultsAndOverrides(t *testing.T) {
	if err := Validate(nil); err != nil {
		t.Fatal(err)
	}
	if Definitions()["quickCapture"].Binding != "Meta+Shift+Q" {
		t.Fatal("incorrect capture default")
	}
	for _, overrides := range []map[string]string{
		{"quickCapture": "Ctrl+Alt+K"},
		{"quickCapture": ""},
		{"goIssues": "Ctrl+Alt+I G"},
		{"newIssue": "Ctrl+Alt+C", "palette": "Meta+Shift+K"},
	} {
		if err := Validate(overrides); err != nil {
			t.Fatalf("%v: %v", overrides, err)
		}
	}
}

func TestKeyboardRejectsCollisionsAndInvalidChords(t *testing.T) {
	cases := []map[string]string{
		{"unknown": "Ctrl+Q"}, {"newIssue": "Escape"}, {"newIssue": "Tab"},
		{"quickCapture": "Q"}, {"quickCapture": "Shift+Q"}, {"quickCapture": "Mod+Q"},
		{"quickCapture": "Meta+Shift+Q A"}, {"quickCapture": "Meta+Ctrl+Q"},
		{"palette": "Ctrl+Ctrl+Q"}, {"palette": "Ctrl+Mod+Q"},
		{"submit": "C"}, {"findingFix": "G F"},
		{"newIssue": "J"}, {"newIssue": "G"},
		{"quickCapture": "Meta+K"}, {"goMine": "G I"},
	}
	for _, overrides := range cases {
		if err := Validate(overrides); err == nil {
			t.Errorf("accepted %v", overrides)
		}
	}
}
