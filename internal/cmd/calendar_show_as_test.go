package cmd

import "testing"

func TestNormalizeCalendarShowAs(t *testing.T) {
	testCases := map[string]string{
		"free":             "free",
		"Tentative":        "tentative",
		"busy":             "busy",
		"OOF":              "oof",
		"workingElsewhere": "workingElsewhere",
		"unknown":          "unknown",
	}

	for input, expected := range testCases {
		actual, err := normalizeCalendarShowAs(input)
		if err != nil {
			t.Fatalf("normalizeCalendarShowAs(%q) returned error: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalizeCalendarShowAs(%q) = %q, want %q", input, actual, expected)
		}
	}

	if _, err := normalizeCalendarShowAs("maybe"); err == nil {
		t.Fatal("normalizeCalendarShowAs accepted an invalid value")
	}
}
