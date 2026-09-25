// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

// Status codes the NRF returns for each outcome of an NF management request,
// against the responses TS 29.510 V18.6.0 defines for those operations.
package producer_test

import (
	"encoding/json"
	"errors"
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
	// profilesDeleted is set by RestfulAPIDeleteMany. A profile deleted before
	// the upsert is no longer there for the upsert to match, so RestfulAPIPutOne
	// then reports false whatever putOneExisted says, as the datastore would.
	profilesDeleted bool
	// getOne is returned by RestfulAPIGetOne; nil means "no such document".
	getOne map[string]any
	// getOneErr makes RestfulAPIGetOne fail, standing for a datastore that
	// cannot be read as opposed to a document that is not there.
	getOneErr error
	// getMany is what RestfulAPIGetMany returns for the NF profile collection;
	// empty means "no such document". Other collections, the Subscriptions one
	// in particular, answer empty: a profile row read back as a subscription
	// would make the procedure under test notify an empty URI.
	getMany []map[string]any
}

func (db *statusCodeDB) RestfulAPIPutOne(string, bson.M, map[string]any) (bool, error) {
	return db.putOneExisted && !db.profilesDeleted, nil
}

func (db *statusCodeDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	return db.getOne, db.getOneErr
}

func (db *statusCodeDB) RestfulAPIGetMany(collName string, _ bson.M) ([]map[string]any, error) {
	if collName != "NfProfile" {
		return nil, nil
	}
	return db.getMany, nil
}

// RestfulAPIJSONPatch mirrors what the datastore does when the filter matches
// nothing: the patch is applied to a nil document and fails. Without that, a
// case about an absent subscription would never reach the code path that
// reported the failure as success.
func (db *statusCodeDB) RestfulAPIJSONPatch(string, bson.M, []byte) error {
	if db.getOne == nil {
		return errors.New("patch against a document that is not there")
	}
	return nil
}

func (db *statusCodeDB) RestfulAPIDeleteMany(string, bson.M) error {
	db.profilesDeleted = true
	return nil
}

func (db *statusCodeDB) RestfulAPIDeleteOne(string, bson.M) error { return nil }

// storedProfileRow is this case's NF profile as the datastore holds it, under
// the driver's lower-cased field names.
func storedProfileRow() map[string]any {
	return map[string]any{"nfinstanceid": testInstanceID, "nftype": "AUSF"}
}

// storedProfileDB is a datastore that does or does not already hold this
// case's NF profile, answering both the read and the upsert consistently.
func storedProfileDB(stored bool) *statusCodeDB {
	db := &statusCodeDB{putOneExisted: stored}
	if stored {
		db.getOne = storedProfileRow()
	}
	return db
}

// profileExpiryModes are the two ways a registration reaches its upsert. With
// expiry disabled, the default when the key is absent from the configuration,
// the NRF first deletes every profile of the registering NF type, so the upsert
// can no longer tell a replacement from a creation by itself. Anything derived
// from that distinction has to be tested in both.
var profileExpiryModes = []struct {
	name    string
	enabled bool
}{
	{"expiry enabled", true},
	{"expiry disabled", false},
}

// setProfileExpiry sets the profile-expiry configuration a case needs and
// restores it afterwards, so it cannot leak into cases that run later.
// factory.NrfConfig is process-global. NfKeepAliveTime is restored as well:
// registering with expiry disabled overwrites it with one day, so restoring the
// flag alone would leave later cases with that keep-alive.
func setProfileExpiry(t *testing.T, enabled bool) {
	t.Helper()
	previousExpiry := factory.NrfConfig.Configuration.NfProfileExpiryEnable
	previousKeepAlive := factory.NrfConfig.Configuration.NfKeepAliveTime
	factory.NrfConfig.Configuration.NfProfileExpiryEnable = enabled
	t.Cleanup(func() {
		factory.NrfConfig.Configuration.NfProfileExpiryEnable = previousExpiry
		factory.NrfConfig.Configuration.NfKeepAliveTime = previousKeepAlive
	})
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
	for _, mode := range profileExpiryModes {
		for _, tc := range tests {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				useDB(t, storedProfileDB(tc.alreadyExisted))
				setProfileExpiry(t, mode.enabled)

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
	for _, mode := range profileExpiryModes {
		for _, tc := range tests {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
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

				useDB(t, &capturingDB{*storedProfileDB(tc.alreadyExisted), subscriber.URL})
				setProfileExpiry(t, mode.enabled)

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
}

// TS 29.510 clause 5.2.2.4.2 step 2b: an nfInstanceID that is not registered is
// 404, not a successful 204.
func TestDeregisterReportsUnknownInstanceAsNotFound(t *testing.T) {
	tests := []struct {
		name       string
		stored     []map[string]any
		wantStatus int
	}{
		{"instance is not registered", nil, http.StatusNotFound},
		{
			"instance is registered",
			[]map[string]any{storedProfileRow()},
			http.StatusNoContent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useDB(t, &statusCodeDB{getMany: tc.stored})

			request := newRequest(nil)
			request.Params["nfInstanceID"] = testInstanceID
			response := producer.HandleNFDeregisterRequest(request)

			if response.Status != tc.wantStatus {
				t.Errorf("status = %d, want %d", response.Status, tc.wantStatus)
			}
		})
	}
}

// TS 29.510 clause 5.2.2.3.1B step 2b. This is the path the NFs heartbeat over,
// so the code returned for a profile the NRF has lost decides whether they
// recover: their fallback triggers on any problem, but 500 misdescribes it.
func TestPatchReportsUnknownInstanceAsNotFound(t *testing.T) {
	useDB(t, &statusCodeDB{getOne: nil})

	request := newRequest(nil)
	request.Params["nfInstanceID"] = testInstanceID
	request.Body = []byte(`[{"op":"replace","path":"/nfStatus","value":"REGISTERED"}]`)
	response := producer.HandleUpdateNFInstanceRequest(request)

	if response.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Status, http.StatusNotFound)
	}
}

func TestRemoveUnknownSubscriptionReportsNotFound(t *testing.T) {
	useDB(t, &statusCodeDB{getOne: nil})

	request := newRequest(nil)
	request.Params["subscriptionID"] = "no-such-subscription"
	response := producer.HandleRemoveSubscriptionRequest(request)

	if response.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Status, http.StatusNotFound)
	}
}

// TS 29.510 clause 5.2.2.5.6 step 2a defines 204 as the success answer to a
// subscription update, so 204 cannot also be the answer for a subscription the
// NRF does not hold; 404 is what the API defines for that operation. The patch
// fails inside the datastore in this case, which the handler reported as 204.
func TestUpdateUnknownSubscriptionReportsNotFound(t *testing.T) {
	useDB(t, &statusCodeDB{getOne: nil})

	request := newRequest([]byte(`[{"op":"replace","path":"/validityTime","value":"2026-01-01T00:00:00Z"}]`))
	request.Params["subscriptionID"] = "no-such-subscription"
	response := producer.HandleUpdateSubscriptionRequest(request)

	if response.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", response.Status, http.StatusNotFound)
	}
}

// A datastore that cannot be read is a fault on the NRF's side, not the client
// naming a resource that is not there, so neither subscription operation may
// report it as 404.
func TestSubscriptionReadFailureIsNotReportedAsNotFound(t *testing.T) {
	tests := []struct {
		name   string
		handle func(*httpwrapper.Request) *httpwrapper.Response
	}{
		{"removal", producer.HandleRemoveSubscriptionRequest},
		{"update", producer.HandleUpdateSubscriptionRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useDB(t, &statusCodeDB{getOneErr: errors.New("datastore unreachable")})

			request := newRequest([]byte(`[]`))
			request.Params["subscriptionID"] = "some-subscription"
			response := tc.handle(request)

			if response.Status != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", response.Status, http.StatusInternalServerError)
			}
		})
	}
}

// The failure a deregistration of an unknown instance is recorded under keeps
// its NF type label: the procedure looks the type up before deciding the
// instance is absent, and returning an empty string would record the failure
// under an empty label instead of nfTypeUnknown.
func TestDeregisterOfUnknownInstanceKeepsItsMetricLabel(t *testing.T) {
	useDB(t, &statusCodeDB{})

	nfType, problem := producer.NFDeregisterProcedure(testInstanceID)

	if problem == nil || problem.GetStatus() != http.StatusNotFound {
		t.Fatalf("problem = %+v, want a 404", problem)
	}
	if nfType != "UNKNOWN_NF" {
		t.Errorf("nfType = %q, want %q", nfType, "UNKNOWN_NF")
	}
}

// The profile-expiry cases leave the process-global configuration as they
// found it. Registering with expiry disabled rewrites NfKeepAliveTime, which
// setProfileExpiry has to undo as well as the flag.
func TestProfileExpiryModesRestoreTheConfiguration(t *testing.T) {
	// Values, not the struct: factory.NrfConfig.Configuration is a pointer, so
	// holding it would compare the configuration with itself.
	beforeExpiry := factory.NrfConfig.Configuration.NfProfileExpiryEnable
	beforeKeepAlive := factory.NrfConfig.Configuration.NfKeepAliveTime

	for _, mode := range profileExpiryModes {
		t.Run(mode.name, func(t *testing.T) {
			useDB(t, storedProfileDB(false))
			setProfileExpiry(t, mode.enabled)

			producer.HandleNFRegisterRequest(newRequest(testProfile(testInstanceID)))
		})
	}

	after := factory.NrfConfig.Configuration
	if after.NfProfileExpiryEnable != beforeExpiry || after.NfKeepAliveTime != beforeKeepAlive {
		t.Errorf("configuration after = {expiry %v, keepAlive %d}, want {expiry %v, keepAlive %d}",
			after.NfProfileExpiryEnable, after.NfKeepAliveTime, beforeExpiry, beforeKeepAlive)
	}
}

// failingReadDB is a datastore whose single-document reads fail, standing for
// a transient read error while writes still succeed.
type failingReadDB struct {
	statusCodeDB
}

func (db *failingReadDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	return nil, errors.New("transient read failure")
}

// With expiry disabled, the read made before the legacy cleanup only decides
// between 200 and 201. A failure of that read must not fail the registration:
// the profile is still written, and it is reported as created.
func TestRegistrationSurvivesAFailedPreCleanupRead(t *testing.T) {
	useDB(t, &failingReadDB{statusCodeDB{putOneExisted: false}})
	setProfileExpiry(t, false)

	response := producer.HandleNFRegisterRequest(newRequest(testProfile("33333333-3333-4333-8333-333333333333")))

	if response.Status != http.StatusCreated {
		t.Errorf("status = %d, want %d despite the failed read", response.Status, http.StatusCreated)
	}
	if problem, isProblem := response.Body.(*models.ProblemDetails); isProblem {
		t.Errorf("registration failed on a read that only chooses the status code: %+v", problem)
	}
}
