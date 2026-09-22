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
			uriList.Link.Item = make([]models.Link, tc.items)

			NnrfUriListLimit(uriList, tc.limit)

			if got := len(uriList.Link.Item); got != tc.want {
				t.Errorf("items = %d, want %d", got, tc.want)
			}
		})
	}
}
