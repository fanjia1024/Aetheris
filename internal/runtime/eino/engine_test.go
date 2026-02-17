package eino

import "testing"

func TestParseDefaultKey(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		provider, modelKey, err := parseDefaultKey("openai.gpt_35_turbo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider != "openai" || modelKey != "gpt_35_turbo" {
			t.Fatalf("unexpected parsed result: provider=%q modelKey=%q", provider, modelKey)
		}
	})

	invalidKeys := []string{
		"",
		"openai",
		".gpt_35_turbo",
		"openai.",
	}
	for _, key := range invalidKeys {
		t.Run("invalid_"+key, func(t *testing.T) {
			_, _, err := parseDefaultKey(key)
			if err == nil {
				t.Fatalf("expected error for key %q", key)
			}
		})
	}
}
