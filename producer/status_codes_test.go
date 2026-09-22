// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

// Status codes the NRF returns for each outcome of an NF management request,
// against the responses TS 29.510 V18.6.0 defines for those operations.
package producer_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/producer"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// testInstanceID is the NF instance these cases register.
const testInstanceID = "11111111-1111-4111-8111-111111111111"

// statusCodeDB is a datastore stub whose answers each test sets directly, so a
// case reads as the datastore state it is about.
type statusCodeDB struct {
	dbadapter.DBInterface

	// putOneExisted is what RestfulAPIPutOne reports: true when a profile for
	// this NF instance already existed, which is the create-versus-update
	// signal the status code is derived from.
	putOneExisted bool
	// getOne is returned by RestfulAPIGetOne; nil means "no such document".
	getOne map[string]any
	// getMany is returned by RestfulAPIGetMany; empty means "no such document".
	getMany []map[string]any
}

func (db *statusCodeDB) RestfulAPIPutOne(string, bson.M, map[string]any) (bool, error) {
	return db.putOneExisted, nil
}

func (db *statusCodeDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	return db.getOne, nil
}

func (db *statusCodeDB) RestfulAPIGetMany(string, bson.M) ([]map[string]any, error) {
	return db.getMany, nil
}

func (db *statusCodeDB) RestfulAPIDeleteMany(string, bson.M) error { return nil }

func (db *statusCodeDB) RestfulAPIDeleteOne(string, bson.M) error { return nil }

// enableProfileExpiry turns on the profile-expiry configuration a case needs and
// restores the previous value, so the flag cannot leak into cases that run
// after it. factory.NrfConfig is process-global.
func enableProfileExpiry(t *testing.T) {
	t.Helper()
	previous := factory.NrfConfig.Configuration.NfProfileExpiryEnable
	factory.NrfConfig.Configuration.NfProfileExpiryEnable = true
	t.Cleanup(func() { factory.NrfConfig.Configuration.NfProfileExpiryEnable = previous })
}

func useDB(t *testing.T, db dbadapter.DBInterface) {
	t.Helper()
	previous := dbadapter.DBClient
	dbadapter.DBClient = db
	t.Cleanup(func() { dbadapter.DBClient = previous })
}

// newRequest builds the producer-layer request directly: httpwrapper.NewRequest
// exists to adapt a live *http.Request and dereferences it, which a unit test
// has no reason to fabricate.
func newRequest(body any) *httpwrapper.Request {
	return &httpwrapper.Request{
		Params: map[string]string{},
		Query:  url.Values{},
		Header: http.Header{},
		Body:   body,
	}
}

func testProfile(instanceID string) models.NFProfile {
	profile := models.NewNFProfileWithDefaults()
	profile.SetNfInstanceId(instanceID)
	profile.SetNfType(models.NFTYPE_AUSF)
	profile.SetNfStatus(models.NFSTATUS_REGISTERED)
	// Supplying the PLMN list keeps the registration off the webconsole
	// fallback path, which is not what these tests are about.
	profile.SetPlmnList([]models.PlmnId{{Mcc: "208", Mnc: "93"}})
	return *profile
}

// TS 29.510 clause 5.2.2.2.2 step 2a (create) and clause 5.2.2.3.1A step 2a
// (complete replacement of an existing profile over the same PUT).
func TestRegisterDistinguishesCreateFromUpdate(t *testing.T) {
	tests := []struct {
		name           string
		alreadyExisted bool
		wantStatus     int
		wantLocation   bool
	}{
		{"profile did not exist", false, http.StatusCreated, true},
		{"profile already existed", true, http.StatusOK, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useDB(t, &statusCodeDB{putOneExisted: tc.alreadyExisted})
			enableProfileExpiry(t)

			response := producer.HandleNFRegisterRequest(newRequest(testProfile(testInstanceID)))

			if response.Status != tc.wantStatus {
				t.Errorf("status = %d, want %d", response.Status, tc.wantStatus)
			}
			_, hasLocation := response.Header["Location"]
			if hasLocation != tc.wantLocation {
				t.Errorf("Location header present = %v, want %v", hasLocation, tc.wantLocation)
			}
			if response.Body == nil {
				t.Error("expected the stored profile in the response body on both outcomes")
			}
		})
	}
}

// capturingDB points every NF-status subscription at the given callback URL.
type capturingDB struct {
	statusCodeDB
	callbackURL string
}

func (db *capturingDB) RestfulAPIGetMany(string, bson.M) ([]map[string]any, error) {
	return []map[string]any{{
		"subscriptionId":          "subscription-1",
		"nfStatusNotificationUri": db.callbackURL,
	}}, nil
}

// The status code and the notification event are two statements about the same
// thing, and the NRF used to make them disagree: subscribers were told the
// profile had changed while the registering NF was told it had been created.
func TestNotificationEventAgreesWithStatusCode(t *testing.T) {
	tests := []struct {
		name           string
		alreadyExisted bool
		wantStatus     int
		wantEvent      models.NotificationEventType
	}{
		{"create", false, http.StatusCreated, models.NOTIFICATIONEVENTTYPE_NF_REGISTERED},
		{"update", true, http.StatusOK, models.NOTIFICATIONEVENTTYPE_NF_PROFILE_CHANGED},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan models.NotificationData, 1)
			subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data models.NotificationData
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					t.Errorf("decoding notification: %v", err)
				}
				// Several subscription conditions can match the same profile, so
				// more than one notification may arrive. Never block the handler:
				// a stalled subscriber would hold the NRF's client until its
				// 10 s notify timeout and make this test take that long.
				select {
				case received <- data:
				default:
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer subscriber.Close()

			useDB(t, &capturingDB{statusCodeDB{putOneExisted: tc.alreadyExisted}, subscriber.URL})
			enableProfileExpiry(t)

			response := producer.HandleNFRegisterRequest(newRequest(testProfile(testInstanceID)))

			if response.Status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Status, tc.wantStatus)
			}
			select {
			case data := <-received:
				if data.Event != tc.wantEvent {
					t.Errorf("notification event = %v, want %v (status was %d)", data.Event, tc.wantEvent, response.Status)
				}
			default:
				t.Error("no notification was delivered to the subscriber")
			}
		})
	}
}
