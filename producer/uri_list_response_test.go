// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package producer_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/omec-project/nrf/producer"
)

func storedUriList(linkKey string, n int) map[string]any {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{"href": fmt.Sprintf("https://nrf:29510/nnrf-nfm/v1/nf-instances/udm-%d", i+1)}
	}
	return map[string]any{"nfType": "UDM", linkKey: map[string]any{"item": items}}
}

func getNFInstances(t *testing.T, stored map[string]any, query string) (int, map[string]any) {
	t.Helper()
	useDB(t, &statusCodeDB{getOne: stored})
	request := newRequest(nil)
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("bad test query %q: %v", query, err)
	}
	request.Query = values
	response := producer.HandleGetNFInstancesRequest(request)
	raw, err := json.Marshal(response.Body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return response.Status, body
}

func itemCount(t *testing.T, body map[string]any) int {
	t.Helper()
	links, _ := body["_links"].(map[string]any)
	items, _ := links["item"].([]any)
	return len(items)
}

// TS 29.510 clause 6.1.6.2.25: "_links" (plural) holds "item" and "self"; totalItemCount is present.
func TestGetNFInstancesResponseFollowsUriListSchema(t *testing.T) {
	status, body := getNFInstances(t, storedUriList("_links", 3), "nf-type=UDM")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if _, ok := body["_link"]; ok {
		t.Errorf("response carries the pre-spec _link member: %v", body)
	}
	if _, ok := body["nfType"]; ok {
		t.Errorf("response carries nfType, which is not a UriList attribute: %v", body)
	}
	links, ok := body["_links"].(map[string]any)
	if !ok {
		t.Fatalf("no _links object in %v", body)
	}
	if _, ok := links["self"].(map[string]any); !ok {
		t.Errorf("_links has no self link: %v", links)
	}
	if got := itemCount(t, body); got != 3 {
		t.Errorf("items = %d, want 3", got)
	}
	if got := body["totalItemCount"]; got != float64(3) {
		t.Errorf("totalItemCount = %v, want 3", got)
	}
}

// Documents stored before the rename hold their items under "_link" and must stay readable.
func TestGetNFInstancesReadsDocumentsStoredWithLegacyLinkMember(t *testing.T) {
	status, body := getNFInstances(t, storedUriList("_link", 2), "nf-type=UDM")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := itemCount(t, body); got != 2 {
		t.Errorf("items = %d, want 2", got)
	}
	if _, ok := body["_link"]; ok {
		t.Errorf("response carries the pre-spec _link member: %v", body)
	}
}

// With nothing to return, "_links" holds only "self" and totalItemCount is 0.
func TestGetNFInstancesWithNoMatchHasOnlySelf(t *testing.T) {
	status, body := getNFInstances(t, map[string]any{}, "nf-type=UDM")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	links, _ := body["_links"].(map[string]any)
	if _, ok := links["item"]; ok {
		t.Errorf("item present for an empty result: %v", links)
	}
	if _, ok := links["self"]; !ok {
		t.Errorf("self missing for an empty result: %v", links)
	}
	if got, ok := body["totalItemCount"]; !ok || got != float64(0) {
		t.Errorf("totalItemCount = %v (present=%v), want 0", got, ok)
	}
}

func TestGetNFInstancesPagingAndLimit(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantItems int
		wantTotal float64
	}{
		{"limit truncates but total counts all", "nf-type=UDM&limit=2", 2, 5},
		{"first page", "nf-type=UDM&page-number=1&page-size=2", 2, 5},
		{"last partial page", "nf-type=UDM&page-number=3&page-size=2", 1, 5},
		{"page past the end is empty", "nf-type=UDM&page-number=4&page-size=2", 0, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := getNFInstances(t, storedUriList("_links", 5), tc.query)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if got := itemCount(t, body); got != tc.wantItems {
				t.Errorf("items = %d, want %d", got, tc.wantItems)
			}
			if got := body["totalItemCount"]; got != tc.wantTotal {
				t.Errorf("totalItemCount = %v, want %v", got, tc.wantTotal)
			}
		})
	}
}

func TestGetNFInstancesPagesAreDisjointAndOrdered(t *testing.T) {
	var seen []string
	for page := 1; page <= 3; page++ {
		_, body := getNFInstances(t, storedUriList("_links", 5), fmt.Sprintf("nf-type=UDM&page-number=%d&page-size=2", page))
		links, _ := body["_links"].(map[string]any)
		for _, it := range links["item"].([]any) {
			seen = append(seen, it.(map[string]any)["href"].(string))
		}
	}
	if len(seen) != 5 {
		t.Fatalf("pages returned %d items, want 5: %v", len(seen), seen)
	}
	for i, href := range seen {
		if !strings.HasSuffix(href, fmt.Sprintf("udm-%d", i+1)) {
			t.Errorf("item %d = %s, want udm-%d", i, href, i+1)
		}
	}
}

// Clause 6.1.3.2.3.1: page-number and page-size come together; limit is absent when they are present.
func TestGetNFInstancesRejectsInvalidPagingParameters(t *testing.T) {
	for _, query := range []string{
		"nf-type=UDM&page-number=1",
		"nf-type=UDM&page-size=10",
		"nf-type=UDM&page-number=0&page-size=10",
		"nf-type=UDM&page-number=1&page-size=abc",
		"nf-type=UDM&page-number=1&page-size=10&limit=5",
	} {
		t.Run(query, func(t *testing.T) {
			status, _ := getNFInstances(t, storedUriList("_links", 5), query)
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
		})
	}
}
