package plugin

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPreviewRequiresTextAndObject(t *testing.T) {
	for _, raw := range []string{`{"preview":{"kind":"markdown"}}`, `{"preview":[]}`, `{"preview":{"kind":"markdown","text":null}}`} {
		var result Result
		if err := json.Unmarshal([]byte(raw), &result); err == nil {
			t.Fatal("invalid preview shape accepted", raw)
		}
	}
	var result Result
	if err := json.Unmarshal([]byte(`{"preview":{"kind":"markdown","text":""}}`), &result); err != nil {
		t.Fatal("empty Markdown preview should be valid", err)
	}
}

func TestMarkdownPreviewProtocolAndBounds(t *testing.T) {
	for _, mode := range []string{"preview", "bad-preview", "large-preview"} {
		t.Run(mode, func(t *testing.T) {
			m, entry := fixture(t, mode, []string{ReadDocument})
			enable(t, m, entry)
			result, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run"})
			if mode == "preview" {
				if err != nil || result.Preview == nil || result.Preview.Kind != "markdown" || result.Preview.Text != "# Heading" {
					t.Fatal("valid preview rejected", result, err)
				}
				waitState(t, m, Ready)
			} else {
				if err == nil {
					t.Fatal("invalid preview accepted", mode)
				}
				waitState(t, m, Restarting)
			}
		})
	}
}
