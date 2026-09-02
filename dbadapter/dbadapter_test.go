// Copyright (c) 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package dbadapter

import (
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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
				{"name": "_id_", "key": bson.M{"_id": int32(1)}},
			},
			expectedState: ttlIndexMissing,
		},
		{
			name: "ttl index with per document expiry",
			specs: []bson.M{
				{"name": "_id_", "key": bson.M{"_id": int32(1)}},
				{"name": "expireAt", "key": bson.M{"expireAt": int32(1)}, "expireAfterSeconds": int32(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "expireAt",
		},
		{
			name: "ttl index with a common timeout",
			specs: []bson.M{
				{"name": "ttl", "key": bson.M{"expireAt": int32(1)}, "expireAfterSeconds": int32(3600)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "ttl",
			expectedSecs:  3600,
		},
		{
			name: "index on the time field without expireAfterSeconds",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.M{"expireAt": int32(1)}},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  "expireAt",
		},
		{
			name: "key decoded as bson.D",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.D{{Key: "expireAt", Value: int32(1)}}, "expireAfterSeconds": int32(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "expireAt",
		},
		{
			name: "descending key is a valid ttl index",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.M{"expireAt": int32(-1)}, "expireAfterSeconds": int64(0)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "expireAt",
		},
		{
			name: "expireAfterSeconds stored as a double",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.M{"expireAt": int32(1)}, "expireAfterSeconds": float64(3600)},
			},
			expectedState: ttlIndexPresent,
			expectedName:  "expireAt",
			expectedSecs:  3600,
		},
		{
			// MongoDB can leave a TTL index with a NaN expireAfterSeconds, which
			// expires nothing. Treat it as conflicting so that it gets recreated.
			name: "expireAfterSeconds is NaN",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.M{"expireAt": int32(1)}, "expireAfterSeconds": math.NaN()},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  "expireAt",
		},
		{
			name: "expireAfterSeconds outside the int32 range",
			specs: []bson.M{
				{"name": "expireAt", "key": bson.M{"expireAt": int32(1)}, "expireAfterSeconds": int64(math.MaxInt32) + 1},
			},
			expectedState: ttlIndexConflicting,
			expectedName:  "expireAt",
		},
		{
			name: "compound index over the time field does not expire documents",
			specs: []bson.M{
				{"name": "expireAt_1_nftype_1", "key": bson.M{"expireAt": int32(1), "nftype": int32(1)}},
			},
			expectedState: ttlIndexMissing,
		},
		{
			name: "index on another field",
			specs: []bson.M{
				{"name": "createdAt", "key": bson.M{"createdAt": int32(1)}, "expireAfterSeconds": int32(0)},
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
			state, name, seconds := classifyTTLIndex(tc.specs, "expireAt")
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
		{name: "bson.M ascending", key: bson.M{"expireAt": int32(1)}, expected: true},
		{name: "plain map ascending", key: map[string]any{"expireAt": int32(1)}, expected: true},
		{name: "bson.D ascending", key: bson.D{{Key: "expireAt", Value: int32(1)}}, expected: true},
		{name: "float direction", key: bson.M{"expireAt": float64(1)}, expected: true},
		{name: "fractional direction", key: bson.M{"expireAt": 1.5}, expected: false},
		{name: "NaN direction", key: bson.M{"expireAt": math.NaN()}, expected: false},
		{name: "direction that truncates to one", key: bson.M{"expireAt": int64(1)<<32 | 1}, expected: false},
		{name: "other field", key: bson.M{"createdAt": int32(1)}, expected: false},
		{name: "text index", key: bson.M{"expireAt": "text"}, expected: false},
		{name: "compound key", key: bson.D{{Key: "expireAt", Value: int32(1)}, {Key: "nftype", Value: int32(1)}}, expected: false},
		{name: "missing key document", key: nil, expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := indexKeyIsSingleField(tc.key, "expireAt"); got != tc.expected {
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
