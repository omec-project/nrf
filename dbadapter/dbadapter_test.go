// Copyright (c) 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package dbadapter

import (
	"math"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// otherTimeField is a time field other than the one the NRF expires on.
const otherTimeField = "createdAt"

// testFieldNfInstanceId is a synthetic document field name used only by the
// tests below; it is not the package's real field constant.
const testFieldNfInstanceId = "nfInstanceId"

// testApplyJSONPatchInstanceId is a synthetic nfInstanceId value shared by
// the ApplyJSONPatch tests below.
const testApplyJSONPatchInstanceId = "nf-json-patch-1"

func TestClassifyTTLIndex(t *testing.T) {
	tests := []struct {
		name          string
		specs         []bson.M
		expectedState ttlIndexState
		expectedName  string
		expectedSecs  int32
	}{
		{
			name: "no index on the time field",
			specs: []bson.M{
				{specName: "_id_", specKey: bson.M{"_id": int32(1)}},
			},
			expectedState: ttlIndexMissing,
		},
		{
			name: "ttl index with per document expiry",
			specs: []bson.M{
				{specName: "_id_", specKey: bson.M{"_id": int32(1)}},
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(1)}, specExpireAfterSeconds: int32(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  ttlIndexField,
		},
		{
			name: "ttl index with a common timeout",
			specs: []bson.M{
				{specName: "ttl", specKey: bson.M{ttlIndexField: int32(1)}, specExpireAfterSeconds: int32(3600)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "ttl",
			expectedSecs:  3600,
		},
		{
			name: "index on the time field without expireAfterSeconds",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(1)}},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  ttlIndexField,
		},
		{
			name: "key decoded as bson.D",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.D{{Key: ttlIndexField, Value: int32(1)}}, specExpireAfterSeconds: int32(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  ttlIndexField,
		},
		{
			name: "descending key is a valid ttl index",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(-1)}, specExpireAfterSeconds: int64(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  ttlIndexField,
		},
		{
			name: "expireAfterSeconds stored as a double",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(1)}, specExpireAfterSeconds: float64(3600)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  ttlIndexField,
			expectedSecs:  3600,
		},
		{
			// MongoDB can leave a TTL index with a NaN expireAfterSeconds, which
			// expires nothing. Treat it as conflicting so that it gets recreated.
			name: "expireAfterSeconds is NaN",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(1)}, specExpireAfterSeconds: math.NaN()},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  ttlIndexField,
		},
		{
			name: "expireAfterSeconds outside the int32 range",
			specs: []bson.M{
				{specName: ttlIndexField, specKey: bson.M{ttlIndexField: int32(1)}, specExpireAfterSeconds: int64(math.MaxInt32) + 1},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  ttlIndexField,
		},
		{
			name: "compound index over the time field does not expire documents",
			specs: []bson.M{
				{specName: "expireAt_1_nftype_1", specKey: bson.M{ttlIndexField: int32(1), "nftype": int32(1)}},
			},
			expectedState: ttlIndexMissing,
		},
		{
			name: "index on another field",
			specs: []bson.M{
				{specName: otherTimeField, specKey: bson.M{otherTimeField: int32(1)}, specExpireAfterSeconds: int32(0)},
			},
			expectedState: ttlIndexMissing,
		},
		{
			name:          "collection without indexes",
			specs:         nil,
			expectedState: ttlIndexMissing,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, name, seconds := classifyTTLIndex(tc.specs, ttlIndexField)
			if state != tc.expectedState {
				t.Errorf("state = %d, want %d", state, tc.expectedState)
			}
			if name != tc.expectedName {
				t.Errorf("name = %q, want %q", name, tc.expectedName)
			}
			if seconds != tc.expectedSecs {
				t.Errorf("expireAfterSeconds = %d, want %d", seconds, tc.expectedSecs)
			}
		})
	}
}

func TestIndexKeyIsSingleField(t *testing.T) {
	tests := []struct {
		name     string
		key      any
		expected bool
	}{
		{name: "bson.M ascending", key: bson.M{ttlIndexField: int32(1)}, expected: true},
		{name: "plain map ascending", key: map[string]any{ttlIndexField: int32(1)}, expected: true},
		{name: "bson.D ascending", key: bson.D{{Key: ttlIndexField, Value: int32(1)}}, expected: true},
		{name: "float direction", key: bson.M{ttlIndexField: float64(1)}, expected: true},
		{name: "fractional direction", key: bson.M{ttlIndexField: 1.5}, expected: false},
		{name: "NaN direction", key: bson.M{ttlIndexField: math.NaN()}, expected: false},
		{name: "direction that truncates to one", key: bson.M{ttlIndexField: int64(1)<<32 | 1}, expected: false},
		{name: "other field", key: bson.M{otherTimeField: int32(1)}, expected: false},
		{name: "text index", key: bson.M{ttlIndexField: "text"}, expected: false},
		{name: "compound key", key: bson.D{{Key: ttlIndexField, Value: int32(1)}, {Key: "nftype", Value: int32(1)}}, expected: false},
		{name: "missing key document", key: nil, expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := indexKeyIsSingleField(tc.key, ttlIndexField); got != tc.expected {
				t.Errorf("indexKeyIsSingleField() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestToInt32(t *testing.T) {
	tests := []struct {
		name          string
		value         any
		expected      int32
		expectedValid bool
	}{
		{name: "int32", value: int32(-7), expected: -7, expectedValid: true},
		{name: "int64 in range", value: int64(3600), expected: 3600, expectedValid: true},
		{name: "int64 at the lower bound", value: int64(math.MinInt32), expected: math.MinInt32, expectedValid: true},
		{name: "int64 at the upper bound", value: int64(math.MaxInt32), expected: math.MaxInt32, expectedValid: true},
		{name: "int64 below the lower bound", value: int64(math.MinInt32) - 1},
		{name: "int64 above the upper bound", value: int64(math.MaxInt32) + 1},
		{name: "int in range", value: 42, expected: 42, expectedValid: true},
		{name: "int above the upper bound", value: math.MaxInt32 + 1},
		{name: "integral float", value: float64(-1), expected: -1, expectedValid: true},
		{name: "float at the upper bound", value: float64(math.MaxInt32), expected: math.MaxInt32, expectedValid: true},
		{name: "fractional float", value: 1.5},
		{name: "float above the upper bound", value: float64(math.MaxInt32) + 1},
		{name: "float far outside the int64 range", value: 1e30},
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "string", value: "3600"},
		{name: "absent", value: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toInt32(tc.value)
			if ok != tc.expectedValid {
				t.Fatalf("toInt32(%v) ok = %v, want %v", tc.value, ok, tc.expectedValid)
			}
			if got != tc.expected {
				t.Errorf("toInt32(%v) = %d, want %d", tc.value, got, tc.expected)
			}
		})
	}
}

// TestUnchangedConditionComparesVersionNotWholeDocument verifies that
// unchangedCondition compares only fieldDocVersion, not the rest of
// expectedCurrent: comparing whole documents (or their nested arrays/objects)
// via MongoDB's $eq is order-sensitive, but RestfulAPIGetOne decodes
// documents into map[string]interface{}, which never preserves field order,
// so an order-sensitive comparison would misreport an unchanged profile as
// changed.
func TestUnchangedConditionComparesVersionNotWholeDocument(t *testing.T) {
	const testNfInstanceId = "nf-1"
	filter := bson.M{testFieldNfInstanceId: testNfInstanceId}

	t.Run("versioned document compares only the version", func(t *testing.T) {
		expectedCurrent := map[string]interface{}{
			testFieldNfInstanceId: testNfInstanceId,
			"nfServices":          []interface{}{map[string]interface{}{"b": 2, "a": 1}},
			fieldDocVersion:       "v1",
		}
		got := unchangedCondition(filter, expectedCurrent)
		want := bson.M{testFieldNfInstanceId: testNfInstanceId, fieldDocVersion: "v1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("unchangedCondition() = %#v, want %#v", got, want)
		}
	})

	t.Run("unversioned document requires the version to still be absent", func(t *testing.T) {
		expectedCurrent := map[string]interface{}{testFieldNfInstanceId: testNfInstanceId}
		got := unchangedCondition(filter, expectedCurrent)
		want := bson.M{testFieldNfInstanceId: testNfInstanceId, fieldDocVersion: bson.M{mongoOpExists: false}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("unchangedCondition() = %#v, want %#v", got, want)
		}
	})
}

func TestStampDocVersionAddsUniqueTokenWithoutMutatingInput(t *testing.T) {
	original := map[string]interface{}{testFieldNfInstanceId: "nf-1"}

	stamped1 := stampDocVersion(original)
	stamped2 := stampDocVersion(original)

	if _, ok := original[fieldDocVersion]; ok {
		t.Fatalf("stampDocVersion mutated its input: %#v", original)
	}
	v1, ok := stamped1[fieldDocVersion].(string)
	if !ok || v1 == "" {
		t.Fatalf("expected a non-empty string version, got %#v", stamped1[fieldDocVersion])
	}
	v2, ok := stamped2[fieldDocVersion].(string)
	if !ok || v2 == "" {
		t.Fatalf("expected a non-empty string version, got %#v", stamped2[fieldDocVersion])
	}
	if v1 == v2 {
		t.Errorf("expected successive stamps to differ, both were %q", v1)
	}
	if stamped1["nfInstanceId"] != original["nfInstanceId"] {
		t.Errorf("expected other fields to be preserved, got %#v", stamped1)
	}
}

func TestApplyJSONPatchAppliesReplaceOperation(t *testing.T) {
	current := map[string]interface{}{testFieldNfInstanceId: testApplyJSONPatchInstanceId, "nfstatus": "REGISTERED"}
	patchJSON := []byte(`[{"op":"replace","path":"/nfstatus","value":"SUSPENDED"}]`)

	got, err := ApplyJSONPatch(current, patchJSON)
	if err != nil {
		t.Fatalf("ApplyJSONPatch() error = %v", err)
	}
	if got["nfstatus"] != "SUSPENDED" {
		t.Errorf("expected nfstatus to be replaced, got %#v", got["nfstatus"])
	}
	if got[testFieldNfInstanceId] != testApplyJSONPatchInstanceId {
		t.Errorf("expected untouched fields to be preserved, got %#v", got)
	}
	if current["nfstatus"] != "REGISTERED" {
		t.Errorf("expected ApplyJSONPatch not to mutate its input, got %#v", current)
	}
}

func TestApplyJSONPatchAppliesRemoveOperation(t *testing.T) {
	current := map[string]interface{}{testFieldNfInstanceId: testApplyJSONPatchInstanceId, "allowedNfDomains": []string{"example.com"}}
	patchJSON := []byte(`[{"op":"remove","path":"/allowedNfDomains"}]`)

	got, err := ApplyJSONPatch(current, patchJSON)
	if err != nil {
		t.Fatalf("ApplyJSONPatch() error = %v", err)
	}
	if _, ok := got["allowedNfDomains"]; ok {
		t.Errorf("expected allowedNfDomains to be removed, got %#v", got)
	}
}

// TestApplyJSONPatchPreservesBSONDateType verifies the documented reason for
// using bson.MarshalExtJSON/UnmarshalExtJSON instead of encoding/json: a
// date field untouched by the patch must round-trip as a date, not collapse
// into a plain string that would corrupt the TTL-indexed expireAt field once
// written back.
func TestApplyJSONPatchPreservesBSONDateType(t *testing.T) {
	expireAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := map[string]interface{}{testFieldNfInstanceId: testApplyJSONPatchInstanceId, ttlIndexField: expireAt}
	patchJSON := []byte(`[{"op":"add","path":"/nfstatus","value":"SUSPENDED"}]`)

	got, err := ApplyJSONPatch(current, patchJSON)
	if err != nil {
		t.Fatalf("ApplyJSONPatch() error = %v", err)
	}
	dt, ok := got[ttlIndexField].(bson.DateTime)
	if !ok {
		t.Fatalf("expected %s to round-trip as bson.DateTime, got %T (%#v)", ttlIndexField, got[ttlIndexField], got[ttlIndexField])
	}
	if !dt.Time().Equal(expireAt) {
		t.Errorf("expected %s to be preserved, got %v want %v", ttlIndexField, dt.Time(), expireAt)
	}
}

func TestApplyJSONPatchRejectsMalformedPatchJSON(t *testing.T) {
	current := map[string]interface{}{testFieldNfInstanceId: testApplyJSONPatchInstanceId}
	if _, err := ApplyJSONPatch(current, []byte("not valid json")); err == nil {
		t.Fatal("expected an error for malformed patch JSON")
	}
}

func TestApplyJSONPatchRejectsOperationThatCannotApply(t *testing.T) {
	current := map[string]interface{}{testFieldNfInstanceId: testApplyJSONPatchInstanceId}
	// "replace" requires the target member to already exist (RFC 6902 §4.3).
	patchJSON := []byte(`[{"op":"replace","path":"/nfstatus","value":"SUSPENDED"}]`)
	if _, err := ApplyJSONPatch(current, patchJSON); err == nil {
		t.Fatal("expected an error when replacing a member that does not exist")
	}
}
