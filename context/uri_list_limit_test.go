// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// "limit" is optional, so the caller passes 0 when it is absent. Truncating on
// that would return an empty collection for a request that asked for no limit
// at all, and a negative value used to reach make() with a negative length and
// panic.
func TestNnrfUriListLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		items int
		want  int
	}{
		{"no limit requested", 0, 5, 5},
		{"negative limit does not panic or empty the list", -1, 5, 5},
		{"limit below the item count truncates", 3, 5, 3},
		{"limit above the item count keeps everything", 9, 5, 5},
		{"limit equal to the item count keeps everything", 5, 5, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uriList := &UriList{}
			uriList.Links.Item = make([]models.Link, tc.items)

			NnrfUriListLimit(uriList, tc.limit)

			if got := len(uriList.Links.Item); got != tc.want {
				t.Errorf("items = %d, want %d", got, tc.want)
			}
		})
	}
}

const itemKey = "item"

func TestDecodeUriListAcceptsLegacyLinkMember(t *testing.T) {
	item := func(href string) []map[string]any { return []map[string]any{{"href": href}} }
	tests := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{"current member", map[string]any{"_links": map[string]any{itemKey: item("a")}}, "a"},
		{"legacy member", map[string]any{"_link": map[string]any{itemKey: item("b")}}, "b"},
		{"current member wins", map[string]any{
			"_links": map[string]any{itemKey: item("a")},
			"_link":  map[string]any{itemKey: item("b")},
		}, "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeUriList(tc.raw)
			if err != nil {
				t.Fatalf("DecodeUriList: %v", err)
			}
			if len(got.Links.Item) != 1 || got.Links.Item[0].GetHref() != tc.want {
				t.Errorf("items = %v, want one item %q", got.Links.Item, tc.want)
			}
			if len(got.LegacyLink.Item) != 0 {
				t.Errorf("LegacyLink not cleared: %v", got.LegacyLink.Item)
			}
		})
	}
}

func TestPaginateLinks(t *testing.T) {
	items := make([]models.Link, 5)
	tests := []struct {
		page, size, want int
	}{
		{1, 2, 2}, {3, 2, 1}, {4, 2, 0}, {1, 10, 5}, {2, 10, 0},
	}
	for _, tc := range tests {
		if got := len(PaginateLinks(items, tc.page, tc.size)); got != tc.want {
			t.Errorf("page %d size %d: %d items, want %d", tc.page, tc.size, got, tc.want)
		}
	}
}
