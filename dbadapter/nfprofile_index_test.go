// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package dbadapter

import (
	"context"
	"testing"
	"time"

	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestNfProfileIndexKeyIsTheStoredFieldName encodes a profile the way
// NFRegisterProcedure stores it and requires the index to lead with the field
// the instance ID actually lands in. An index on any other spelling -- the
// camel-case nfInstanceId of the API model, say -- would be created, reported
// present, and never chosen.
func TestNfProfileIndexKeyIsTheStoredFieldName(t *testing.T) {
	const id = "4947a69a-f61b-4bc1-b9da-47c9c5d14b64"
	raw, err := bson.Marshal(models.NFProfile{NfInstanceId: id})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var stored bson.M
	if err := bson.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	specs := nfProfileIndexes()
	if len(specs) != 1 || len(specs[0].Keys) != 1 {
		t.Fatalf("expected one single-field index, got %+v", specs)
	}
	key := specs[0].Keys[0].Key
	if stored[key] != id {
		t.Errorf("index %q is on %q, but a stored profile does not carry its instance ID there",
			specs[0].Name, key)
	}
}

func TestNfProfileIndexesAssertNoUniqueness(t *testing.T) {
	for _, spec := range nfProfileIndexes() {
		if spec.Unique {
			t.Errorf("index %q is unique; a collection already holding a duplicate "+
				"would make it impossible to create, and the NRF unable to start", spec.Name)
		}
		if spec.Name == "" {
			t.Error("an index has no name, so a later version cannot replace it")
		}
	}
}

func TestEnsureIndexesTargetsTheCollectionItIsGiven(t *testing.T) {
	stub := &indexStubDB{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureIndexes(ctx, stub, nfProfileCollection, nfProfileIndexes()); err != nil {
		t.Fatalf("ensureIndexes failed: %v", err)
	}
	if len(stub.ensured) != len(nfProfileIndexes()) {
		t.Fatalf("ensured %d index(es), want %d", len(stub.ensured), len(nfProfileIndexes()))
	}
	for i, spec := range stub.ensured {
		if stub.collections[i] != nfProfileCollection {
			t.Errorf("index %q was ensured on collection %q, want %q",
				spec.Name, stub.collections[i], nfProfileCollection)
		}
	}
}
