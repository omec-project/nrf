// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package producer_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/polling"
	"github.com/omec-project/nrf/producer"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testUpdateNfInstanceId   = "instance-1"
	testNfInstanceIDParamKey = "nfInstanceID"
)

type MockMongoDBClient struct {
	dbadapter.DBInterface
}

func TestMain(m *testing.M) {
	if err := factory.InitConfigFactory("../nrfTest/nrfcfg.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, "error in InitConfigFactory:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
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

func (db *MockMongoDBClient) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, expectedCurrent, putData map[string]interface{}) (bool, error) {
	logger.HandlerLog.Infoln("called Mock RestfulAPIReplaceIfUnchanged")
	return true, nil
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
		nfPlmnList                []models.PlmnId
		expectedNfProfilePlmnList []models.PlmnId
		expectedWebconsoleCalled  bool
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
			expectedWebconsoleCalled: true,
		},
		{
			name: "NF with provided empty PLMNs and NRF with PLMNs",
			nrfPlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			nfPlmnList: []models.PlmnId{},
			expectedNfProfilePlmnList: []models.PlmnId{
				{
					Mcc: "001",
					Mnc: "01",
				},
			},
			expectedWebconsoleCalled: true,
		},
		{
			name: "NF with provided PLMNs and NRF with PLMNs",
			nrfPlmnList: []models.PlmnId{
				{
					Mcc: "999",
					Mnc: "99",
				},
			},
			nfPlmnList: []models.PlmnId{
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
			expectedWebconsoleCalled: false,
		},
		{
			name:        "NF with provided PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList: []models.PlmnId{
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
			expectedWebconsoleCalled: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webconsoleCalled := false
			originalDBClient := dbadapter.DBClient
			originalFetchPlmnConfig := polling.FetchPlmnConfig
			defer func() {
				dbadapter.DBClient = originalDBClient
				polling.FetchPlmnConfig = originalFetchPlmnConfig
			}()
			polling.FetchPlmnConfig = func() ([]models.PlmnId, error) {
				webconsoleCalled = true
				return tc.nrfPlmnList, nil
			}
			dbadapter.DBClient = &MockMongoDBClient{}
			nf := models.NewNFProfileWithDefaults()
			nf.SetNfType(models.NFTYPE_AUSF)
			nf.SetNfInstanceId(uuid.New().String())
			nf.SetNfStatus(models.NFSTATUS_REGISTERED)
			nf.SetPlmnList(tc.nfPlmnList)
			_, data, err := producer.NFRegisterProcedure(*nf)
			if err != nil {
				t.Fatalf("failed to register NF: %v", err)
			}
			var nfPlmns []models.PlmnId
			if data != nil {
				nfPlmns = data.GetPlmnList()
			}
			if !reflect.DeepEqual(tc.expectedNfProfilePlmnList, nfPlmns) {
				t.Errorf("Expected %v, got %v", tc.expectedNfProfilePlmnList, nfPlmns)
			}
			if tc.expectedWebconsoleCalled != webconsoleCalled {
				t.Errorf("Expected webconsole calls: %v, got: %v", tc.expectedWebconsoleCalled, webconsoleCalled)
			}
		})
	}
}

func TestNFRegisterProcedureFailure(t *testing.T) {
	testCases := []struct {
		name        string
		nrfPlmnList []models.PlmnId
		nfPlmnList  []models.PlmnId
	}{
		{
			name:        "NF with no provided PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList:  nil,
		},
		{
			name:        "NF with provided empty PLMNs and NRF with no PLMNs",
			nrfPlmnList: []models.PlmnId{},
			nfPlmnList:  []models.PlmnId{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webconsoleCalled := false
			originalDBClient := dbadapter.DBClient
			originalFetchPlmnConfig := polling.FetchPlmnConfig
			defer func() {
				dbadapter.DBClient = originalDBClient
				polling.FetchPlmnConfig = originalFetchPlmnConfig
			}()
			polling.FetchPlmnConfig = func() ([]models.PlmnId, error) {
				webconsoleCalled = true
				return tc.nrfPlmnList, nil
			}
			dbadapter.DBClient = &MockMongoDBClient{}
			nf := models.NewNFProfileWithDefaults()
			nf.SetNfType(models.NFTYPE_AUSF)
			nf.SetNfInstanceId(uuid.New().String())
			nf.SetNfStatus(models.NFSTATUS_REGISTERED)
			nf.SetPlmnList(tc.nfPlmnList)
			_, data, err := producer.NFRegisterProcedure(*nf)
			if err == nil {
				t.Errorf("Expected error, got: %v", data)
			}
			if !webconsoleCalled {
				t.Error("Expected webconsole to be called, it was not")
			}
		})
	}
}

func TestNFRegisterProcedureFailureNoProvidedPlmnListAndWebconsoleUnreachable(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	originalFetchPlmnConfig := polling.FetchPlmnConfig
	defer func() {
		dbadapter.DBClient = originalDBClient
		polling.FetchPlmnConfig = originalFetchPlmnConfig
	}()
	polling.FetchPlmnConfig = func() ([]models.PlmnId, error) {
		return nil, errors.New("http error")
	}
	dbadapter.DBClient = &MockMongoDBClient{}
	nf := models.NewNFProfileWithDefaults()
	nf.SetNfType(models.NFTYPE_AUSF)
	nf.SetNfInstanceId(uuid.New().String())
	nf.SetNfStatus(models.NFSTATUS_REGISTERED)
	_, data, err := producer.NFRegisterProcedure(*nf)
	if err == nil {
		t.Errorf("Expected error, got: %v", data)
	}
}

// ReplaceCaptureDBClient captures every document that
// updateNFInstanceProcedure attempts to persist via the atomic
// RestfulAPIReplaceIfUnchanged, so a test can inspect the exact candidate
// produced by applying a JSON Patch to validPreviousNfDoc().
type ReplaceCaptureDBClient struct {
	MockMongoDBClient
	replaceCalls []map[string]interface{}
}

func (db *ReplaceCaptureDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return validPreviousNfDoc(), nil
}

func (db *ReplaceCaptureDBClient) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, expectedCurrent, putData map[string]interface{}) (bool, error) {
	db.replaceCalls = append(db.replaceCalls, putData)
	return true, nil
}

func TestHandleUpdateNFInstanceRequestAppliesNfStatusPatch(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	replaceCaptureDBClient := &ReplaceCaptureDBClient{}
	dbadapter.DBClient = replaceCaptureDBClient

	response := producer.HandleUpdateNFInstanceRequest(nfStatusRegisteredPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Status)
	}
	if len(replaceCaptureDBClient.replaceCalls) != 1 {
		t.Fatalf("expected exactly one persist attempt, got %d", len(replaceCaptureDBClient.replaceCalls))
	}
	persisted := replaceCaptureDBClient.replaceCalls[0]
	if status, _ := persisted["nfstatus"].(string); status != string(models.NFSTATUS_REGISTERED) {
		t.Fatalf("expected the persisted document's lowercase BSON key nfstatus to be set, got %+v", persisted)
	}
}

// metadataPreservingReplaceCaptureDBClient is a ReplaceCaptureDBClient whose
// RestfulAPIGetOne returns validPreviousNfDoc() augmented with "createdAt",
// mirroring the field NFRegisterProcedure stamps on registration (see
// nf_management.go) that has no counterpart in models.NFProfile.
type metadataPreservingReplaceCaptureDBClient struct {
	ReplaceCaptureDBClient
}

func (db *metadataPreservingReplaceCaptureDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	doc := validPreviousNfDoc()
	doc["createdAt"] = time.Now()
	return doc, nil
}

// TestHandleUpdateNFInstanceRequestPreservesNonModelMetadata verifies that a
// PATCH does not drop document fields that have no counterpart in
// models.NFProfile, such as "createdAt": the candidate persisted via
// RestfulAPIReplaceIfUnchanged is built solely from the patched
// models.NFProfile, so without carrying such fields over from the pre-patch
// snapshot, every PATCH would silently erase them.
func TestHandleUpdateNFInstanceRequestPreservesNonModelMetadata(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	dbClient := &metadataPreservingReplaceCaptureDBClient{}
	dbadapter.DBClient = dbClient

	response := producer.HandleUpdateNFInstanceRequest(nfStatusRegisteredPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Status)
	}
	if len(dbClient.replaceCalls) != 1 {
		t.Fatalf("expected exactly one persist attempt, got %d", len(dbClient.replaceCalls))
	}
	persisted := dbClient.replaceCalls[0]
	if _, ok := persisted["createdAt"]; !ok {
		t.Fatalf("expected createdAt to be preserved across the patch, got %+v", persisted)
	}
}

// validPreviousNfDoc returns the pre-patch document used by the update
// tests below: a valid profile with no allowedNfDomains restriction. The
// field names match the lowercase keys real MongoDB documents use (e.g.
// "nfservices", not the model's JSON field name "nfServices"), since that
// is what MongoDB's default BSON marshaling of models.NFProfile (no bson
// struct tags) actually produces.
func validPreviousNfDoc() map[string]interface{} {
	return map[string]interface{}{
		"nfinstanceid": testUpdateNfInstanceId,
		"nftype":       string(models.NFTYPE_AUSF),
		"nfstatus":     string(models.NFSTATUS_REGISTERED),
		"nfservices": []map[string]interface{}{{
			"servicename":     "nausf-auth",
			"scheme":          string(models.URISCHEME_HTTPS),
			"nfservicestatus": string(models.NFSERVICESTATUS_REGISTERED),
		}},
	}
}

// invalidAllowedNfDomainsPatchRequest builds an HandleUpdateNFInstanceRequest
// for testUpdateNfInstanceId whose JSON Patch adds an allowedNfDomains
// pattern invalid for requesterFqdn matching (see ValidateAllowedNfDomains)
// to the profile's only NF service.
func invalidAllowedNfDomainsPatchRequest(t *testing.T) *httpwrapper.Request {
	t.Helper()
	patchJSON, err := json.Marshal([]models.PatchItem{
		{
			Op:    models.PATCHOPERATION_ADD,
			Path:  "/nfServices/0/allowedNfDomains",
			Value: []string{"(unclosed"},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal patch JSON: %v", err)
	}
	return &httpwrapper.Request{
		Params: map[string]string{testNfInstanceIDParamKey: testUpdateNfInstanceId},
		Body:   patchJSON,
	}
}

// TestHandleUpdateNFInstanceRequestAppliesNestedFieldPatch verifies that a
// JSON Patch path into a nested field, such as
// "/nfServices/0/allowedNfDomains", resolves and applies correctly.
// previousDoc is the raw MongoDB document, whose keys are the driver's
// default-lowercased BSON field names (e.g. "nfservices"), but a real
// client's patch path uses the model's actual JSON field names (e.g.
// "nfServices"); applying the patch directly to the raw document would fail
// with a missing-path error for any field but the specially-cased
// nfStatus/nfstatus.
func TestHandleUpdateNFInstanceRequestAppliesNestedFieldPatch(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	replaceCaptureDBClient := &ReplaceCaptureDBClient{}
	dbadapter.DBClient = replaceCaptureDBClient

	patchJSON, err := json.Marshal([]models.PatchItem{
		{
			Op:    models.PATCHOPERATION_ADD,
			Path:  "/nfServices/0/allowedNfDomains",
			Value: []string{`^.*\.example\.com$`},
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal patch JSON: %v", err)
	}
	request := &httpwrapper.Request{
		Params: map[string]string{testNfInstanceIDParamKey: testUpdateNfInstanceId},
		Body:   patchJSON,
	}

	response := producer.HandleUpdateNFInstanceRequest(request)
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Status)
	}
	if len(replaceCaptureDBClient.replaceCalls) != 1 {
		t.Fatalf("expected exactly one persist attempt, got %d", len(replaceCaptureDBClient.replaceCalls))
	}
}

// nfStatusRegisteredPatchRequest builds an HandleUpdateNFInstanceRequest for
// testUpdateNfInstanceId with a JSON Patch that replaces /nfStatus with
// REGISTERED, the minimal well-formed patch shared by the update tests below.
func nfStatusRegisteredPatchRequest(t *testing.T) *httpwrapper.Request {
	t.Helper()
	patchJSON, err := json.Marshal([]models.PatchItem{
		{
			Op:    models.PATCHOPERATION_REPLACE,
			Path:  "/nfStatus",
			Value: models.NFSTATUS_REGISTERED,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal patch JSON: %v", err)
	}
	return &httpwrapper.Request{
		Params: map[string]string{testNfInstanceIDParamKey: testUpdateNfInstanceId},
		Body:   patchJSON,
	}
}

// rejectingReplaceDBClient fails the test if RestfulAPIReplaceIfUnchanged is
// ever called: it is used to verify that a patch producing an invalid NF
// profile is rejected without persisting anything, not even transiently.
type rejectingReplaceDBClient struct {
	MockMongoDBClient
	t *testing.T
}

func (db *rejectingReplaceDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return validPreviousNfDoc(), nil
}

func (db *rejectingReplaceDBClient) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, expectedCurrent, putData map[string]interface{}) (bool, error) {
	db.t.Fatal("expected RestfulAPIReplaceIfUnchanged not to be called for a patch that produces an invalid NF profile")
	return false, nil
}

// TestHandleUpdateNFInstanceRequestRejectsInvalidPatchedAllowedNfDomainsWithoutPersisting
// verifies that a JSON Patch resulting in an invalid allowedNfDomains
// pattern is rejected with 400 before it is ever attempted to be persisted:
// validating the candidate before the conditional write, rather than
// persisting it first and reverting afterwards, ensures the pattern can
// never be observed by a concurrent discovery query or left behind by a
// crash between the write and a later revert.
func TestHandleUpdateNFInstanceRequestRejectsInvalidPatchedAllowedNfDomainsWithoutPersisting(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	dbadapter.DBClient = &rejectingReplaceDBClient{t: t}

	response := producer.HandleUpdateNFInstanceRequest(invalidAllowedNfDomainsPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Status)
	}
}

// TestHandleUpdateNFInstanceRequestRejectsNfInstanceIdChangeWithoutPersisting
// verifies that a JSON Patch changing /nfInstanceId is rejected with 400
// before it is ever attempted to be persisted: the document is replaced
// using the URL's original nfInstanceID as the filter, so persisting it with
// a different nfInstanceId field value would make it unreachable by its
// original ID and claim an identity that filter was never conditioned on.
func TestHandleUpdateNFInstanceRequestRejectsNfInstanceIdChangeWithoutPersisting(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	dbadapter.DBClient = &rejectingReplaceDBClient{t: t}

	patchJSON, err := json.Marshal([]models.PatchItem{
		{
			Op:    models.PATCHOPERATION_REPLACE,
			Path:  "/nfInstanceId",
			Value: "some-other-instance-id",
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal patch JSON: %v", err)
	}
	request := &httpwrapper.Request{
		Params: map[string]string{testNfInstanceIDParamKey: testUpdateNfInstanceId},
		Body:   patchJSON,
	}

	response := producer.HandleUpdateNFInstanceRequest(request)
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Status)
	}
}

// replaceFailureDBClient simulates a database failure (e.g. a transient
// network error) while persisting an otherwise valid patched profile.
type replaceFailureDBClient struct {
	MockMongoDBClient
}

func (db *replaceFailureDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return validPreviousNfDoc(), nil
}

func (db *replaceFailureDBClient) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, expectedCurrent, putData map[string]interface{}) (bool, error) {
	return false, errors.New("simulated database failure")
}

// TestHandleUpdateNFInstanceRequestSurfacesPersistFailureAs500 verifies that
// a database failure while persisting a validated patch is surfaced as a
// server-side (500) error rather than the 400 used for a rejected patch.
func TestHandleUpdateNFInstanceRequestSurfacesPersistFailureAs500(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	dbadapter.DBClient = &replaceFailureDBClient{}

	response := producer.HandleUpdateNFInstanceRequest(nfStatusRegisteredPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusInternalServerError {
		t.Fatalf("expected status %d when persisting fails, got %d", http.StatusInternalServerError, response.Status)
	}
}

// TestHandleUpdateNFInstanceRequestFoldsExpiryIntoSingleWrite verifies that,
// with NfProfileExpiryEnable set, a patch is persisted with expireAt already
// included in a single conditional write. A prior version added expireAt to
// a second, separate write conditioned on the unstamped in-memory candidate;
// RestfulAPIReplaceIfUnchanged always stamps what it persists with a fresh
// _docVersion, so that second write's condition could never match the
// now-stamped stored document and every such update falsely failed as a
// concurrent-update conflict.
func TestHandleUpdateNFInstanceRequestFoldsExpiryIntoSingleWrite(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	originalExpiryEnable := factory.NrfConfig.Configuration.NfProfileExpiryEnable
	originalKeepAliveTime := factory.NrfConfig.Configuration.NfKeepAliveTime
	defer func() {
		dbadapter.DBClient = originalDBClient
		factory.NrfConfig.Configuration.NfProfileExpiryEnable = originalExpiryEnable
		factory.NrfConfig.Configuration.NfKeepAliveTime = originalKeepAliveTime
	}()
	factory.NrfConfig.Configuration.NfProfileExpiryEnable = true
	factory.NrfConfig.Configuration.NfKeepAliveTime = 30

	replaceCaptureDBClient := &ReplaceCaptureDBClient{}
	dbadapter.DBClient = replaceCaptureDBClient

	response := producer.HandleUpdateNFInstanceRequest(nfStatusRegisteredPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Status)
	}
	if len(replaceCaptureDBClient.replaceCalls) != 1 {
		t.Fatalf("expected exactly one persist attempt, got %d", len(replaceCaptureDBClient.replaceCalls))
	}
	if _, hasExpireAt := replaceCaptureDBClient.replaceCalls[0]["expireAt"]; !hasExpireAt {
		t.Fatalf("expected the single persisted document to include expireAt, got %+v", replaceCaptureDBClient.replaceCalls[0])
	}
}

// conflictingDBClient simulates an NF instance that was modified
// concurrently between updateNFInstanceProcedure's snapshot read and its
// conditional write.
type conflictingDBClient struct {
	MockMongoDBClient
	getOneCalls     int
	replaceAttempts int
}

func (db *conflictingDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	db.getOneCalls++
	return validPreviousNfDoc(), nil
}

func (db *conflictingDBClient) RestfulAPIReplaceIfUnchanged(collName string, filter bson.M, expectedCurrent, putData map[string]interface{}) (bool, error) {
	db.replaceAttempts++
	return false, nil
}

// TestHandleUpdateNFInstanceRequestReturnsConflictOnConcurrentUpdate verifies
// that updateNFInstanceProcedure reports 409 Conflict, without retrying, the
// first time RestfulAPIReplaceIfUnchanged reports a lost race: patchJSON may
// contain array-index paths or remove/move operations that are not safe to
// blindly replay against a document that changed shape since it was
// snapshotted, so the client must re-GET and resubmit instead.
func TestHandleUpdateNFInstanceRequestReturnsConflictOnConcurrentUpdate(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	conflictingClient := &conflictingDBClient{}
	dbadapter.DBClient = conflictingClient

	response := producer.HandleUpdateNFInstanceRequest(nfStatusRegisteredPatchRequest(t))
	if response == nil {
		t.Fatal("expected non-nil response")
	}
	if response.Status != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, response.Status)
	}
	if conflictingClient.replaceAttempts != 1 {
		t.Fatalf("expected exactly 1 persist attempt (no retry), got %d", conflictingClient.replaceAttempts)
	}
	if conflictingClient.getOneCalls != 1 {
		t.Fatalf("expected exactly 1 read (no retry), got %d", conflictingClient.getOneCalls)
	}
}
