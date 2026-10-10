package schema

import "testing"

func TestListingContentHashNormalizesBodyMarkupAndWhitespace(t *testing.T) {
	left := ListingContentHash("Engineer", "$100k", "<p>Hello    world</p>")
	right := ListingContentHash("Engineer", "$100k", "Hello world")
	if left != right {
		t.Fatalf("markup-only body change changed hash: %s != %s", left, right)
	}
	changed := ListingContentHash("Engineer", "$120k", "Hello world")
	if changed == left {
		t.Fatal("salary content change did not change hash")
	}
}
