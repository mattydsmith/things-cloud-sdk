package main

import "testing"

func TestGenerateUUID_AlwaysExactly22Chars(t *testing.T) {
	t.Parallel()

	// Generate enough UUIDs to reliably hit the small-leading-bytes case
	// (~1.4% probability, so 100k iterations covers it ~1400 times).
	for i := 0; i < 100_000; i++ {
		got := generateUUID()
		if len(got) != 22 {
			t.Fatalf("iteration %d: generateUUID() = %q (len %d), want length 22", i, got, len(got))
		}
		if !isBase58UUID(got) {
			t.Fatalf("iteration %d: generateUUID() = %q, fails isBase58UUID", i, got)
		}
	}
}
