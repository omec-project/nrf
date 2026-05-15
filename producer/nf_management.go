// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-viper/mapstructure/v2"
	nrfContext "github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	stats "github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/bson"
)

const nfStatusNotifyTimeout = 10 * time.Second

var nfStatusNotifyHTTPClient = &http.Client{Timeout: nfStatusNotifyTimeout}

func normalizeNFInstancePatchJSON(patchJSON []byte) []byte {
	var patchItems []models.PatchItem
	if err := json.Unmarshal(patchJSON, &patchItems); err != nil {
		return patchJSON
	}

	changed := false
	for index := range patchItems {
		if patchItems[index].Path == "/nfStatus" {
			patchItems[index].Path = "/nfstatus"
			changed = true
		}
	}

	if !changed {
		return patchJSON
	}

	normalizedPatchJSON, err := json.Marshal(patchItems)
	if err != nil {
		return patchJSON
	}

	return normalizedPatchJSON
}

func HandleNFDeregisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle NFDeregisterRequest")
	nfInstanceId := request.Params["nfInstanceID"]

	nfType, problemDetails := NFDeregisterProcedure(nfInstanceId)

	if problemDetails != nil {
		logger.ManagementLog.Debugln("deregister failure")
		stats.IncrementNrfRegistrationsStats("deregister", nfType, "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		logger.ManagementLog.Debugln("deregister Success")
		stats.IncrementNrfRegistrationsStats("deregister", nfType, "SUCCESS")
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func HandleGetNFInstanceRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle GetNFInstanceRequest")
	nfInstanceId := request.Params["nfInstanceID"]

	response := GetNFInstanceProcedure(nfInstanceId)

	if response != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusNotFound)
		problemDetails.SetCause("CONTEXT_NOT_FOUND")
		problemDetails.SetDetail("NF instance not found")
		return httpwrapper.NewResponse(http.StatusNotFound, nil, problemDetails)
	}
}

func HandleNFRegisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle NFRegisterRequest")
	nfProfile := request.Body.(models.NFProfile)

	header, response, problemDetails := NFRegisterProcedure(nfProfile)

	if response != nil {
		logger.ManagementLog.Debugln("register success")
		stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "SUCCESS")
		return httpwrapper.NewResponse(http.StatusCreated, header, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("register failed")
		stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	logger.ManagementLog.Debugln("register failed")
	stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "FAILURE")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleUpdateNFInstanceRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle UpdateNFInstanceRequest")
	nfInstanceID := request.Params["nfInstanceID"]
	if nfInstanceID == "" {
		logger.ManagementLog.Errorln("nfInstanceID is missing")
		problemDetails := utils.ProblemDetailsMalformedRequestSyntax("Missing nfInstanceID")
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, problemDetails)
	}

	patchJSON, ok := request.Body.([]byte)
	if !ok {
		logger.ManagementLog.Errorln("invalid body format")
		problemDetails := utils.ProblemDetailsMalformedRequestSyntax("Invalid body format")
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, problemDetails)
	}
	patchJSON = normalizeNFInstancePatchJSON(patchJSON)

	response, err := updateNFInstanceProcedure(nfInstanceID, patchJSON)
	if err != nil {
		logger.ManagementLog.Errorln("updateNFInstanceProcedure failed:", err)
		problemDetails := utils.ProblemDetailsSystemFailure("Update procedure failed")
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}

	if response == nil {
		logger.ManagementLog.Errorln("received nil response after update procedure")
		problemDetails := utils.ProblemDetailsSystemFailure("Update procedure returned nil response")
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}

	nfType := string(response.GetNfType())
	if nfType == "" {
		logger.ManagementLog.Warnln("response missing NF type")
		nfType = "unknown"
	}

	stats.IncrementNrfRegistrationsStats("update", nfType, "SUCCESS")
	return httpwrapper.NewResponse(http.StatusOK, nil, response)
}

func HandleGetNFInstancesRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("handle GetNFInstancesRequest")
	nfType := request.Query.Get("nf-type")
	limitRaw := request.Query.Get("limit")
	limit, err := strconv.Atoi(limitRaw)
	if err != nil {
		logger.ManagementLog.Errorln("error converting limit query parameter:", limitRaw, err)
		problemDetails := models.ProblemDetails{
			Title:  openapi.PtrString("Invalid Parameter"),
			Status: openapi.PtrInt32(int32(http.StatusBadRequest)),
			Detail: openapi.PtrString(err.Error()),
		}

		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	response, problemDetails := GetNFInstancesProcedure(nfType, limit)
	if response != nil {
		logger.ManagementLog.Debugln("GetNFInstances success")
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("GetNFInstances failed")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	logger.ManagementLog.Debugln("GetNFInstances failed")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleRemoveSubscriptionRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle RemoveSubscription")
	subscriptionID := request.Params["subscriptionID"]

	nfType := GetNfTypeBySubscriptionID(request.Params["subscriptionID"])
	RemoveSubscriptionProcedure(subscriptionID)
	stats.IncrementNrfSubscriptionsStats("unsubscribe", nfType, "SUCCESS")

	return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
}

func HandleUpdateSubscriptionRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle UpdateSubscription")
	subscriptionID := request.Params["subscriptionID"]
	patchJSON := request.Body.([]byte)

	nfType := GetNfTypeBySubscriptionID(subscriptionID)
	response := UpdateSubscriptionProcedure(subscriptionID, patchJSON)

	if response != nil {
		stats.IncrementNrfSubscriptionsStats("update", nfType, "SUCCESS")
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else {
		stats.IncrementNrfSubscriptionsStats("update", nfType, "FAILURE")
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func HandleCreateSubscriptionRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle CreateSubscriptionRequest")
	subscription := request.Body.(models.SubscriptionData)

	response, problemDetails := CreateSubscriptionProcedure(subscription)
	if response != nil {
		logger.ManagementLog.Debugln("CreateSubscription success")
		stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.GetReqNfType()), "SUCCESS")
		return httpwrapper.NewResponse(http.StatusCreated, nil, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("CreateSubscription failed")
		stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.GetReqNfType()), "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	logger.ManagementLog.Debugln("CreateSubscription failed")
	stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.GetReqNfType()), "FAILURE")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func CreateSubscriptionProcedure(subscription models.SubscriptionData) (response bson.M,
	problemDetails *models.ProblemDetails,
) {
	subscription.SubscriptionId = openapi.PtrString(nrfContext.SetsubscriptionId())

	tmp, err := json.Marshal(subscription)
	if err != nil {
		logger.ManagementLog.Errorln("Marshal error in CreateSubscriptionProcedure: ", err)
	}
	putData := bson.M{}
	err = json.Unmarshal(tmp, &putData)
	if err != nil {
		logger.ManagementLog.Errorln("Unmarshal error in CreateSubscriptionProcedure: ", err)
	}

	// TODO: need to store Condition !
	if ok, _ := dbadapter.DBClient.RestfulAPIPost("Subscriptions", bson.M{"subscriptionId": subscription.GetSubscriptionId()},
		putData); !ok { // subscription id not exist before
		return putData, nil
	} else {
		problemDetails = models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusBadRequest)
		problemDetails.SetCause("CREATE_SUBSCRIPTION_ERROR")
		return nil, problemDetails
	}
}

func UpdateSubscriptionProcedure(subscriptionID string, patchJSON []byte) (response map[string]interface{}) {
	collName := "Subscriptions"
	filter := bson.M{"subscriptionId": subscriptionID}

	err := dbadapter.DBClient.RestfulAPIJSONPatch(collName, filter, patchJSON)
	if err == nil {
		response, _ = dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		return response
	} else {
		logger.ManagementLog.Warnln("Error UpdateSubscriptionProcedure: ", err)
		return nil
	}
}

func RemoveSubscriptionProcedure(subscriptionID string) {
	collName := "Subscriptions"
	filter := bson.M{"subscriptionId": subscriptionID}
	logger.ManagementLog.Infoln("removing SubscriptionId:", subscriptionID)

	err := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Errorf("failed to remove subscription with ID %s: %v", subscriptionID, err)
		return
	}
	logger.ManagementLog.Infof("removed subscription with ID %s", subscriptionID)
}

func GetNFInstancesProcedure(nfType string, limit int) (response *nrfContext.UriList,
	problemDetail *models.ProblemDetails,
) {
	// nfType := c.Query("nf-type")
	// limit, err := strconv.Atoi(c.Query("limit"))
	collName := "urilist"
	filter := bson.M{"nfType": nfType}

	UL, _ := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	logger.ManagementLog.Infoln("UL: ", UL)
	originalUL := &nrfContext.UriList{}
	err := mapstructure.Decode(UL, originalUL)
	if err != nil {
		logger.ManagementLog.Errorln("Decode error in GetNFInstancesProcedure: ", err)
		problemDetail := utils.ProblemDetailsSystemFailure(err.Error())
		return nil, problemDetail
	}
	nrfContext.NnrfUriListLimit(originalUL, limit)
	// c.JSON(http.StatusOK, originalUL)
	return originalUL, nil
}

func NFDeleteAll(nfType string) (problemDetails *models.ProblemDetails) {
	collName := "NfProfile"
	filter := bson.M{"nftype": nfType}

	err := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Errorf("failed to delete NF profiles of type %s: %v", nfType, err)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetTitle("NF Profiles Deletion Failed")
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetDetail(err.Error())
		return problemDetails
	}

	logger.ManagementLog.Infof("successfully deleted NF profiles of type %s", nfType)
	return nil
}

func NFDeregisterProcedure(nfInstanceID string) (nfType string, problemDetails *models.ProblemDetails) {
	collName := "NfProfile"
	filter := bson.M{"nfinstanceid": nfInstanceID}
	nfType = GetNfTypeByNfInstanceID(nfInstanceID)

	nfProfilesRaw, err := dbadapter.DBClient.RestfulAPIGetMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Warnln("error fetching NF profiles:", err)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("FETCH_ERROR")
		problemDetails.SetDetail(err.Error())
		return "", problemDetails
	}

	time.Sleep(time.Duration(1) * time.Second)

	deleteManyErr := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if deleteManyErr != nil {
		logger.ManagementLog.Warnln("error in deleting NF profiles:", deleteManyErr)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("NF_DELETE_ERROR")
		problemDetails.SetDetail(deleteManyErr.Error())
		return "", problemDetails
	}

	// nfProfile data for response
	nfProfiles, err := util.Decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.ManagementLog.Warnln("Time decode error: ", err)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("NOTIFICATION_ERROR")
		problemDetails.SetDetail(err.Error())
		return "", problemDetails
	}

	// NF Down Notification to other instances of same NfType
	if len(nfProfiles) != 0 {
		nfProfile0 := util.ConvertNFProfileDiscoveryToNFProfile(nfProfiles[0])
		sendNFDownNotification(nfProfile0, nfInstanceID)
		uriList := nrfContext.GetNotificationUri(nfProfile0)
		nfInstanceUri := nrfContext.GetNfInstanceURI(nfInstanceID)
		// set info for NotificationData
		Notification_event := models.NOTIFICATIONEVENTTYPE_NF_DEREGISTERED
		for _, uri := range uriList {
			logger.ManagementLog.Infof("status Notification Uri: %v", uri)
			problemDetails = SendNFStatusNotify(Notification_event, nfInstanceUri, uri)
			if problemDetails != nil {
				logger.ManagementLog.Infoln("error in status notify", problemDetails)
			}
		}
	}

	// delete subscriptions of deregistered NF instance
	filter = bson.M{"subscrCond.nfInstanceId": nfInstanceID}
	deleteErr := dbadapter.DBClient.RestfulAPIDeleteMany("Subscriptions", filter)
	if deleteErr != nil {
		logger.ManagementLog.Warnln("error in deleting subscriptions:", deleteErr)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("SUBSCRIPTION_DELETE_ERROR")
		problemDetails.SetDetail(deleteErr.Error())
		return "", problemDetails
	}

	return nfType, nil
}

func sendNFDownNotification(nfProfile models.NFProfile, nfInstanceID string) {
	if nfProfile.NfType == models.NFTYPE_AMF {
		url := "http://amf:29518" + "/namf-oam/v1/amfInstanceDown/" + nfInstanceID
		notifyCtx, cancel := context.WithTimeout(context.Background(), nfStatusNotifyTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(notifyCtx, http.MethodPost, url, nil)
		if err != nil {
			logger.ManagementLog.Infoln("Error in creating request ", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := nfStatusNotifyHTTPClient.Do(req)
		if err != nil {
			logger.ManagementLog.Infoln("Errored when sending request to the server", err)
			return
		}
		if resp != nil && resp.Body != nil {
			defer func() {
				if bodyCloseErr := resp.Body.Close(); bodyCloseErr != nil {
					logger.ManagementLog.Errorf("NF down notification response body cannot close: %+v", bodyCloseErr)
				}
			}()
		}
	}
}

func updateNFInstanceProcedure(nfInstanceID string, patchJSON []byte) (*models.NFProfile, error) {
	// Validation for NF Instance ID
	if nfInstanceID == "" {
		logger.ManagementLog.Errorln("nf Instance ID is required")
		return nil, fmt.Errorf("NF Instance ID is required")
	}
	collName := "NfProfile"
	filter := bson.M{"nfinstanceid": nfInstanceID}

	// Patch the existing NF Instance
	patchError := dbadapter.DBClient.RestfulAPIJSONPatch(collName, filter, patchJSON)
	if patchError != nil {
		logger.ManagementLog.Errorln("patch error in UpdateNFInstanceProcedure:", patchError)
		return nil, fmt.Errorf("patch error: %v", patchError)
	}
	// Get the updated NF Instance
	nf, getErr := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if getErr != nil || nf == nil {
		logger.ManagementLog.Errorln("failed to get NF instance:", getErr)
		return nil, fmt.Errorf("failed to get NF instance: %v", getErr)
	}

	nfProfilesRaw := []map[string]interface{}{nf}

	// Decode NF instance
	nfProfiles, decodeErr := util.Decode(nfProfilesRaw, time.RFC3339)
	if decodeErr != nil {
		logger.ManagementLog.Errorln("decoding error:", decodeErr)
		return nil, fmt.Errorf("decoding error: %v", decodeErr)
	}

	if len(nfProfiles) == 0 {
		// Handle empty decoded profiles case
		logger.ManagementLog.Errorln("decoded NF profiles are empty")
		return nil, fmt.Errorf("decoded NF profiles are empty")
	}

	// Update expiry time if enabled
	// Currently we are using 3 times the hearbeat timer as the expiry time interval.
	// We should update it to be configurable : TBD
	if factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		timein := time.Now().Local().Add(time.Second * time.Duration(factory.NrfConfig.Configuration.NfKeepAliveTime*3))
		nf["expireAt"] = timein
	}
	// Put the updated NF instance
	_, putErr := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, nf)
	if putErr != nil {
		logger.ManagementLog.Errorf("nf profile [%s] update failed: %v", nfProfiles[0].NfType, putErr)
		return nil, fmt.Errorf("NF profile update is failed: %v", putErr)
	}

	logger.ManagementLog.Infof("nf profile [%s] update success", nfProfiles[0].NfType)
	updatedProfile := util.ConvertNFProfileDiscoveryToNFProfile(nfProfiles[0])
	return &updatedProfile, nil
}

func GetNFInstanceProcedure(nfInstanceID string) *models.NFProfile {
	collName := "NfProfile"
	filter := bson.M{"nfinstanceid": nfInstanceID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil || response == nil {
		return nil
	}

	decodedProfiles, decodeErr := util.Decode([]map[string]any{response}, time.RFC3339)
	if decodeErr != nil || len(decodedProfiles) == 0 {
		logger.ManagementLog.Warnf("failed to decode NF profile for %s: %v", nfInstanceID, decodeErr)
		return nil
	}

	nfProfile := util.ConvertNFProfileDiscoveryToNFProfile(decodedProfiles[0])
	return &nfProfile
}

func NFRegisterProcedure(nfProfile models.NFProfile) (header http.Header, response *models.NFProfile,
	problemDetails *models.ProblemDetails,
) {
	logger.ManagementLog.Debugln("[NRF] In NFRegisterProcedure")
	var nf models.NFProfile
	err := nrfContext.NnrfNFManagementDataModel(&nf, nfProfile)
	if err != nil {
		logger.ManagementLog.Errorln("NfProfile Validation failed", err)
		problemDetails = models.NewProblemDetails()
		problemDetails.SetTitle(nfProfile.NfInstanceId)
		problemDetails.SetStatus(http.StatusBadRequest)
		problemDetails.SetCause("INVALID_REQUEST")
		problemDetails.SetDetail(err.Error())
		return nil, nil, problemDetails
	}

	// make location header
	locationHeaderValue := nrfContext.SetLocationHeader(nfProfile)

	// Marshal nf to bson
	putData := bson.M{}
	bsonBytes, err := bson.Marshal(nf)
	if err != nil {
		logger.ManagementLog.Errorln("bson marshal error in NFRegisterProcedure:", err)
		problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
		return nil, nil, problemDetails
	}

	err = bson.Unmarshal(bsonBytes, &putData)
	if err != nil {
		logger.ManagementLog.Errorln("bson unmarshal error in NFRegisterProcedure:", err)
		problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
		return nil, nil, problemDetails
	}

	// set db info
	collName := "NfProfile"
	nfInstanceId := nf.GetNfInstanceId()
	filter := bson.M{"nfinstanceid": nfInstanceId}

	// fallback to older approach
	if !factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		NFDeleteAll(string(nf.NfType))
	} else {
		timein := time.Now().Local().Add(time.Second * time.Duration(nf.GetHeartBeatTimer()*3))
		putData["expireAt"] = timein
		nfs, _ := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		if len(nfs) == 0 {
			putData["createdAt"] = time.Now()
		}
	}

	// Update NF Profile case
	if ok, _ := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, putData); ok { // true insert
		logger.ManagementLog.Infoln("RestfulAPIPutOne True Insert")
		uriList := nrfContext.GetNotificationUri(nf)

		// set info for NotificationData
		Notification_event := models.NOTIFICATIONEVENTTYPE_NF_PROFILE_CHANGED
		nfInstanceUri := locationHeaderValue

		// receive the rsp from handler
		for _, uri := range uriList {
			problemDetails = SendNFStatusNotify(Notification_event, nfInstanceUri, uri)
			if problemDetails != nil {
				return nil, nil, problemDetails
			}
		}

		header = make(http.Header)
		header.Add("Location", locationHeaderValue)
		return header, &nf, nil
	} else { // Create NF Profile case
		logger.ManagementLog.Infoln("create NF Profile", nfProfile.GetNfType())
		uriList := nrfContext.GetNotificationUri(nf)
		// set info for NotificationData
		notification_event := models.NOTIFICATIONEVENTTYPE_NF_REGISTERED
		nfInstanceUri := locationHeaderValue

		for _, uri := range uriList {
			problemDetails = SendNFStatusNotify(notification_event, nfInstanceUri, uri)
			if problemDetails != nil {
				return nil, nil, problemDetails
			}
		}

		header = make(http.Header)
		header.Add("Location", locationHeaderValue)
		logger.ManagementLog.Infoln("location header:", locationHeaderValue)
		return header, &nf, nil
	}
}

func GetNfTypeBySubscriptionID(subscriptionID string) (nfType string) {
	collName := "Subscriptions"
	filter := bson.M{"subscriptionId": subscriptionID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return "UNKNOWN_NF"
	}
	if response["reqNfType"] != nil {
		return fmt.Sprint(response["reqNfType"])
	}
	return "UNKNOWN_NF"
}

func GetNfTypeByNfInstanceID(nfInstanceID string) (nfType string) {
	collName := "NfProfile"
	filter := bson.M{"nfinstanceid": nfInstanceID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return "UNKNOWN_NF"
	}
	if response["nftype"] != nil {
		return fmt.Sprint(response["nftype"])
	}
	return "UNKNOWN_NF"
}

func SendNFStatusNotify(Notification_event models.NotificationEventType, nfInstanceUri string,
	url string,
) *models.ProblemDetails {
	notificationData := models.NotificationData{
		Event:         Notification_event,
		NfInstanceUri: nfInstanceUri,
	}
	body, err := json.Marshal(notificationData)
	if err != nil {
		logger.ManagementLog.Infof("notify fail: %+v", err)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("NOTIFICATION_ERROR")
		problemDetails.SetDetail(err.Error())
		return problemDetails
	}

	notifyCtx, cancel := context.WithTimeout(context.Background(), nfStatusNotifyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(notifyCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logger.ManagementLog.Infof("notify fail: %+v", err)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("NOTIFICATION_ERROR")
		problemDetails.SetDetail(err.Error())
		return problemDetails
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, application/problem+json")

	res, err := nfStatusNotifyHTTPClient.Do(req)
	if err != nil {
		logger.ManagementLog.Infof("notify fail: %+v", err)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause("NOTIFICATION_ERROR")
		problemDetails.SetDetail(err.Error())
		return problemDetails
	}
	if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ManagementLog.Errorf("NotificationApi response body cannot close: %+v", resCloseErr)
			}
		}()
		if status := res.StatusCode; status != http.StatusNoContent && status != http.StatusOK {
			logger.ManagementLog.Warnln("error status in NotificationPost:", status)
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(int32(status))
			problemDetails.SetCause("NOTIFICATION_ERROR")
			responseBody, readErr := io.ReadAll(res.Body)
			if readErr == nil && len(responseBody) > 0 {
				var remoteProblem models.ProblemDetails
				if decodeErr := json.Unmarshal(responseBody, &remoteProblem); decodeErr == nil {
					return &remoteProblem
				}
				problemDetails.SetDetail(string(responseBody))
			}
			return problemDetails
		}
	}
	return nil
}
