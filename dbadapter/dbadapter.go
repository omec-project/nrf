// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package dbadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	nfProfileCollection = "NfProfile"
	ttlIndexField       = "expireAt"

	// MongoDB may have no writable primary yet when the NRF starts (election in
	// progress, or the NRF started before MongoDB was ready), so index creation
	// is retried with backoff instead of being attempted once.
	ttlIndexOpTimeout    = 30 * time.Second
	ttlIndexRetryTimeout = 5 * time.Minute
	ttlIndexRetryInitial = time.Second
	ttlIndexRetryMax     = 30 * time.Second
)

type DBInterface interface {
	RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error)
	RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error)
	RestfulAPIPutOne(collName string, filter bson.M, putData map[string]interface{}) (bool, error)
	RestfulAPIPutOneNotUpdate(collName string, filter bson.M, putData map[string]interface{}) (bool, error)
	RestfulAPIDeleteOne(collName string, filter bson.M) error
	RestfulAPIDeleteMany(collName string, filter bson.M) error
	RestfulAPIMergePatch(collName string, filter bson.M, patchData map[string]interface{}) error
	RestfulAPIJSONPatch(collName string, filter bson.M, patchJSON []byte) error
	RestfulAPIJSONPatchExtend(collName string, filter bson.M, patchJSON []byte, dataName string) error
	RestfulAPIPost(collName string, filter bson.M, postData map[string]interface{}) (bool, error)
	RestfulAPIPutMany(collName string, filterArray []bson.M, putDataArray []map[string]interface{}) error
}

var DBClient DBInterface = nil

type MongoDBClient struct {
	mongoapi.MongoClient
}

func iterateChangeStream(routineCtx context.Context, stream *mongo.ChangeStream) {
	logger.AppLog.Infoln("iterate change stream for timeout")
	defer stream.Close(routineCtx)
	for stream.Next(routineCtx) {
		var data bson.M
		if err := stream.Decode(&data); err != nil {
			panic(err)
		}
		logger.AppLog.Infoln("iterate stream:", data)
	}
}

func ConnectToDBClient(dbName string, url string, enableStream bool, nfProfileExpiryEnable bool) DBInterface {
	for {
		MongoClient, _ := mongoapi.NewMongoClient(url, dbName)
		if MongoClient != nil {
			logger.AppLog.Infoln("MongoDB Connection Successful")
			DBClient = MongoClient
			break
		} else {
			logger.AppLog.Infoln("MongoDB Connection Failed")
		}
	}

	db := DBClient.(*mongoapi.MongoClient)
	if enableStream {
		logger.AppLog.Infoln("MongoDB Change stream Enabled")
		database := db.Client.Database(dbName)
		NfProfileColl := database.Collection("NfProfile")
		// create stream to monitor actions on the collection
		NfProfStream, err := NfProfileColl.Watch(context.TODO(), mongo.Pipeline{})
		if err != nil {
			panic(err)
		}
		routineCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// run routine to get messages from stream
		go iterateChangeStream(routineCtx, NfProfStream)
	}

	if nfProfileExpiryEnable {
		logger.AppLog.Infoln("NfProfile document expiry enabled")
		ctx, cancel := context.WithTimeout(context.Background(), ttlIndexRetryTimeout)
		defer cancel()
		if err := ensureTTLIndex(ctx, db, nfProfileCollection, ttlIndexField); err != nil {
			// The TTL index is the only thing that removes expired NF profiles when
			// nfProfileExpiryEnable is set. Running without it leaks dead profiles
			// into NF discovery forever, so exit and let the deployment restart the
			// NRF once MongoDB accepts writes instead of serving stale endpoints.
			logger.AppLog.Fatalf("could not ensure ttl index for field '%s' in collection '%s': %v",
				ttlIndexField, nfProfileCollection, err)
		}
	}
	return DBClient
}

// ttlIndexState describes what the indexes of a collection say about the TTL
// index the NRF relies on to expire dead NF profiles.
type ttlIndexState int

const (
	// ttlIndexMissing: no index on the time field at all.
	ttlIndexMissing ttlIndexState = iota
	// ttlIndexPresent: an index on the time field with expireAfterSeconds set,
	// so MongoDB expires the documents.
	ttlIndexPresent
	// ttlIndexConflicting: an index on the time field exists but has no
	// expireAfterSeconds, so nothing is ever expired.
	ttlIndexConflicting
)

// ensureTTLIndex makes sure a TTL index on timeField exists, retrying with
// backoff until ctx expires. It reports success only after the index has been
// observed through listIndexes, never on the strength of a create call alone.
func ensureTTLIndex(ctx context.Context, db *mongoapi.MongoClient, collName, timeField string) error {
	backoff := ttlIndexRetryInitial
	for attempt := 1; ; attempt++ {
		err := ensureTTLIndexOnce(ctx, db, collName, timeField)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("gave up after %d attempts: %w (last error: %v)", attempt, ctxErr, err)
		}
		logger.AppLog.Warnf("attempt %d to ensure ttl index for field '%s' in collection '%s' failed, retrying in %s: %v",
			attempt, timeField, collName, backoff, err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up after %d attempts: %w (last error: %v)", attempt, ctx.Err(), err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, ttlIndexRetryMax)
	}
}

// ensureTTLIndexOnce runs a single inspect/repair/create/verify cycle.
func ensureTTLIndexOnce(ctx context.Context, db *mongoapi.MongoClient, collName, timeField string) error {
	opCtx, cancel := context.WithTimeout(ctx, ttlIndexOpTimeout)
	defer cancel()

	state, name, seconds, err := inspectTTLIndex(opCtx, db, collName, timeField)
	if err != nil {
		return err
	}
	switch state {
	case ttlIndexPresent:
		if seconds != 0 {
			logger.AppLog.Warnf("index '%s' in collection '%s' expires documents %d seconds after '%s' "+
				"instead of at the time stored in it", name, collName, seconds, timeField)
		}
		logger.AppLog.Infof("ttl index exists for field '%s' in collection '%s'", timeField, collName)
		return nil
	case ttlIndexConflicting:
		logger.AppLog.Warnf("index '%s' in collection '%s' has no expireAfterSeconds, so expired documents are "+
			"never removed; dropping it to recreate it as a ttl index", name, collName)
		if dropErr := db.RestfulAPIDropTTLIndexWithContext(opCtx, collName, name); dropErr != nil {
			return dropErr
		}
	case ttlIndexMissing:
	}

	if err = db.RestfulAPICreateTTLIndexWithContext(opCtx, collName, 0, timeField); err != nil {
		// An index of the same name with different options blocks creation. Drop
		// it here so the next attempt starts from a clean collection.
		if mongoapi.IsIndexOptionsConflict(err) {
			logger.AppLog.Warnf("index '%s' in collection '%s' conflicts with the ttl index, dropping it: %v",
				timeField, collName, err)
			if dropErr := db.RestfulAPIDropTTLIndexWithContext(opCtx, collName, timeField); dropErr != nil {
				return errors.Join(err, dropErr)
			}
		}
		return err
	}

	// Verify against listIndexes rather than trusting the create call.
	state, _, _, err = inspectTTLIndex(opCtx, db, collName, timeField)
	if err != nil {
		return err
	}
	if state != ttlIndexPresent {
		return fmt.Errorf("ttl index for field '%s' in collection '%s' is still absent after creation",
			timeField, collName)
	}
	logger.AppLog.Infof("ttl index created for field '%s' in collection '%s'", timeField, collName)
	return nil
}

// inspectTTLIndex reads the index specifications of a collection and classifies
// the one covering timeField.
func inspectTTLIndex(ctx context.Context, db *mongoapi.MongoClient, collName, timeField string) (ttlIndexState, string, int32, error) {
	specs, err := db.RestfulAPIListIndexes(ctx, collName)
	if err != nil {
		return ttlIndexMissing, "", 0, err
	}
	state, name, seconds := classifyTTLIndex(specs, timeField)
	if name == "" {
		name = timeField
	}
	return state, name, seconds, nil
}

// classifyTTLIndex reports whether the given listIndexes specifications contain
// a TTL index over timeField, together with the name and expireAfterSeconds of
// the matching index.
func classifyTTLIndex(specs []bson.M, timeField string) (ttlIndexState, string, int32) {
	for _, spec := range specs {
		if !indexKeyIsSingleField(spec["key"], timeField) {
			continue
		}
		name, _ := spec["name"].(string)
		if seconds, ok := toInt32(spec["expireAfterSeconds"]); ok {
			return ttlIndexPresent, name, seconds
		}
		return ttlIndexConflicting, name, 0
	}
	return ttlIndexMissing, "", 0
}

// indexKeyIsSingleField reports whether an index key document covers exactly
// field. The key is decoded as bson.M or bson.D depending on the BSON options
// of the client, so both are accepted. Either direction is valid for a TTL
// index, which MongoDB only supports on a single field.
func indexKeyIsSingleField(key any, field string) bool {
	switch k := key.(type) {
	case bson.M:
		return len(k) == 1 && isIndexDirection(k[field])
	case map[string]any:
		return len(k) == 1 && isIndexDirection(k[field])
	case bson.D:
		return len(k) == 1 && k[0].Key == field && isIndexDirection(k[0].Value)
	}
	return false
}

func isIndexDirection(value any) bool {
	direction, ok := toInt32(value)
	return ok && (direction == 1 || direction == -1)
}

// toInt32 normalizes the numeric BSON types listIndexes may return.
func toInt32(value any) (int32, bool) {
	switch v := value.(type) {
	case int32:
		return v, true
	case int64:
		return int32(v), true
	case int:
		return int32(v), true
	case float64:
		return int32(v), true
	}
	return 0, false
}
