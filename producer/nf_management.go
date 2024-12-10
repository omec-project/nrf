// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mitchellh/mapstructure"
	nrf_context "github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	stats "github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/Nnrf_NFManagement"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/bson"
)

func HandleNFDeregisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle NFDeregisterRequest")
	nfInstanceId := request.Params["nfInstanceID"]

	nfType, problemDetails := NFDeregisterProcedure(nfInstanceId)

	if problemDetails != nil {
		logger.ManagementLog.Debugln("deregister failure")
		stats.IncrementNrfRegistrationsStats("deregister", nfType, "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
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
		problemDetails := &models.ProblemDetails{
			Status: http.StatusNotFound,
			Cause:  "UNSPECIFIED",
		}
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
}

func HandleNFRegisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle NFRegisterRequest")
	nfProfile := request.Body.(models.NfProfile)

	header, response, problemDetails := NFRegisterProcedure(nfProfile)

	if response != nil {
		logger.ManagementLog.Debugln("register success")
		stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "SUCCESS")
		return httpwrapper.NewResponse(http.StatusCreated, header, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("register failed")
		stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	logger.ManagementLog.Debugln("register failed")
	stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "FAILURE")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleUpdateNFInstanceRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle UpdateNFInstanceRequest")
	nfInstanceID := request.Params["nfInstanceID"]
	if nfInstanceID == "" {
		logger.ManagementLog.Errorln("nfInstanceID is missing")
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, map[string]string{"error": "Missing nfInstanceID"})
	}

	patchJSON, ok := request.Body.([]byte)
	if !ok {
		logger.ManagementLog.Errorln("invalid body format")
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, map[string]string{"error": "Invalid body format"})
	}

	response, err := updateNFInstanceProcedure(nfInstanceID, patchJSON)
	if err != nil {
		logger.ManagementLog.Errorln("updateNFInstanceProcedure failed:", err)
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, map[string]string{"error": "Update procedure failed"})
	}

	if response == nil {
		logger.ManagementLog.Errorln("received nil response after update procedure")
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, map[string]string{"error": "Update procedure returned nil response"})
	}

	nfType, ok := response["nfType"].(string)
	if !ok {
		logger.ManagementLog.Warnln("response missing 'nfType' or wrong format")
		nfType = "unknown"
	}

	stats.IncrementNrfRegistrationsStats("update", nfType, "SUCCESS")
	return httpwrapper.NewResponse(http.StatusOK, nil, response)
}

func HandleGetNFInstancesRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle GetNFInstancesRequest")
	nfType := request.Query.Get("nf-type")
	limit, err := strconv.Atoi(request.Query.Get("limit"))
	if err != nil {
		logger.ManagementLog.Errorln("Error in string conversion: ", limit)
		problemDetails := models.ProblemDetails{
			Title:  "Invalid Parameter",
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		}

		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}

	response, problemDetails := GetNFInstancesProcedure(nfType, limit)
	if response != nil {
		logger.ManagementLog.Debugln("GetNFInstances success")
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("GetNFInstances failed")
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
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
	subscription := request.Body.(models.NrfSubscriptionData)

	response, problemDetails := CreateSubscriptionProcedure(subscription)
	if response != nil {
		logger.ManagementLog.Debugln("CreateSubscription success")
		stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.ReqNfType), "SUCCESS")
		return httpwrapper.NewResponse(http.StatusCreated, nil, response)
	} else if problemDetails != nil {
		logger.ManagementLog.Debugln("CreateSubscription failed")
		stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.ReqNfType), "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	logger.ManagementLog.Debugln("CreateSubscription failed")
	stats.IncrementNrfSubscriptionsStats("subscribe", string(subscription.ReqNfType), "FAILURE")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func CreateSubscriptionProcedure(subscription models.NrfSubscriptionData) (response bson.M,
	problemDetails *models.ProblemDetails,
) {
	subscription.SubscriptionId = nrf_context.SetsubscriptionId()

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
	if ok, _ := dbadapter.DBClient.RestfulAPIPost("Subscriptions", bson.M{"subscriptionId": subscription.SubscriptionId},
		putData); !ok { // subscription id not exist before
		return putData, nil
	} else {
		problemDetails = &models.ProblemDetails{
			Status: http.StatusBadRequest,
			Cause:  "CREATE_SUBSCRIPTION_ERROR",
		}

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

func GetNFInstancesProcedure(nfType string, limit int) (response *nrf_context.UriList,
	problemDetail *models.ProblemDetails,
) {
	// nfType := c.Query("nf-type")
	// limit, err := strconv.Atoi(c.Query("limit"))
	collName := "urilist"
	filter := bson.M{"nfType": nfType}

	UL, _ := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	logger.ManagementLog.Infoln("UL: ", UL)
	originalUL := &nrf_context.UriList{}
	err := mapstructure.Decode(UL, originalUL)
	if err != nil {
		logger.ManagementLog.Errorln("Decode error in GetNFInstancesProcedure: ", err)
		problemDetail := &models.ProblemDetails{
			Title:  "System failure",
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
			Cause:  "SYSTEM_FAILURE",
		}
		return nil, problemDetail
	}
	nrf_context.NnrfUriListLimit(originalUL, limit)
	// c.JSON(http.StatusOK, originalUL)
	return originalUL, nil
}

func NFDeleteAll(nfType string) (problemDetails *models.ProblemDetails) {
	collName := "NfProfile"
	filter := bson.M{"nfType": nfType}

	err := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Errorln("failed to delete NF profiles of type %s: %v", nfType, err)
		problemDetails = &models.ProblemDetails{
			Title:  "NF Profiles Deletion Failed",
			Status: 500,
			Detail: err.Error(),
		}
		return problemDetails
	}

	logger.ManagementLog.Infoln("successfully deleted NF profiles of type %s", nfType)
	return nil
}

func NFDeregisterProcedure(nfInstanceID string) (nfType string, problemDetails *models.ProblemDetails) {
	collName := "NfProfile"
	filter := bson.M{"nfInstanceId": nfInstanceID}
	nfType = GetNfTypeByNfInstanceID(nfInstanceID)

	nfProfilesRaw, err := dbadapter.DBClient.RestfulAPIGetMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Warnln("error fetching NF profiles:", err)
		problemDetails = &models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "FETCH_ERROR",
			Detail: err.Error(),
		}
		return "", problemDetails
	}

	time.Sleep(time.Duration(1) * time.Second)

	deleteManyErr := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if deleteManyErr != nil {
		logger.ManagementLog.Warnln("error in deleting NF profiles:", deleteManyErr)
		problemDetails = &models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "NF_DELETE_ERROR",
			Detail: deleteManyErr.Error(),
		}
		return "", problemDetails
	}

	// nfProfile data for response
	nfProfiles, err := util.Decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.ManagementLog.Warnln("Time decode error: ", err)
		problemDetails = &models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "NOTIFICATION_ERROR",
			Detail: err.Error(),
		}
		return "", problemDetails
	}

	// NF Down Notification to other instances of same NfType
	if len(nfProfiles) != 0 {
		sendNFDownNotification(nfProfiles[0], nfInstanceID)
		uriList := nrf_context.GetNofificationUri(nfProfiles[0])
		nfInstanceUri := nrf_context.GetNfInstanceURI(nfInstanceID)
		// set info for NotificationData
		Notification_event := models.NotificationEventType_DEREGISTERED
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
		problemDetails = &models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SUBSCRIPTION_DELETE_ERROR",
			Detail: deleteErr.Error(),
		}
		return "", problemDetails
	}

	return nfType, nil
}

func sendNFDownNotification(nfProfile models.NfProfile, nfInstanceID string) {
	if nfProfile.NfType == models.NfType_AMF {
		url := "http://amf:29518" + "/namf-oam/v1/amfInstanceDown/" + nfInstanceID
		req, err := http.NewRequest(http.MethodPost, url, nil)
		if err != nil {
			logger.ManagementLog.Infoln("Error in creating request ", err)
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		_, err = client.Do(req)
		if err != nil {
			logger.ManagementLog.Infoln("Errored when sending request to the server", err)
		}
	}
}

func updateNFInstanceProcedure(nfInstanceID string, patchJSON []byte) (response map[string]interface{}, err error) {
	// Validation for NF Instance ID
	if nfInstanceID == "" {
		logger.ManagementLog.Errorln("nf Instance ID is required")
		return nil, fmt.Errorf("NF Instance ID is required")
	}
	collName := "NfProfile"
	filter := bson.M{"nfInstanceId": nfInstanceID}

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
	return nf, nil
}

func GetNFInstanceProcedure(nfInstanceID string) (response map[string]interface{}) {
	collName := "NfProfile"
	filter := bson.M{"nfInstanceId": nfInstanceID}
	response, _ = dbadapter.DBClient.RestfulAPIGetOne(collName, filter)

	return response
}

func NFRegisterProcedure(nfProfile models.NfProfile) (header http.Header, response bson.M,
	problemDetails *models.ProblemDetails,
) {
	logger.ManagementLog.Debugln("[NRF] In NFRegisterProcedure")
	var nf models.NfProfile
	err := nrf_context.NnrfNFManagementDataModel(&nf, nfProfile)
	if err != nil {
		logger.ManagementLog.Errorln("NfProfile Validation failed.", err)
		str1 := fmt.Sprint(nfProfile.HeartBeatTimer)
		problemDetails = &models.ProblemDetails{
			Title:  nfProfile.NfInstanceId,
			Status: http.StatusBadRequest,
			Detail: str1,
		}
		return nil, nil, problemDetails
	}

	// make location header
	locationHeaderValue := nrf_context.SetLocationHeader(nfProfile)

	// Marshal nf to bson
	tmp, err := json.Marshal(nf)
	if err != nil {
		logger.ManagementLog.Errorln("Marshal error in NFRegisterProcedure: ", err)
	}
	putData := bson.M{}
	err = json.Unmarshal(tmp, &putData)
	if err != nil {
		logger.ManagementLog.Errorln("Unmarshal error in NFRegisterProcedure: ", err)
	}

	// set db info
	collName := "NfProfile"
	nfInstanceId := nf.NfInstanceId
	filter := bson.M{"nfInstanceId": nfInstanceId}

	// fallback to older approach
	if !factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		NFDeleteAll(string(nf.NfType))
	} else {
		timein := time.Now().Local().Add(time.Second * time.Duration(nf.HeartBeatTimer*3))
		putData["expireAt"] = timein
		nfs, _ := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		if len(nfs) == 0 {
			putData["createdAt"] = time.Now()
		}
	}

	// Update NF Profile case
	if ok, _ := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, putData); ok { // true insert
		logger.ManagementLog.Infoln("RestfulAPIPutOne True Insert")
		uriList := nrf_context.GetNofificationUri(nf)

		// set info for NotificationData
		Notification_event := models.NotificationEventType_PROFILE_CHANGED
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
		return header, putData, nil
	} else { // Create NF Profile case
		logger.ManagementLog.Infoln("Create NF Profile ", nfProfile.NfType)
		uriList := nrf_context.GetNofificationUri(nf)
		// set info for NotificationData
		Notification_event := models.NotificationEventType_REGISTERED
		nfInstanceUri := locationHeaderValue

		for _, uri := range uriList {
			problemDetails = SendNFStatusNotify(Notification_event, nfInstanceUri, uri)
			if problemDetails != nil {
				return nil, nil, problemDetails
			}
		}

		header = make(http.Header)
		header.Add("Location", locationHeaderValue)
		logger.ManagementLog.Infoln("Location header: ", locationHeaderValue)
		return header, putData, nil
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
	filter := bson.M{"nfInstanceId": nfInstanceID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return "UNKNOWN_NF"
	}
	if response["nfType"] != nil {
		return fmt.Sprint(response["nfType"])
	}
	return "UNKNOWN_NF"
}

func SendNFStatusNotify(Notification_event models.NotificationEventType, nfInstanceUri string,
	url string,
) *models.ProblemDetails {
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	// url = fmt.Sprintf("%s%s", url, "/notification")

	configuration.SetBasePathNoGroup(url)
	notifcationData := models.NotificationData{
		Event:         Notification_event,
		NfInstanceUri: nfInstanceUri,
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	res, err := client.NotificationApi.NotificationPost(context.TODO(), notifcationData)
	if err != nil {
		logger.ManagementLog.Infof("Notify fail: %v", err)
		problemDetails := &models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "NOTIFICATION_ERROR",
			Detail: err.Error(),
		}
		return problemDetails
	}
	if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ManagementLog.Errorf("NotificationApi response body cannot close: %+v", resCloseErr)
			}
		}()
		if status := res.StatusCode; status != http.StatusNoContent && status != http.StatusOK {
			logger.ManagementLog.Warnln("Error status in NotificationPost: ", status)
			problemDetails := &models.ProblemDetails{
				Status: int32(status),
				Cause:  "NOTIFICATION_ERROR",
			}
			return problemDetails
		}
	}
	return nil
}
