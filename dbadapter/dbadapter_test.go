// Copyright (c) 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package dbadapter

import (
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
