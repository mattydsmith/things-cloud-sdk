package thingscloud

import (
	"fmt"
	"testing"
)

func TestHistory_Items(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		server := fakeServer(fakeResponse{200, "history-items-success.json"})
		defer server.Close()

		c := New(fmt.Sprintf("http://%s", server.Listener.Addr().String()), "martin@example.com", "")
		h := &History{
			Client: c,
			ID:     "33333abb-bfe4-4b03-a5c9-106d42220c72",
		}
		items, _, err := h.Items(ItemsOptions{})
		if err != nil {
			t.Fatalf("Expected items request to succeed, but didn't: %q", err.Error())
		}

		if len(items) < 1 {
			t.Fatalf("Expected items, but got none: %#v", items)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()
		server := fakeBodyServer(200, "{")
		defer server.Close()

		c := New(fmt.Sprintf("http://%s", server.Listener.Addr().String()), "martin@example.com", "")
		h := &History{Client: c, ID: "33333abb-bfe4-4b03-a5c9-106d42220c72"}
		if _, _, err := h.Items(ItemsOptions{}); err == nil {
			t.Fatal("Expected malformed items JSON to fail")
		}
	})

	t.Run("InvalidRequest", func(t *testing.T) {
		t.Parallel()
		c := New("http://example.com", "martin@example.com", "")
		h := &History{Client: c, ID: "bad\nid"}
		if _, _, err := h.Items(ItemsOptions{}); err == nil {
			t.Fatal("Expected invalid history ID to return an error")
		}
	})
}
