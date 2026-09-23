// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/go-viper/mapstructure/v2"
	nrfContext "github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	stats "github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const nfStatusNotifyTimeout = 10 * time.Second

var nfStatusNotifyHTTPClient = &http.Client{Timeout: nfStatusNotifyTimeout}

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
		problemDetails := utils.ProblemDetailsContextNotFound("NF instance not found")
		return httpwrapper.NewResponse(http.StatusNotFound, nil, problemDetails)
	}
}

func HandleNFRegisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ManagementLog.Infoln("Handle NFRegisterRequest")
	nfProfile := request.Body.(models.NFProfile)

	outcome, header, response, problemDetails := NFRegisterProcedure(nfProfile)

	if response != nil {
		logger.ManagementLog.Debugln("register success")
		stats.IncrementNrfRegistrationsStats("register", string(nfProfile.NfType), "SUCCESS")
		// 201 only when the profile did not exist before; a complete
		// replacement of an existing one is 200 (TS 29.510 clauses 5.2.2.2.2
		// step 2a and 5.2.2.3.1A step 2a).
		status := http.StatusCreated
		if outcome == nfProfileReplaced {
			status = http.StatusOK
		}
		return httpwrapper.NewResponse(status, header, response)
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

	response, err := updateNFInstanceProcedure(nfInstanceID, patchJSON)
	if err != nil {
		logger.ManagementLog.Errorln("updateNFInstanceProcedure failed:", err)
		if errors.Is(err, errInvalidPatchedNfProfile) {
			problemDetails := utils.ProblemDetailsWithCause("NF profile validation failed", http.StatusBadRequest, err.Error(), utils.CauseInvalidRequest)
			return httpwrapper.NewResponse(http.StatusBadRequest, nil, problemDetails)
		}
		if errors.Is(err, errConcurrentNfInstanceUpdate) {
			// 409, not 500: the client's own request never failed server-side, it
			// just kept losing a race with other updates; a plain retry is the
			// expected remedy.
			problemDetails := utils.ProblemDetailsWithCause("Concurrent update conflict", http.StatusConflict, err.Error(), utils.CauseRequestRejected)
			return httpwrapper.NewResponse(http.StatusConflict, nil, problemDetails)
		}
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
		problemDetails := utils.ProblemDetails("Invalid Parameter", http.StatusBadRequest, err.Error())

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
	subscription.SetSubscriptionId(nrfContext.SetsubscriptionId())

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
	ok, err := dbadapter.DBClient.RestfulAPIPost(collSubscriptions, bson.M{fieldSubscriptionId: subscription.GetSubscriptionId()},
		putData)
	if err != nil {
		logger.ManagementLog.Errorln("Post error in CreateSubscriptionProcedure: ", err)
		return nil, utils.ProblemDetailsSystemFailure(err.Error())
	}
	if !ok { // subscription id not exist before
		return putData, nil
	} else {
		problemDetails = utils.ProblemDetailsWithCause("Create subscription error", http.StatusBadRequest, "", utils.CauseCreateSubscriptionError)
		return nil, problemDetails
	}
}

func UpdateSubscriptionProcedure(subscriptionID string, patchJSON []byte) (response map[string]interface{}) {
	collName := collSubscriptions
	filter := bson.M{fieldSubscriptionId: subscriptionID}

	err := dbadapter.DBClient.RestfulAPIJSONPatch(collName, filter, patchJSON)
	if err == nil {
		var getErr error
		response, getErr = dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		if getErr != nil {
			logger.ManagementLog.Warnln("Error fetching updated subscription: ", getErr)
		}
		return response
	} else {
		logger.ManagementLog.Warnln("Error UpdateSubscriptionProcedure: ", err)
		return nil
	}
}

func RemoveSubscriptionProcedure(subscriptionID string) {
	collName := collSubscriptions
	filter := bson.M{fieldSubscriptionId: subscriptionID}
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
	collName := collUriList
	filter := bson.M{fieldNfType: nfType}

	UL, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		logger.ManagementLog.Warnln("Error fetching urilist in GetNFInstancesProcedure: ", err)
		return nil, utils.ProblemDetailsSystemFailure(err.Error())
	}
	logger.ManagementLog.Infoln("UL: ", UL)
	originalUL := &nrfContext.UriList{}
	err = mapstructure.Decode(UL, originalUL)
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
	collName := collNfProfile
	filter := bson.M{fieldNfTypeLower: nfType}

	err := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Errorf("failed to delete NF profiles of type %s: %v", nfType, err)
		problemDetails = utils.ProblemDetails("NF Profiles Deletion Failed", http.StatusInternalServerError, err.Error())
		return problemDetails
	}
	profileCache.evictByNfType(nfType)

	logger.ManagementLog.Infof("successfully deleted NF profiles of type %s", nfType)
	return nil
}

func NFDeregisterProcedure(nfInstanceID string) (nfType string, problemDetails *models.ProblemDetails) {
	collName := collNfProfile
	filter := bson.M{fieldNfInstanceId: nfInstanceID}
	nfType = GetNfTypeByNfInstanceID(nfInstanceID)

	nfProfilesRaw, err := dbadapter.DBClient.RestfulAPIGetMany(collName, filter)
	if err != nil {
		logger.ManagementLog.Warnln("error fetching NF profiles:", err)
		problemDetails = utils.ProblemDetailsWithCause("Fetch error", http.StatusInternalServerError, err.Error(), utils.CauseFetchError)
		return "", problemDetails
	}

	time.Sleep(time.Duration(1) * time.Second)

	deleteManyErr := dbadapter.DBClient.RestfulAPIDeleteMany(collName, filter)
	if deleteManyErr != nil {
		logger.ManagementLog.Warnln("error in deleting NF profiles:", deleteManyErr)
		problemDetails = utils.ProblemDetailsWithCause("NF delete error", http.StatusInternalServerError, deleteManyErr.Error(), utils.CauseNfDeleteError)
		return "", problemDetails
	}
	profileCache.evict(nfInstanceID)

	// nfProfile data for response
	nfProfiles, err := util.Decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.ManagementLog.Warnln("Time decode error: ", err)
		problemDetails = utils.ProblemDetailsWithCause("Notification error", http.StatusInternalServerError, err.Error(), utils.CauseNotificationError)
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
	deleteErr := dbadapter.DBClient.RestfulAPIDeleteMany(collSubscriptions, filter)
	if deleteErr != nil {
		logger.ManagementLog.Warnln("error in deleting subscriptions:", deleteErr)
		problemDetails = utils.ProblemDetailsWithCause("Subscription delete error", http.StatusInternalServerError, deleteErr.Error(), utils.CauseSubscriptionDeleteError)
		return "", problemDetails
	}

	return nfType, nil
}

func sendNFDownNotification(nfProfile models.NFProfile, nfInstanceID string) {
	if nfProfile.GetNfType() == models.NFTYPE_AMF {
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

// errInvalidPatchedNfProfile marks an update rejected because the profile
// resulting from the JSON Patch fails ValidateAllowedNfDomains; the JSON
// Patch applied by updateNFInstanceProcedure bypasses
// NnrfNFManagementDataModel's registration-time validation.
var errInvalidPatchedNfProfile = errors.New("invalid NF profile after patch")

// errConcurrentNfInstanceUpdate marks an update rejected because the NF
// instance was modified concurrently between the snapshot updateNFInstanceProcedure
// patched and validated and the write that would have persisted it. This is
// reported to the client as a conflict rather than retried: patchJSON may
// contain array-index paths or remove/move operations that are not safe to
// blindly replay against a document that has since changed shape, so the
// client (which knows what the patch was meant to do) must re-GET and
// resubmit instead.
var errConcurrentNfInstanceUpdate = errors.New("NF instance was modified concurrently; retry the request")

// decodeNFProfile decodes a raw MongoDB NF profile document into
// models.NFProfile. The document's keys are the driver's default-lowercased
// BSON field names (e.g. "nfservices", "allowednfdomains"; see the fieldXxx
// constants in nf_discovery.go), not the model's JSON field names.
func decodeNFProfile(raw map[string]interface{}) (models.NFProfile, error) {
	nfProfiles, decodeErr := util.Decode([]map[string]interface{}{raw}, time.RFC3339)
	if decodeErr != nil {
		return models.NFProfile{}, fmt.Errorf("decoding error: %v", decodeErr)
	}
	if len(nfProfiles) == 0 {
		return models.NFProfile{}, fmt.Errorf("decoded NF profiles are empty")
	}
	return util.ConvertNFProfileDiscoveryToNFProfile(nfProfiles[0]), nil
}

// applyJSONPatchToNFProfile applies patchJSON to profile using the model's
// own JSON field names (e.g. "/nfServices/0/allowedNfDomains"), matching
// what a TS 29.510 client actually sends. This is deliberately not applied
// to the raw MongoDB document (see decodeNFProfile): a real patch path
// would not resolve against its lowercased BSON keys.
func applyJSONPatchToNFProfile(profile models.NFProfile, patchJSON []byte) (models.NFProfile, error) {
	// models.NFProfile has no json struct tags, so json.Marshal would serialize
	// fields under their literal Go names (e.g. "NfStatus") instead of the
	// camelCase TS 29.510 names (e.g. "nfStatus") a patch path targets; ToMap
	// produces the correctly-cased keys.
	profileMap, err := profile.ToMap()
	if err != nil {
		return models.NFProfile{}, fmt.Errorf("failed to convert NF profile to map: %w", err)
	}
	profileJSON, err := json.Marshal(profileMap)
	if err != nil {
		return models.NFProfile{}, fmt.Errorf("failed to marshal NF profile: %w", err)
	}
	patch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return models.NFProfile{}, fmt.Errorf("failed to decode JSON patch: %w", err)
	}
	patchedJSON, err := patch.Apply(profileJSON)
	if err != nil {
		return models.NFProfile{}, fmt.Errorf("failed to apply JSON patch: %w", err)
	}
	var patched models.NFProfile
	if err := json.Unmarshal(patchedJSON, &patched); err != nil {
		return models.NFProfile{}, fmt.Errorf("failed to unmarshal patched NF profile: %w", err)
	}
	return patched, nil
}

// nfProfileToBSONMap converts nf to the map[string]interface{} shape
// MongoDB documents use (the driver's default-lowercased BSON keys),
// mirroring the bson.Marshal/Unmarshal round trip NFRegisterProcedure uses
// to build putData.
func nfProfileToBSONMap(nf models.NFProfile) (map[string]interface{}, error) {
	bsonBytes, err := bson.Marshal(nf)
	if err != nil {
		return nil, fmt.Errorf("bson marshal error: %w", err)
	}
	data := map[string]interface{}{}
	if err := bson.Unmarshal(bsonBytes, &data); err != nil {
		return nil, fmt.Errorf("bson unmarshal error: %w", err)
	}
	return data, nil
}

func updateNFInstanceProcedure(nfInstanceID string, patchJSON []byte) (*models.NFProfile, error) {
	// Validation for NF Instance ID
	if nfInstanceID == "" {
		logger.ManagementLog.Errorln("nf Instance ID is required")
		return nil, fmt.Errorf("NF Instance ID is required")
	}
	collName := collNfProfile
	filter := bson.M{fieldNfInstanceId: nfInstanceID}

	// Snapshot the pre-patch document, apply patchJSON to a candidate
	// models.NFProfile in memory, and validate that candidate before ever
	// attempting to persist it. Only once it is known to be valid is it
	// written, conditioned on the stored document still equalling the
	// snapshot (RestfulAPIReplaceIfUnchanged), so a concurrent update landing
	// in between is never silently overwritten. A conflict is reported to the
	// client (errConcurrentNfInstanceUpdate) rather than retried here:
	// patchJSON's operations (array-index paths, remove, move, ...) are not
	// generally safe to blindly replay against a document that changed shape
	// since it was snapshotted.
	previousDoc, getPreviousErr := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if getPreviousErr != nil || previousDoc == nil {
		logger.ManagementLog.Errorln("failed to get NF instance:", getPreviousErr)
		return nil, fmt.Errorf("failed to get NF instance: %v", getPreviousErr)
	}

	previousProfile, decodeErr := decodeNFProfile(previousDoc)
	if decodeErr != nil {
		logger.ManagementLog.Errorln("failed to decode NF instance:", decodeErr)
		return nil, decodeErr
	}

	updatedProfile, patchErr := applyJSONPatchToNFProfile(previousProfile, patchJSON)
	if patchErr != nil {
		logger.ManagementLog.Errorln("patch error in UpdateNFInstanceProcedure:", patchErr)
		return nil, fmt.Errorf("patch error: %v", patchErr)
	}

	// nfInstanceId is the document's identity (filter, above) and its own key
	// in the fallback URI-list cache; a patch that changes or removes it would
	// persist the update under the old key while the profile itself claims a
	// different (or no) identity, making it unreachable by nfInstanceID and
	// potentially colliding with whatever identity it was changed to.
	if updatedProfile.GetNfInstanceId() != nfInstanceID {
		return nil, fmt.Errorf("%w: nfInstanceId cannot be changed by a patch", errInvalidPatchedNfProfile)
	}

	if validateErr := nrfContext.ValidateAllowedNfDomains(updatedProfile); validateErr != nil {
		logger.ManagementLog.Errorln("patched NF profile is invalid, rejecting without persisting:", validateErr)
		return nil, fmt.Errorf("%w: %v", errInvalidPatchedNfProfile, validateErr)
	}

	candidate, marshalErr := nfProfileToBSONMap(updatedProfile)
	if marshalErr != nil {
		logger.ManagementLog.Errorln("failed to marshal patched NF profile:", marshalErr)
		return nil, fmt.Errorf("failed to marshal patched NF profile: %v", marshalErr)
	}

	// candidate is built solely from models.NFProfile, so NRF-internal
	// metadata stored on the document but absent from that model (e.g.
	// createdAt, or expireAt while NfProfileExpiryEnable is off) has no
	// counterpart in it. Carry any such field over from previousDoc before
	// the full replace below, so it is not silently dropped; the expiry
	// policy below still refreshes/overrides expireAt when enabled.
	for key, value := range previousDoc {
		if _, exists := candidate[key]; !exists {
			candidate[key] = value
		}
	}

	// Currently we are using 3 times the heartbeat timer as the expiry
	// time interval. We should update it to be configurable : TBD
	if factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		timein := time.Now().Local().Add(time.Second * time.Duration(factory.NrfConfig.Configuration.NfKeepAliveTime*3))
		candidate[fieldExpireAt] = timein
	}

	replaced, replaceErr := dbadapter.DBClient.RestfulAPIReplaceIfUnchanged(collName, filter, previousDoc, candidate)
	if replaceErr != nil {
		logger.ManagementLog.Errorf("failed to persist patched NF instance [%s]: %v", nfInstanceID, replaceErr)
		return nil, fmt.Errorf("failed to persist patched NF instance: %v", replaceErr)
	}
	if !replaced {
		logger.ManagementLog.Warnf("NF instance [%s] was modified concurrently; rejecting instead of replaying the patch", nfInstanceID)
		return nil, errConcurrentNfInstanceUpdate
	}

	profileCache.evict(nfInstanceID)

	logger.ManagementLog.Infof("nf profile [%s] update success", updatedProfile.NfType)
	return &updatedProfile, nil
}

func GetNFInstanceProcedure(nfInstanceID string) *models.NFProfile {
	collName := collNfProfile
	filter := bson.M{fieldNfInstanceId: nfInstanceID}
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

// nfRegistrationOutcome distinguishes the two successful outcomes of a
// registration request. TS 29.510 gives them different status codes: a newly
// created profile is 201 Created (clause 5.2.2.2.2 step 2a) and a complete
// replacement of an existing one, over the same PUT, is 200 OK (clause
// 5.2.2.3.1A step 2a).
type nfRegistrationOutcome int

const (
	nfProfileCreated nfRegistrationOutcome = iota
	nfProfileReplaced
)

func NFRegisterProcedure(nfProfile models.NFProfile) (outcome nfRegistrationOutcome, header http.Header,
	response *models.NFProfile, problemDetails *models.ProblemDetails,
) {
	logger.ManagementLog.Debugln("[NRF] In NFRegisterProcedure")
	var nf models.NFProfile
	err := nrfContext.NnrfNFManagementDataModel(&nf, nfProfile)
	if err != nil {
		logger.ManagementLog.Errorln("NfProfile Validation failed", err)
		problemDetails = utils.ProblemDetailsWithCause("NF profile validation failed", http.StatusBadRequest, err.Error(), utils.CauseInvalidRequest)
		return nfProfileCreated, nil, nil, problemDetails
	}

	// make location header
	locationHeaderValue := nrfContext.SetLocationHeader(nfProfile)
	// Marshal nf to bson
	putData := bson.M{}
	bsonBytes, err := bson.Marshal(nf)
	if err != nil {
		logger.ManagementLog.Errorln("bson marshal error in NFRegisterProcedure:", err)
		problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
		return nfProfileCreated, nil, nil, problemDetails
	}
	err = bson.Unmarshal(bsonBytes, &putData)
	if err != nil {
		logger.ManagementLog.Errorln("bson unmarshal error in NFRegisterProcedure:", err)
		problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
		return nfProfileCreated, nil, nil, problemDetails
	}
	// set db info
	collName := collNfProfile
	nfInstanceId := nf.GetNfInstanceId()
	filter := bson.M{fieldNfInstanceId: nfInstanceId}
	// replacedBeforeCleanup records whether this instance was registered before
	// the legacy cleanup below ran. That cleanup deletes every profile of this
	// NF type, this instance's included, so by the time the upsert runs it can
	// no longer see the profile it is replacing.
	replacedBeforeCleanup := false
	// fallback to older approach
	if !factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		existing, getErr := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		if getErr != nil {
			logger.ManagementLog.Warnln("Error fetching existing NF profile: ", getErr)
			return nfProfileCreated, nil, nil, utils.ProblemDetailsSystemFailure(getErr.Error())
		}
		replacedBeforeCleanup = len(existing) > 0
		NFDeleteAll(string(nf.NfType))
	} else {
		timein := time.Now().Local().Add(time.Second * time.Duration(nf.GetHeartBeatTimer()*3))
		putData[fieldExpireAt] = timein
		nfs, getErr := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
		if getErr != nil {
			logger.ManagementLog.Warnln("Error fetching existing NF profile: ", getErr)
			return nfProfileCreated, nil, nil, utils.ProblemDetailsSystemFailure(getErr.Error())
		}
		if len(nfs) == 0 {
			putData["createdAt"] = time.Now()
		}
	}
	// Update NF Profile case
	return handleNFProfileUpdateOrCreate(nf, nfProfile, locationHeaderValue, collName, filter, putData, replacedBeforeCleanup)
}

func handleNFProfileUpdateOrCreate(
	nf models.NFProfile,
	nfProfile models.NFProfile,
	locationHeaderValue string,
	collName string,
	filter bson.M,
	putData bson.M,
	replacedBeforeCleanup bool,
) (nfRegistrationOutcome, http.Header, *models.NFProfile, *models.ProblemDetails) {
	// RestfulAPIPutOne upserts and reports MatchedCount > 0, i.e. whether a
	// profile for this NF instance already existed. With profile expiry enabled
	// that is the authoritative answer, taken from the write itself, where a
	// separate existence query would race with a concurrent registration of the
	// same instance. With expiry disabled it cannot be: the legacy cleanup has
	// already deleted the profile, so the upsert always inserts, and the answer
	// has to come from the read made before that delete. That read is not
	// atomic with the write: two concurrent registrations of the same instance
	// can both find it absent, and the second's cleanup deletes what the first
	// just wrote, so both answer 201. This mode answered 201 to every
	// re-registration before, so the window only narrows; closing it would
	// mean the cleanup sparing the registering instance's own profile.
	existed, err := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, putData)
	if err != nil {
		logger.ManagementLog.Errorln("RestfulAPIPutOne error:", err)
		return nfProfileCreated, nil, nil, utils.ProblemDetailsSystemFailure(err.Error())
	}
	existed = existed || replacedBeforeCleanup

	outcome := nfProfileCreated
	notificationEvent := models.NOTIFICATIONEVENTTYPE_NF_REGISTERED
	if existed {
		outcome = nfProfileReplaced
		notificationEvent = models.NOTIFICATIONEVENTTYPE_NF_PROFILE_CHANGED
		profileCache.evict(nf.GetNfInstanceId())
		logger.ManagementLog.Infoln("RestfulAPIPutOne update")
	} else {
		logger.ManagementLog.Infoln("create NF Profile", nfProfile.GetNfType())
	}

	for _, uri := range nrfContext.GetNotificationUri(nf) {
		if pd := SendNFStatusNotify(notificationEvent, locationHeaderValue, uri); pd != nil {
			return outcome, nil, nil, pd
		}
	}

	// TS 29.510 clause 5.2.2.2.2 step 2a specifies the Location header for the
	// resource NFRegister created. Clause 5.2.2.3.1A step 2a, the complete
	// replacement of an existing profile over the same PUT, specifies no such
	// header, the resource having already existed.
	var header http.Header
	if outcome == nfProfileCreated {
		header = make(http.Header)
		header.Add("Location", locationHeaderValue)
		logger.ManagementLog.Infoln("location header:", locationHeaderValue)
	}
	return outcome, header, &nf, nil
}

func GetNfTypeBySubscriptionID(subscriptionID string) (nfType string) {
	collName := collSubscriptions
	filter := bson.M{fieldSubscriptionId: subscriptionID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return nfTypeUnknown
	}
	if response["reqNfType"] != nil {
		return fmt.Sprint(response["reqNfType"])
	}
	return nfTypeUnknown
}

func GetNfTypeByNfInstanceID(nfInstanceID string) (nfType string) {
	collName := collNfProfile
	filter := bson.M{fieldNfInstanceId: nfInstanceID}
	response, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		return nfTypeUnknown
	}
	if response[fieldNfTypeLower] != nil {
		return fmt.Sprint(response[fieldNfTypeLower])
	}
	return nfTypeUnknown
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
		problemDetails := utils.ProblemDetailsWithCause("Notification error", http.StatusInternalServerError, err.Error(), utils.CauseNotificationError)
		return problemDetails
	}

	notifyCtx, cancel := context.WithTimeout(context.Background(), nfStatusNotifyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(notifyCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logger.ManagementLog.Infof("notify fail: %+v", err)
		problemDetails := utils.ProblemDetailsWithCause("Notification error", http.StatusInternalServerError, err.Error(), utils.CauseNotificationError)
		return problemDetails
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, application/problem+json")

	res, err := nfStatusNotifyHTTPClient.Do(req)
	if err != nil {
		logger.ManagementLog.Infof("notify fail: %+v", err)
		problemDetails := utils.ProblemDetailsWithCause("Notification error", http.StatusInternalServerError, err.Error(), utils.CauseNotificationError)
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
			responseBody, readErr := io.ReadAll(res.Body)
			if readErr == nil && len(responseBody) > 0 {
				var remoteProblem models.ProblemDetails
				if decodeErr := json.Unmarshal(responseBody, &remoteProblem); decodeErr == nil {
					return &remoteProblem
				}
				problemDetails := utils.ProblemDetailsWithCause("Notification error", status, string(responseBody), utils.CauseNotificationError)
				return problemDetails
			}
			problemDetails := utils.ProblemDetailsWithCause("Notification error", status, "", utils.CauseNotificationError)
			return problemDetails
		}
	}
	return nil
}
