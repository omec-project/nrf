// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package dbadapter

import (
	"github.com/omec-project/MongoDBLibrary"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type DBInterface interface {
	RestfulAPIGetOne(collName string, filter bson.M) map[string]interface{}
	RestfulAPIGetMany(collName string, filter bson.M) []map[string]interface{}
	PutOneWithTimeout(collName string, filter bson.M, putData map[string]interface{}, timeout int32, timeField string) bool
	RestfulAPIPutOne(collName string, filter bson.M, putData map[string]interface{}) bool
	RestfulAPIPutOneNotUpdate(collName string, filter bson.M, putData map[string]interface{}) bool
	RestfulAPIPutMany(collName string, filterArray []bson.M, putDataArray []map[string]interface{}) bool
	RestfulAPIDeleteOne(collName string, filter bson.M)
	RestfulAPIDeleteMany(collName string, filter bson.M)
	RestfulAPIMergePatch(collName string, filter bson.M, patchData map[string]interface{}) bool
	RestfulAPIJSONPatch(collName string, filter bson.M, patchJSON []byte) bool
	RestfulAPIJSONPatchExtend(collName string, filter bson.M, patchJSON []byte, dataName string) bool
	RestfulAPIPost(collName string, filter bson.M, postData map[string]interface{}) bool
	RestfulAPIPostMany(collName string, filter bson.M, postDataArray []interface{}) bool
}

var DBClient DBInterface = nil

type MongoDBClient struct {
	Client *mongo.Client
	dbName string
}

func ConnectToDBClient(setdbName string, url string) DBInterface {
	dbc := &MongoDBClient{}
	MongoDBLibrary.SetMongoDB(setdbName, url)
	if MongoDBLibrary.Client != nil {
		dbc.Client = MongoDBLibrary.Client
		dbc.dbName = setdbName
	}
	DBClient = dbc
	return dbc
}

func (db *MongoDBClient) RestfulAPIGetOne(collName string, filter bson.M) map[string]interface{} {
	return MongoDBLibrary.RestfulAPIGetOne(collName, filter)
}

func (db *MongoDBClient) RestfulAPIGetMany(collName string, filter bson.M) []map[string]interface{} {
	return MongoDBLibrary.RestfulAPIGetMany(collName, filter)
}
func (db *MongoDBClient) PutOneWithTimeout(collName string, filter bson.M, putData map[string]interface{}, timeout int32, timeField string) bool {
	return MongoDBLibrary.PutOneWithTimeout(collName, filter, putData, timeout, timeField)
}
func (db *MongoDBClient) RestfulAPIPutOne(collName string, filter bson.M, putData map[string]interface{}) bool {
	return MongoDBLibrary.RestfulAPIPutOne(collName, filter, putData)
}
func (db *MongoDBClient) RestfulAPIPutOneNotUpdate(collName string, filter bson.M, putData map[string]interface{}) bool {
	return MongoDBLibrary.RestfulAPIPutOneNotUpdate(collName, filter, putData)
}
func (db *MongoDBClient) RestfulAPIPutMany(collName string, filterArray []bson.M, putDataArray []map[string]interface{}) bool {
	return MongoDBLibrary.RestfulAPIPutMany(collName, filterArray, putDataArray)
}
func (db *MongoDBClient) RestfulAPIDeleteOne(collName string, filter bson.M) {
	MongoDBLibrary.RestfulAPIDeleteOne(collName, filter)
}
func (db *MongoDBClient) RestfulAPIDeleteMany(collName string, filter bson.M) {
	MongoDBLibrary.RestfulAPIDeleteMany(collName, filter)
}
func (db *MongoDBClient) RestfulAPIMergePatch(collName string, filter bson.M, patchData map[string]interface{}) bool {
	return MongoDBLibrary.RestfulAPIMergePatch(collName, filter, patchData)
}
func (db *MongoDBClient) RestfulAPIJSONPatch(collName string, filter bson.M, patchJSON []byte) bool {
	return MongoDBLibrary.RestfulAPIJSONPatch(collName, filter, patchJSON)
}
func (db *MongoDBClient) RestfulAPIJSONPatchExtend(collName string, filter bson.M, patchJSON []byte, dataName string) bool {
	return MongoDBLibrary.RestfulAPIJSONPatchExtend(collName, filter, patchJSON, dataName)
}
func (db *MongoDBClient) RestfulAPIPost(collName string, filter bson.M, postData map[string]interface{}) bool {
	return MongoDBLibrary.RestfulAPIPost(collName, filter, postData)
}
func (db *MongoDBClient) RestfulAPIPostMany(collName string, filter bson.M, postDataArray []interface{}) bool {
	return MongoDBLibrary.RestfulAPIPostMany(collName, filter, postDataArray)
}
