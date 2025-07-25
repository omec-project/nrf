// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package dbadapter

import (
	"context"

	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	RestfulAPIPutMany(collName string, filterArray []primitive.M, putDataArray []map[string]interface{}) error
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
		ttlIndexCreated := db.RestfulAPICreateTTLIndex("NfProfile", 0, "expireAt")
		ttlIndexStatus := "exists"
		if ttlIndexCreated {
			ttlIndexStatus = "created"
		}
		logger.AppLog.Infof("ttl Index %s for field 'expireAt' in collection 'NfProfile'", ttlIndexStatus)
	}
	return DBClient
}

func (db *MongoDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return db.MongoClient.RestfulAPIGetOne(collName, filter)
}

func (db *MongoDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	return db.MongoClient.RestfulAPIGetMany(collName, filter)
}

func (db *MongoDBClient) RestfulAPIPutOne(collName string, filter bson.M, putData map[string]interface{}) (bool, error) {
	return db.MongoClient.RestfulAPIPutOne(collName, filter, putData)
}

func (db *MongoDBClient) RestfulAPIPutOneNotUpdate(collName string, filter bson.M, putData map[string]interface{}) (bool, error) {
	return db.MongoClient.RestfulAPIPutOneNotUpdate(collName, filter, putData)
}

func (db *MongoDBClient) RestfulAPIPutMany(collName string, filterArray []primitive.M, putDataArray []map[string]interface{}) error {
	return db.MongoClient.RestfulAPIPutMany(collName, filterArray, putDataArray)
}

func (db *MongoDBClient) RestfulAPIDeleteOne(collName string, filter bson.M) {
	db.MongoClient.RestfulAPIDeleteOne(collName, filter)
}

func (db *MongoDBClient) RestfulAPIDeleteMany(collName string, filter bson.M) {
	db.MongoClient.RestfulAPIDeleteMany(collName, filter)
}

func (db *MongoDBClient) RestfulAPIMergePatch(collName string, filter bson.M, patchData map[string]interface{}) error {
	return db.MongoClient.RestfulAPIMergePatch(collName, filter, patchData)
}

func (db *MongoDBClient) RestfulAPIJSONPatch(collName string, filter bson.M, patchJSON []byte) error {
	return db.MongoClient.RestfulAPIJSONPatch(collName, filter, patchJSON)
}

func (db *MongoDBClient) RestfulAPIJSONPatchExtend(collName string, filter bson.M, patchJSON []byte, dataName string) error {
	return db.MongoClient.RestfulAPIJSONPatchExtend(collName, filter, patchJSON, dataName)
}

func (db *MongoDBClient) RestfulAPIPost(collName string, filter bson.M, postData map[string]interface{}) (bool, error) {
	return db.MongoClient.RestfulAPIPost(collName, filter, postData)
}

func (db *MongoDBClient) RestfulAPIPostMany(collName string, filter bson.M, postDataArray []interface{}) error {
	return db.MongoClient.RestfulAPIPostMany(collName, filter, postDataArray)
}
