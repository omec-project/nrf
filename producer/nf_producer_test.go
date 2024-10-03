// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package producer_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/producer"
	"github.com/omec-project/openapi/models"
	"go.mongodb.org/mongo-driver/bson"
)

type MockMongoDBClient struct {
	dbadapter.DBInterface
}

func init() {
	factory.InitConfigFactory("../nrfTest/nrfcfg.yaml")
}

func (db *MockMongoDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIGetOne")
	return nil, nil
}

func (db *MockMongoDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIGetMany")
	return nil, nil
}
func (db *MockMongoDBClient) PutOneWithTimeout(collName string, filter bson.M, putData map[string]interface{}, timeout int32, timeField string) bool {
	logger.HandlerLog.Infoln("called Mock PutOneWithTimeout")
	return true
}
func (db *MockMongoDBClient) RestfulAPIPutOne(collName string, filter bson.M, putData map[string]interface{}) (bool, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIPutOne")
	return true, nil
}
func (db *MockMongoDBClient) RestfulAPIPutOneNotUpdate(collName string, filter bson.M, putData map[string]interface{}) (bool, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIPutOneNotUpdate")
	return true, nil
}
func (db *MockMongoDBClient) RestfulAPIPutMany(collName string, filterArray []bson.M, putDataArray []map[string]interface{}) error {
	logger.HandlerLog.Infoln("called Mock RestfulAPIPutMany")
	return nil
}
func (db *MockMongoDBClient) RestfulAPIDeleteOne(collName string, filter bson.M) error {
	logger.HandlerLog.Infoln("called Mock RestfulAPIDeleteOne")
	return nil
}
func (db *MockMongoDBClient) RestfulAPIDeleteMany(collName string, filter bson.M) error {
	logger.HandlerLog.Infoln("called Mock RestfulAPIDeleteMany")
	return nil
}
func (db *MockMongoDBClient) RestfulAPIMergePatch(collName string, filter bson.M, patchData map[string]interface{}) error {
	logger.HandlerLog.Infoln("called Mock RestfulAPIMergePatch")
	return nil
}
func (db *MockMongoDBClient) RestfulAPIJSONPatch(collName string, filter bson.M, patchJSON []byte) error {
	return nil
}
func (db *MockMongoDBClient) RestfulAPIJSONPatchExtend(collName string, filter bson.M, patchJSON []byte, dataName string) error {
	logger.HandlerLog.Infoln("called Mock RestfulAPIJSONPatchExtend")
	return nil
}
func (db *MockMongoDBClient) RestfulAPIPost(collName string, filter bson.M, postData map[string]interface{}) (bool, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIPost")
	return true, nil
}
func (db *MockMongoDBClient) RestfulAPIPostMany(collName string, filter bson.M, postDataArray []interface{}) bool {
	logger.HandlerLog.Infoln("called Mock RestfulAPIPost")
	return true
}

func TestNFRegisterProcedure(t *testing.T) {
	dbadapter.DBClient = &MockMongoDBClient{}
	var nf models.NfProfile
	nf.NfType = models.NfType_PCF
	nf.NfInstanceId = uuid.New().String()
	nf.NfStatus = models.NfStatus_REGISTERED
	_, _, err := producer.NFRegisterProcedure(nf)
	if err != nil {
		t.Errorf("testcase failed: %v", err)
	}
}
