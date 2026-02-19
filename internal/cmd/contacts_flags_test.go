package cmd

import (
	"reflect"
	"testing"
)

func TestContactsCreateExtendedFlagsParse(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"contacts", "create",
		"--display-name", "Jane Doe",
		"--email", "jane@example.com",
		"--mobile-phone", "+1-555-0100",
		"--org", "Contoso",
		"--title", "Engineer",
		"--url", "https://example.com",
		"--note", "Team lead",
		"--custom", "team=platform",
		"--custom", "region=NA",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if cli.Contacts.Create.Org != "Contoso" {
		t.Fatalf("unexpected org: %q", cli.Contacts.Create.Org)
	}
	if cli.Contacts.Create.Title != "Engineer" {
		t.Fatalf("unexpected title: %q", cli.Contacts.Create.Title)
	}
	if cli.Contacts.Create.URL != "https://example.com" {
		t.Fatalf("unexpected url: %q", cli.Contacts.Create.URL)
	}
	if cli.Contacts.Create.Note != "Team lead" {
		t.Fatalf("unexpected note: %q", cli.Contacts.Create.Note)
	}
	if !reflect.DeepEqual(cli.Contacts.Create.Custom, []string{"team=platform", "region=NA"}) {
		t.Fatalf("unexpected custom flags: %#v", cli.Contacts.Create.Custom)
	}
}

func TestContactsUpdateExtendedFlagsParse(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"contacts", "update", "contact-id",
		"--org", "Contoso",
		"--title", "Manager",
		"--url", "https://example.com/profile",
		"--note", "Updated notes",
		"--custom", "tier=gold",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if cli.Contacts.Update.Org != "Contoso" {
		t.Fatalf("unexpected org: %q", cli.Contacts.Update.Org)
	}
	if cli.Contacts.Update.Title != "Manager" {
		t.Fatalf("unexpected title: %q", cli.Contacts.Update.Title)
	}
	if cli.Contacts.Update.URL != "https://example.com/profile" {
		t.Fatalf("unexpected url: %q", cli.Contacts.Update.URL)
	}
	if cli.Contacts.Update.Note != "Updated notes" {
		t.Fatalf("unexpected note: %q", cli.Contacts.Update.Note)
	}
	if !reflect.DeepEqual(cli.Contacts.Update.Custom, []string{"tier=gold"}) {
		t.Fatalf("unexpected custom flags: %#v", cli.Contacts.Update.Custom)
	}
}
