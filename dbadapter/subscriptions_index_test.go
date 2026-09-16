// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package dbadapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/util/mongoapi"
)

// subscrCond is the field every whole-subdocument notification filter matches on.
const subscrCond = "subscrCond"

// indexStubDB implements indexEnsurer and nothing else -- which is the point of
// that interface being narrow: a double for the index setup needs no knowledge
// of the rest of the datastore API.
type indexStubDB struct {
	collections []string
	ensured     []mongoapi.IndexSpec
	results     []error
}

func (s *indexStubDB) EnsureIndex(_ context.Context, collName string, spec mongoapi.IndexSpec) error {
	s.collections = append(s.collections, collName)
	s.ensured = append(s.ensured, spec)
	if len(s.results) == 0 {
		return nil
	}
	err := s.results[0]
	s.results = s.results[1:]
	return err
}

func indexNamed(t *testing.T, name string) mongoapi.IndexSpec {
	t.Helper()
	for _, spec := range subscriptionsIndexes() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("no index named %q", name)
	return mongoapi.IndexSpec{}
}

// TestSubscriptionsIndexesServeEveryNotificationFilter pins the index set to
// the filters GetNotificationUri and deregistration build. A filter no index
// serves scans the collection, and the notification still goes out -- just
// after a scan per condition -- so nothing at runtime reports it.
//
// The filter list below is transcribed from context/management_data.go rather
// than read from it: that package imports this one, so a test here cannot call
// the builders without an import cycle. It therefore catches an index that stops
// serving a filter, and not a filter that changes shape -- if a builder is given
// a new predicate, this test keeps passing. Say so rather than let the next
// reader assume otherwise.
func TestSubscriptionsIndexesServeEveryNotificationFilter(t *testing.T) {
	// The field each filter is keyed on, and the builder that makes it.
	// Only the builders whose filter an index can actually serve. The ones left
	// out are left out on purpose, and the test below says which and why --
	// listing them here would have claimed they were served, and passed,
	// because this check only looks at an index's leading field.
	filters := map[string]string{
		"addNfTypeCond":             subscrCond,
		"addNfInstanceIDCond":       subscrCond,
		"addAmfCond":                subscrCond,
		"addNfGroupCond":            subscrCond,
		"addServiceNameCond":        "subscrCond.serviceName",
		"addNetworkSliceCond":       "subscrCond.nsiList",
		"deregistration DeleteMany": "subscrCond.nfInstanceId",
	}

	indexed := map[string]bool{}
	for _, spec := range subscriptionsIndexes() {
		if len(spec.Keys) == 0 {
			t.Errorf("index %q has no keys", spec.Name)
			continue
		}
		indexed[spec.Keys[0].Key] = true
	}

	for builder, field := range filters {
		if !indexed[field] {
			t.Errorf("%s filters on %q, which no index leads with, so it scans Subscriptions", builder, field)
		}
	}
}

// TestSubscriptionsIndexesKeepTheParentAndTheDottedPathApart is the finding
// that a "consolidate by prefix" reading would get wrong: MongoDB serves a
// whole-embedded-document match from an index on the parent field and a dotted
// match from an index on the path, and neither serves the other kind.
func TestSubscriptionsIndexesKeepTheParentAndTheDottedPathApart(t *testing.T) {
	parent := indexNamed(t, "subscriptionsBySubscrCond")
	if parent.Keys[0].Key != subscrCond {
		t.Errorf("index %q leads with %q, not the parent field", parent.Name, parent.Keys[0].Key)
	}

	dotted := indexNamed(t, "subscriptionsByNfInstanceId")
	if dotted.Keys[0].Key != "subscrCond.nfInstanceId" {
		t.Errorf("index %q leads with %q, not the dotted path", dotted.Name, dotted.Keys[0].Key)
	}
	if !strings.HasPrefix(dotted.Keys[0].Key, parent.Keys[0].Key+".") {
		t.Fatal("this test no longer covers a nested pair, so it no longer covers the finding")
	}
}

func TestSubscriptionsIndexesAssertNoUniqueness(t *testing.T) {
	for _, spec := range subscriptionsIndexes() {
		if spec.Unique {
			t.Errorf("index %q is unique; a subscription condition is not an identity and "+
				"several NFs can subscribe to the same one", spec.Name)
		}
		if spec.Name == "" {
			t.Error("an index has no name, so a later version cannot replace it")
		}
	}
}

func TestEnsureSubscriptionsIndexesCreatesThemAll(t *testing.T) {
	stub := &indexStubDB{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureSubscriptionsIndexes(ctx, stub); err != nil {
		t.Fatalf("ensureSubscriptionsIndexes failed: %v", err)
	}

	// The set and its keys, not the count: counting alone passes if a spec is
	// sent twice while another is dropped, or if any of them goes to the wrong
	// collection.
	ensured := map[string]mongoapi.IndexSpec{}
	for i, spec := range stub.ensured {
		if stub.collections[i] != subscriptionsCollection {
			t.Errorf("index %q was ensured on collection %q, want %q",
				spec.Name, stub.collections[i], subscriptionsCollection)
		}
		if _, repeated := ensured[spec.Name]; repeated {
			t.Errorf("index %q was ensured twice", spec.Name)
		}
		ensured[spec.Name] = spec
	}

	for _, want := range subscriptionsIndexes() {
		got, present := ensured[want.Name]
		if !present {
			t.Errorf("index %q was declared but never ensured", want.Name)
			continue
		}
		if len(got.Keys) != len(want.Keys) || got.Keys[0].Key != want.Keys[0].Key {
			t.Errorf("index %q was ensured over %v, want %v", want.Name, got.Keys, want.Keys)
		}
		delete(ensured, want.Name)
	}
	for name := range ensured {
		t.Errorf("index %q was ensured but is not one of subscriptionsIndexes()", name)
	}
}

func TestEnsureSubscriptionsIndexesRetriesAndThenSucceeds(t *testing.T) {
	stub := &indexStubDB{results: []error{errors.New("no primary yet")}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureSubscriptionsIndexes(ctx, stub); err != nil {
		t.Fatalf("gave up on an error that went away: %v", err)
	}
	if len(stub.ensured) != len(subscriptionsIndexes())+1 {
		t.Errorf("expected one retry, got %d call(s)", len(stub.ensured))
	}
}

func TestEnsureSubscriptionsIndexesReportsWhenTheBudgetIsSpent(t *testing.T) {
	stub := &indexStubDB{results: []error{errors.New("still no primary")}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	err := ensureSubscriptionsIndexes(ctx, stub)
	if err == nil {
		t.Fatal("expected an error once the budget was spent")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error does not carry the deadline that caused it: %v", err)
	}
}

// TestTheElemMatchFiltersAreKnownToBeServedByNoIndex records the two builders no
// index here covers, so that their absence from the map above reads as a
// decision rather than an oversight.
//
// addGuamiListCond builds {subscrCond: {$elemMatch: ...}}, and addNetworkSliceCond
// builds the same shape for its S-NSSAI half -- which is its whole filter when
// nsiList is absent. $elemMatch selects only where the field is an array, and
// subscrCond is a oneOf embedded document that never is one, so those predicates
// match nothing whatever the collection carries. Indexing cannot change that; it
// is a defect in the notification matching, and not this change's to fix.
func TestTheElemMatchFiltersAreKnownToBeServedByNoIndex(t *testing.T) {
	unserved := []string{"addGuamiListCond", "addNetworkSliceCond $elemMatch branch"}

	// Nothing to assert about the index set -- the point is the opposite, that
	// no index claims these. Guard instead against someone adding one and
	// believing it helps.
	for _, spec := range subscriptionsIndexes() {
		if len(spec.Keys) == 0 {
			continue
		}
		if spec.Keys[0].Key == subscrCond && spec.PartialFilter != nil {
			t.Errorf("index %q filters on subscrCond in a way that suggests it targets %v; "+
				"an $elemMatch predicate against a non-array field is served by no index",
				spec.Name, unserved)
		}
	}

	if len(unserved) != 2 {
		t.Errorf("the list of unserved builders changed to %v; if a filter shape was fixed upstream "+
			"it may now be indexable, and the map above should gain it", unserved)
	}
}
