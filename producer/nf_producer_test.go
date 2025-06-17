// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package producer_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/omec-project/nrf/context"
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

func TestNFRegisterProcedureSuccess(t *testing.T) {
	testCases := []struct {
		name                      string
		nrfPlmnList               []models.PlmnId
		nfPlmnList                *[]models.PlmnId
		expectedNfProfilePlmnList []models.PlmnId
	}{
		{
			name: "NF with no provided PLMNs and NRF with PLMNs",
			nrfPlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			nfPlmnList: nil,
			expectedNfProfilePlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
		},
		{
			name: "NF with provided empty PLMNs and NRF with PLMNs",
			nrfPlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			nfPlmnList: &[]models.PlmnId{},
			expectedNfProfilePlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
		},
		{
			name: "NF with provided PLMNs and NRF with PLMNs",
			nrfPlmnList: []models.PlmnId{
				{
					Mcc: "999",
					Mnc: "99",
				},
			},
			nfPlmnList: &[]models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			expectedNfProfilePlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
		},
		{
			name:        "NF with provided PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList: &[]models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			expectedNfProfilePlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalDBClient := dbadapter.DBClient
			originalNrfContextPlmnList := context.GetSelf().PlmnList
			defer func() {
				dbadapter.DBClient = originalDBClient
				context.GetSelf().PlmnList = originalNrfContextPlmnList
			}()
			context.GetSelf().PlmnList = tc.nrfPlmnList
			dbadapter.DBClient = &MockMongoDBClient{}
			var nf models.NfProfile
			nf.NfType = models.NfType_AUSF
			nf.NfInstanceId = uuid.New().String()
			nf.NfStatus = models.NfStatus_REGISTERED
			nf.PlmnList = tc.nfPlmnList
			_, data, err := producer.NFRegisterProcedure(nf)
			if err != nil {
				t.Errorf("failed to register NF: %v", err)
			}
			rawNfPlmns, _ := json.Marshal(data["plmnList"])
			var nfPlmns []models.PlmnId
			json.Unmarshal(rawNfPlmns, &nfPlmns)
			if !reflect.DeepEqual(tc.expectedNfProfilePlmnList, nfPlmns) {
				t.Errorf("Expected %v, got %v", tc.expectedNfProfilePlmnList, nfPlmns)
			}
		})
	}
}

func TestNFRegisterProcedureFailure(t *testing.T) {
	testCases := []struct {
		name        string
		nrfPlmnList []models.PlmnId
		nfPlmnList  *[]models.PlmnId
	}{
		{
			name:        "NF with no provided PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList:  nil,
		},
		{
			name:        "NF with provided empty PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList:  &[]models.PlmnId{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalDBClient := dbadapter.DBClient
			originalNrfContextPlmnList := context.GetSelf().PlmnList
			defer func() {
				dbadapter.DBClient = originalDBClient
				context.GetSelf().PlmnList = originalNrfContextPlmnList
			}()
			context.GetSelf().PlmnList = tc.nrfPlmnList
			dbadapter.DBClient = &MockMongoDBClient{}
			var nf models.NfProfile
			nf.NfType = models.NfType_AUSF
			nf.NfInstanceId = uuid.New().String()
			nf.NfStatus = models.NfStatus_REGISTERED
			nf.PlmnList = tc.nfPlmnList
			_, data, err := producer.NFRegisterProcedure(nf)
			if err == nil {
				t.Errorf("Expected error, got: %v", data)
			}
		})
	}
}
