package contacts

import (
	"reflect"
	"testing"
)

func TestParseCustomFieldsSortsAndDeduplicates(t *testing.T) {
	fields, err := ParseCustomFields([]string{
		"tier=gold",
		"region=NA,team=platform",
		"tier=platinum",
	})
	if err != nil {
		t.Fatalf("ParseCustomFields failed: %v", err)
	}

	want := []CustomField{
		{Key: "region", Value: "NA"},
		{Key: "team", Value: "platform"},
		{Key: "tier", Value: "platinum"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("unexpected fields: got %#v want %#v", fields, want)
	}
}

func TestParseCustomFieldsRejectsInvalidInput(t *testing.T) {
	_, err := ParseCustomFields([]string{"not-a-pair"})
	if err == nil {
		t.Fatal("expected ParseCustomFields to fail for invalid input")
	}
}

func TestMergeCustomFieldCategoriesPreservesNonCustomValues(t *testing.T) {
	existing := append([]string{"Friends"}, EncodeCustomFieldCategories([]CustomField{{Key: "old", Value: "1"}})...)
	merged := MergeCustomFieldCategories(existing, []CustomField{{Key: "new", Value: "2"}})

	want := append([]string{"Friends"}, EncodeCustomFieldCategories([]CustomField{{Key: "new", Value: "2"}})...)
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("unexpected merged categories: got %#v want %#v", merged, want)
	}
}

func TestAddNormalizedCustomFieldsAddsSortedCustomSlice(t *testing.T) {
	item := map[string]any{
		"id": "contact-id",
		"categories": []any{
			"Friends",
			"mog.custom.team=platform",
			"mog.custom.region=NA",
		},
	}

	addNormalizedCustomFields(item)

	custom, ok := item["custom"].([]CustomField)
	if !ok {
		t.Fatalf("expected custom field slice, got %#v", item["custom"])
	}

	want := []CustomField{
		{Key: "region", Value: "NA"},
		{Key: "team", Value: "platform"},
	}
	if !reflect.DeepEqual(custom, want) {
		t.Fatalf("unexpected custom fields: got %#v want %#v", custom, want)
	}
}
