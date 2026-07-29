package teams

import "testing"

func TestParseChatMentions(t *testing.T) {
	t.Parallel()

	got, err := ParseChatMentions([]string{
		"Jane Doe:aad-user-1",
		"Ops: Lead:aad-user-2",
	})
	if err != nil {
		t.Fatalf("ParseChatMentions failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two mentions, got %#v", got)
	}
	if got[0].DisplayName != "Jane Doe" || got[0].UserID != "aad-user-1" {
		t.Fatalf("unexpected first mention: %#v", got[0])
	}
	if got[1].DisplayName != "Ops: Lead" || got[1].UserID != "aad-user-2" {
		t.Fatalf("unexpected second mention: %#v", got[1])
	}
}

func TestParseChatMentionRejectsInvalidSpecs(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"",
		"Jane Doe",
		":aad-user-1",
		"Jane Doe:",
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseChatMention(tc); err == nil {
				t.Fatalf("expected %q to be invalid", tc)
			}
		})
	}
}

func TestBuildMentionedChatContentUsesExistingMatchingTags(t *testing.T) {
	t.Parallel()

	mentions := []ChatMention{{DisplayName: "Jane Doe", UserID: "aad-user-1"}}
	got := buildMentionedChatContent(`Already tagged <at id="0">Jane Doe</at>`, "html", mentions)
	want := `Already tagged <at id="0">Jane Doe</at>`
	if got != want {
		t.Fatalf("unexpected content:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestBuildMentionedChatContentEscapesTextBodies(t *testing.T) {
	t.Parallel()

	mentions := []ChatMention{{DisplayName: "Jane & Co", UserID: "aad-user-1"}}
	got := buildMentionedChatContent("Hello <team>\nPlease review", "text", mentions)
	want := `Hello &lt;team&gt;<br>Please review<br><at id="0">Jane &amp; Co</at>`
	if got != want {
		t.Fatalf("unexpected content:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestBuildChatMentionsPayloadShape(t *testing.T) {
	t.Parallel()

	payload, err := buildChatMentionsPayload("Hello", "text", []ChatMention{
		{DisplayName: "Jane Doe", UserID: "aad-user-1"},
	})
	if err != nil {
		t.Fatalf("buildChatMentionsPayload failed: %v", err)
	}

	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body map, got %#v", payload["body"])
	}
	if body["contentType"] != "html" {
		t.Fatalf("unexpected contentType: %#v", body["contentType"])
	}
	if body["content"] != `Hello<br><at id="0">Jane Doe</at>` {
		t.Fatalf("unexpected content: %#v", body["content"])
	}

	mentions, ok := payload["mentions"].([]map[string]any)
	if !ok || len(mentions) != 1 {
		t.Fatalf("unexpected mentions: %#v", payload["mentions"])
	}
	if mentions[0]["id"] != 0 || mentions[0]["mentionText"] != "Jane Doe" {
		t.Fatalf("unexpected mention payload: %#v", mentions[0])
	}
	mentioned, ok := mentions[0]["mentioned"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected mentioned payload: %#v", mentions[0]["mentioned"])
	}
	user, ok := mentioned["user"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected mentioned user: %#v", mentioned["user"])
	}
	if user["displayName"] != "Jane Doe" || user["id"] != "aad-user-1" || user["userIdentityType"] != "aadUser" {
		t.Fatalf("unexpected mentioned user payload: %#v", user)
	}
}

func TestBuildChatMentionsPayloadNoMentionsShapeUnchanged(t *testing.T) {
	t.Parallel()

	payload, err := buildChatMentionsPayload(" <b>Hello</b> ", "html", nil)
	if err != nil {
		t.Fatalf("buildChatMentionsPayload failed: %v", err)
	}

	body, ok := payload["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body map, got %#v", payload["body"])
	}
	if body["contentType"] != "html" || body["content"] != "<b>Hello</b>" {
		t.Fatalf("unexpected body payload: %#v", body)
	}
	if _, ok := payload["mentions"]; ok {
		t.Fatalf("expected no mentions key, got %#v", payload["mentions"])
	}
}
