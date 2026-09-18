// SPDX-FileCopyrightText: 2025 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"regexp/syntax"
	"strconv"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/polling"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	fieldNfType     = "nfType"
	fieldSubscrCond = "subscrCond"
	fieldNfGroupId  = "nfGroupId"
	mongoOpOr       = "$or"
)

func NnrfNFManagementDataModel(nf *models.NFProfile, nfprofile models.NFProfile) error {
	if nfprofile.GetNfInstanceId() == "" {
		return fmt.Errorf("NfInstanceId field is required")
	}
	nf.SetNfInstanceId(nfprofile.GetNfInstanceId())

	if nfprofile.GetNfType() == "" {
		return fmt.Errorf("NfType field is required")
	}
	nf.SetNfType(nfprofile.GetNfType())

	if nfprofile.GetNfStatus() == "" {
		return fmt.Errorf("NfStatus field is required")
	}
	nf.SetNfStatus(nfprofile.GetNfStatus())

	if err := ValidateAllowedNfDomains(nfprofile); err != nil {
		return err
	}

	plmnList, hasPlmnList := nfprofile.GetPlmnListOk()
	nfPlmnList, err := buildNfProfilePlmnList(plmnList, hasPlmnList)
	if err != nil {
		return err
	}

	nnrfNFManagementCondition(nf, nfprofile)
	nf.SetPlmnList(nfPlmnList)
	nnrfNFManagementOption(nf, nfprofile)

	return nil
}

// ValidateAllowedNfDomains rejects registration or update of an NF profile
// whose nfServices or nfServiceList (TS 29.510 clause 6.1.6.2.2) contains an
// allowedNfDomains pattern that fails to compile as an RE2 regular
// expression (regexp.Compile), the engine used by the in-memory
// requesterFqdn check (see matchesAllowedNfDomainPattern in the producer
// package). TS 29.510 specifies allowedNfDomains as an ECMA-262 pattern, but
// this NRF deliberately enforces the narrower subset of ECMA-262 supported
// by RE2 (no backreferences or lookaround) so that a pattern behaves
// identically whether matched in memory (RE2) or, once persisted, via the
// MongoDB-backed discovery query's $regexMatch. Callers must invoke this for
// both new registrations and patched updates, since rejecting invalid or
// unsupported patterns before they are stored, rather than after, prevents a
// single malformed entry from later causing $regexMatch to fail the whole
// discovery request.
func ValidateAllowedNfDomains(nfprofile models.NFProfile) error {
	if nfServices, ok := nfprofile.GetNfServicesOk(); ok {
		for _, service := range nfServices {
			if err := validateServiceAllowedNfDomains(service); err != nil {
				return err
			}
		}
	}
	if nfServiceList, ok := nfprofile.GetNfServiceListOk(); ok {
		for _, service := range *nfServiceList {
			if err := validateServiceAllowedNfDomains(service); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateServiceAllowedNfDomains(service models.NFService) error {
	allowedDomains, ok := service.GetAllowedNfDomainsOk()
	if !ok {
		return nil
	}
	for _, pattern := range allowedDomains {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("allowedNfDomains pattern %q for service %q is not a valid RE2 regular expression "+
				"(this NRF requires the subset of ECMA-262 syntax supported by RE2; avoid backreferences and "+
				"lookaround): %w", pattern, service.ServiceName, err)
		}
		if err := rejectReDoSRiskPattern(pattern); err != nil {
			return fmt.Errorf("allowedNfDomains pattern %q for service %q: %w", pattern, service.ServiceName, err)
		}
	}
	return nil
}

// maxAllowedNfDomainsPatternLength bounds how expensive a single stored
// allowedNfDomains pattern can be to compile/evaluate.
const maxAllowedNfDomainsPatternLength = 256

// maxAllowedNfDomainsRepeatCount bounds how many times any single
// quantifier in an allowedNfDomains pattern may repeat. Pattern length alone
// is not a reliable proxy for evaluation cost: a short, bounded quantifier
// such as "a{100000}" forces a search space far larger than
// maxAllowedNfDomainsPatternLength would suggest.
const maxAllowedNfDomainsRepeatCount = 1000

// maxAllowedNfDomainsAlternationCount bounds how many alternation groups
// (e.g. "(a|aa)") an allowedNfDomains pattern may contain in total, regardless
// of nesting. This is what closes the gap hasUnsafeRepeat otherwise leaves
// open: hasUnsafeRepeat only rejects an alternation repeated by an explicit
// quantifier (e.g. "(a|aa)+"), but concatenating the same ambiguous
// alternation several times by hand (e.g. "(a|aa)(a|aa)(a|aa)") gives a
// backtracking engine exponentially many ways to partition a non-matching
// input just as surely, without ever using a quantifier a per-quantifier
// bound could catch. Rather than attempt to distinguish an ambiguous
// alternation (branches sharing a prefix) from a safe one, every pattern is
// limited to at most one alternation group in total.
const maxAllowedNfDomainsAlternationCount = 1

// maxAllowedNfDomainsNullableQuantifierCount bounds how many nullable
// quantifiers (?, *, or {0,m}: constructs that can match the empty string) an
// allowedNfDomains pattern may contain in total, regardless of nesting. This
// closes the same kind of gap for optional quantifiers that
// maxAllowedNfDomainsAlternationCount closes for alternation: hasUnsafeRepeat
// only rejects a nullable quantifier nested inside an outer repeat (e.g.
// "(a?a?)+"), but concatenating several nullable quantifiers by hand at the
// top level (e.g. "a?a?a?a?a?a?a?a?a?a?b") gives a backtracking engine
// exponentially many ways to assign each one empty or not on a non-matching
// input, without ever nesting one inside a repeat.
const maxAllowedNfDomainsNullableQuantifierCount = 1

// rejectReDoSRiskPattern rejects allowedNfDomains patterns that are prone to
// catastrophic (exponential-time) backtracking. RE2 (regexp.Compile) is
// immune to this by construction, but once persisted the same pattern is also
// evaluated by MongoDB's PCRE-based $regexMatch (see allowedNfDomainsMatchCond
// in the producer package), which does backtrack. An NF that can register or
// patch its own profile could otherwise plant a pattern such as "(a+)+",
// "(a|aa){1000}", "(a|aa)(a|aa)(a|aa)", or "a?a?a?a?a?a?a?a?a?a?b" - valid
// RE2, but exponential (or merely very slow) under PCRE for a crafted,
// non-matching input - and use a discovery query to burn CPU on the shared
// MongoDB instance. To stay on the safe side of that risk, this enforces a
// restricted subset rather than trying to precisely detect every ambiguous
// pattern: a quantifier that can match more than once (*, +, {n,}, or
// {n,m}/{n} with a max greater than one) may not itself repeat a
// subexpression that contains another such quantifier or an alternation; no
// quantifier's bound may exceed maxAllowedNfDomainsRepeatCount, regardless of
// nesting; the pattern may contain at most maxAllowedNfDomainsAlternationCount
// alternation groups in total, so several ambiguous alternations cannot be
// chained by concatenation instead of repetition; and likewise at most
// maxAllowedNfDomainsNullableQuantifierCount nullable quantifiers (?, *, or
// {0,m}) in total, so several cannot be chained by concatenation either.
// allowedNfDomains patterns should stay simple (anchors, character classes, a
// single level of quantifiers, and at most one alternation group or nullable
// quantifier).
func rejectReDoSRiskPattern(pattern string) error {
	if len(pattern) > maxAllowedNfDomainsPatternLength {
		return fmt.Errorf("pattern exceeds maximum length of %d characters", maxAllowedNfDomainsPatternLength)
	}
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		// regexp.Compile already accepted this pattern, so a parse error here is
		// unexpected; fail closed rather than skip the complexity check.
		return fmt.Errorf("failed to analyze pattern: %w", err)
	}
	if hasExcessiveRepeatCount(parsed) {
		return fmt.Errorf("pattern has a quantifier bound greater than %d, which can be expensive to evaluate "+
			"even without ambiguous nesting", maxAllowedNfDomainsRepeatCount)
	}
	if hasUnsafeRepeat(parsed) {
		return fmt.Errorf("pattern has a quantifier repeating a repetition or alternation " +
			"(e.g. \"(a+)+\" or \"(a|aa){1000}\"), which can cause catastrophic backtracking when evaluated " +
			"by MongoDB's PCRE-based $regexMatch")
	}
	if countAlternations(parsed) > maxAllowedNfDomainsAlternationCount {
		return fmt.Errorf("pattern has more than %d alternation group(s); chaining several ambiguous alternations "+
			"by concatenation (e.g. \"(a|aa)(a|aa)(a|aa)\") can cause catastrophic backtracking when evaluated "+
			"by MongoDB's PCRE-based $regexMatch, just as repeating one with a quantifier can",
			maxAllowedNfDomainsAlternationCount)
	}
	if countNullableQuantifiers(parsed) > maxAllowedNfDomainsNullableQuantifierCount {
		return fmt.Errorf("pattern has more than %d nullable quantifier(s) (?, *, or {0,m}); chaining several "+
			"by concatenation (e.g. \"a?a?a?a?a?a?a?a?a?a?b\") can cause catastrophic backtracking when evaluated "+
			"by MongoDB's PCRE-based $regexMatch",
			maxAllowedNfDomainsNullableQuantifierCount)
	}
	return nil
}

// countAlternations reports the total number of alternation (OpAlternate)
// nodes in re, at any depth. Used to bound the number of alternation groups
// an allowedNfDomains pattern may contain in total (see
// maxAllowedNfDomainsAlternationCount), since concatenating several ambiguous
// alternations is exponential for the same reason repeating one is.
func countAlternations(re *syntax.Regexp) int {
	count := 0
	if re.Op == syntax.OpAlternate {
		count++
	}
	for _, sub := range re.Sub {
		count += countAlternations(sub)
	}
	return count
}

// countNullableQuantifiers reports the total number of nullable quantifier
// (OpQuest, OpStar, or OpRepeat with Min == 0) nodes in re, at any depth.
// Used to bound the number of nullable quantifiers an allowedNfDomains
// pattern may contain in total (see
// maxAllowedNfDomainsNullableQuantifierCount), since concatenating several is
// exponential for the same reason nesting one inside a repeat is.
func countNullableQuantifiers(re *syntax.Regexp) int {
	count := 0
	switch re.Op {
	case syntax.OpQuest, syntax.OpStar:
		count++
	case syntax.OpRepeat:
		if re.Min == 0 {
			count++
		}
	}
	for _, sub := range re.Sub {
		count += countNullableQuantifiers(sub)
	}
	return count
}

// hasExcessiveRepeatCount reports whether re, or anything under it, is a
// quantifier whose bound exceeds maxAllowedNfDomainsRepeatCount.
func hasExcessiveRepeatCount(re *syntax.Regexp) bool {
	if re.Op == syntax.OpRepeat && (re.Min > maxAllowedNfDomainsRepeatCount || re.Max > maxAllowedNfDomainsRepeatCount) {
		return true
	}
	for _, sub := range re.Sub {
		if hasExcessiveRepeatCount(sub) {
			return true
		}
	}
	return false
}

// hasUnsafeRepeat reports whether re contains a quantifier that can match
// more than once whose repeated subexpression itself contains another such
// quantifier or an alternation.
func hasUnsafeRepeat(re *syntax.Regexp) bool {
	if isRepeatable(re) && containsRepetitionOrAlternation(re.Sub[0]) {
		return true
	}
	for _, sub := range re.Sub {
		if hasUnsafeRepeat(sub) {
			return true
		}
	}
	return false
}

// containsRepetitionOrAlternation reports whether re, or anything under it, is
// a repeatable quantifier, an optional quantifier (?), or an alternation:
// constructs that, when themselves repeated, are the classic causes of
// catastrophic backtracking (see hasUnsafeRepeat). Although a bare "?"
// cannot itself compound ambiguity by repeating (see isRepeatable), nesting
// one inside an outer repeat (e.g. "(a?a?)+") still gives a backtracking
// engine exponentially many ways to partition a non-matching input, so it
// must count as an ambiguity source here even though isRepeatable excludes it.
func containsRepetitionOrAlternation(re *syntax.Regexp) bool {
	if isRepeatable(re) || re.Op == syntax.OpQuest || re.Op == syntax.OpAlternate {
		return true
	}
	for _, sub := range re.Sub {
		if containsRepetitionOrAlternation(sub) {
			return true
		}
	}
	return false
}

// isRepeatable reports whether re can match its subexpression more than
// once: unbounded (*, +, {n,}) or bounded with a max greater than one. A
// bounded quantifier with a max of exactly one degenerate case (e.g. {0,1})
// cannot itself compound ambiguity through repetition.
func isRepeatable(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpStar, syntax.OpPlus:
		return true
	case syntax.OpRepeat:
		return re.Max == -1 || re.Max > 1
	default:
		return false
	}
}

func buildNfProfilePlmnList(nfProvidedPlmnList []models.PlmnId, hasProvidedPlmnList bool) ([]models.PlmnId, error) {
	// NF provided a list of supported PLMNs
	if hasProvidedPlmnList && len(nfProvidedPlmnList) != 0 {
		return nfProvidedPlmnList, nil
	}
	// NF did not provide supported PLMNs: fetch from webconsole
	logger.ManagementLog.Warnln("PLMN config not provided by NF, using supported PLMNs from webconsole")
	supportedPlmnList, err := polling.FetchPlmnConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PLMN config from webconsole: %v", err)
	}
	logger.ManagementLog.Debugf("Fetched PLMN list from webconsole: %+v", supportedPlmnList)
	if len(supportedPlmnList) == 0 {
		return nil, fmt.Errorf("PLMN config not provided by NF and no local PLMN config available")
	}
	return supportedPlmnList, nil
}

func SetsubscriptionId() string {
	id, err := uuid.NewRandom()
	if err != nil {
		logger.ManagementLog.Errorf("failed to generate UUID for subscription ID: %v", err)
		// Fallback to a time-based ID to avoid panicking and keep subscription creation non-fatal
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return id.String()
}

func nnrfNFManagementCondition(nf *models.NFProfile, nfprofile models.NFProfile) {
	// HeartBeatTimer
	if !factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		// setting 1day keepAliveTimer value
		factory.NrfConfig.Configuration.NfKeepAliveTime = 24 * 60 * 60
	} else if factory.NrfConfig.Configuration.NfKeepAliveTime == 0 {
		logger.ManagementLog.Infoln("NfProfileExpiryEnable: true but keepAliveTime: 0, setting default keepAliveTimer: 60 sec")
		factory.NrfConfig.Configuration.NfKeepAliveTime = 60
	}
	nf.SetHeartBeatTimer(factory.NrfConfig.Configuration.NfKeepAliveTime)
	logger.ManagementLog.Infof("heartbeat timer value: %d sec", nf.GetHeartBeatTimer())

	// fqdn
	if fqdn, ok := nfprofile.GetFqdnOk(); ok {
		nf.SetFqdn(*fqdn)
	}
	// interPlmnFqdn
	interPlmnFqdn, ok := nfprofile.GetInterPlmnFqdnOk()
	if ok {
		nf.SetInterPlmnFqdn(*interPlmnFqdn)
	}
	// ipv4Addresses
	if ipv4Addresses, ok := nfprofile.GetIpv4AddressesOk(); ok {
		a := make([]string, len(ipv4Addresses))
		copy(a, ipv4Addresses)
		nf.SetIpv4Addresses(a)
	}
	// ipv6Addresses
	if ipv6Addresses, ok := nfprofile.GetIpv6AddressesOk(); ok {
		a := make([]string, len(ipv6Addresses))
		copy(a, ipv6Addresses)
		nf.SetIpv6Addresses(a)
	}
}

func nnrfNFManagementOption(nf *models.NFProfile, nfprofile models.NFProfile) {
	copyBasicLists(nf, nfprofile)
	copyPriorityCapacityLoad(nf, nfprofile)
	copyLocality(nf, nfprofile)

	copyUdrInfo(nf, nfprofile)
	copyUdmInfo(nf, nfprofile)
	copyAusfInfo(nf, nfprofile)
	copyAmfInfo(nf, nfprofile)
	copySmfInfo(nf, nfprofile)
	copyUpfInfo(nf, nfprofile)
	copyPcfInfo(nf, nfprofile)
	copyBsfInfo(nf, nfprofile)
	copyChfInfo(nf, nfprofile)

	copyNrfInfo(nf, nfprofile)
	copyRecoveryTime(nf, nfprofile)
	copyNfServicePersistence(nf, nfprofile)
	copyNfServices(nf, nfprofile)
}

func copyBasicLists(nf *models.NFProfile, nfprofile models.NFProfile) {
	// sNssais
	if sNssais, ok := nfprofile.GetSNssaisOk(); ok {
		a := make([]models.Snssai, len(sNssais))
		copy(a, sNssais)
		nf.SetSNssais(a)
	}

	// nsiList
	if nsiList, ok := nfprofile.GetNsiListOk(); ok {
		a := make([]string, len(nsiList))
		copy(a, nsiList)
		nf.SetNsiList(a)
	}

	// allowedPlmns
	if allowedPlmns, ok := nfprofile.GetAllowedPlmnsOk(); ok {
		a := make([]models.PlmnId, len(allowedPlmns))
		copy(a, allowedPlmns)
		nf.SetAllowedPlmns(a)
	}

	// allowedNfTypes
	if allowedNfTypes, ok := nfprofile.GetAllowedNfTypesOk(); ok {
		a := make([]models.NFType, len(allowedNfTypes))
		copy(a, allowedNfTypes)
		nf.SetAllowedNfTypes(a)
	}
	// allowedNfDomains
	if allowedNfDomains, ok := nfprofile.GetAllowedNfDomainsOk(); ok {
		a := make([]string, len(allowedNfDomains))
		copy(a, allowedNfDomains)
		nf.SetAllowedNfDomains(a)
	}

	// allowedNssais
	if allowedNssais, ok := nfprofile.GetAllowedNssaisOk(); ok {
		a := make([]models.Snssai, len(allowedNssais))
		copy(a, allowedNssais)
		nf.SetAllowedNssais(a)
	}
}

func copyPriorityCapacityLoad(nf *models.NFProfile, nfprofile models.NFProfile) {
	// Priority: range [0, 65535] per TS 29.510 clause 6.1.6.2.2
	// Use GetPriorityOk so that an explicitly set priority of 0 (highest
	// priority) is not silently dropped.
	if priority, ok := nfprofile.GetPriorityOk(); ok && *priority >= 0 && *priority <= 65535 {
		nf.SetPriority(*priority)
	}
	// Capacity: range [0, 65535] per TS 29.510 clause 6.1.6.2.2
	if capacity, ok := nfprofile.GetCapacityOk(); ok && *capacity >= 0 && *capacity <= 65535 {
		nf.SetCapacity(*capacity)
	}
	// Load: range [0, 100] per TS 29.510 clause 6.1.6.2.2
	if load, ok := nfprofile.GetLoadOk(); ok && *load >= 0 && *load <= 100 {
		nf.SetLoad(*load)
	}
}

func copyLocality(nf *models.NFProfile, nfprofile models.NFProfile) {
	// Locality
	if nfprofile.GetLocality() != "" {
		nf.SetLocality(nfprofile.GetLocality())
	}
}

func copyUdrInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// udrInfo
	if nfprofile.UdrInfo != nil {
		a := models.NewUdrInfo()

		if groupId, ok := nfprofile.UdrInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.UdrInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if gpsiRanges, ok := nfprofile.UdrInfo.GetGpsiRangesOk(); ok {
			a.SetGpsiRanges(gpsiRanges)
		}

		if externalGroupIdentifiersRanges, ok := nfprofile.UdrInfo.GetExternalGroupIdentifiersRangesOk(); ok {
			a.SetExternalGroupIdentifiersRanges(externalGroupIdentifiersRanges)
		}

		if supportedDataSets, ok := nfprofile.UdrInfo.GetSupportedDataSetsOk(); ok {
			a.SetSupportedDataSets(supportedDataSets)
		}

		nf.SetUdrInfo(*a)
	}
}

func copyUdmInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// udmInfo
	if nfprofile.UdmInfo != nil {
		a := models.NewUdmInfo()

		if groupId, ok := nfprofile.UdmInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.UdmInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if gpsiRanges, ok := nfprofile.UdmInfo.GetGpsiRangesOk(); ok {
			a.SetGpsiRanges(gpsiRanges)
		}

		if externalGroupIdentifiersRanges, ok := nfprofile.UdmInfo.GetExternalGroupIdentifiersRangesOk(); ok {
			a.SetExternalGroupIdentifiersRanges(externalGroupIdentifiersRanges)
		}

		if routingIndicators, ok := nfprofile.UdmInfo.GetRoutingIndicatorsOk(); ok {
			a.SetRoutingIndicators(routingIndicators)
		}

		nf.SetUdmInfo(*a)
	}
}

func copyAusfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// ausfInfo
	if nfprofile.AusfInfo != nil {
		a := models.NewAusfInfo()

		if groupId, ok := nfprofile.AusfInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.AusfInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if routingIndicators, ok := nfprofile.AusfInfo.GetRoutingIndicatorsOk(); ok {
			a.SetRoutingIndicators(routingIndicators)
		}

		nf.SetAusfInfo(*a)
	}
}

func copyAmfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// amfInfo
	if nfprofile.AmfInfo != nil {
		a := models.NewAmfInfoWithDefaults()

		if amfSetId, ok := nfprofile.AmfInfo.GetAmfSetIdOk(); ok {
			a.SetAmfSetId(*amfSetId)
		}

		if amfRegionId, ok := nfprofile.AmfInfo.GetAmfRegionIdOk(); ok {
			a.SetAmfRegionId(*amfRegionId)
		}

		if guamiList, ok := nfprofile.AmfInfo.GetGuamiListOk(); ok {
			a.SetGuamiList(guamiList)
		}

		if taiList, ok := nfprofile.AmfInfo.GetTaiListOk(); ok {
			a.SetTaiList(taiList)
		}

		if taiRangeList, ok := nfprofile.AmfInfo.GetTaiRangeListOk(); ok {
			a.SetTaiRangeList(taiRangeList)
		}

		if backupInfoAmfFailure, ok := nfprofile.AmfInfo.GetBackupInfoAmfFailureOk(); ok {
			a.SetBackupInfoAmfFailure(backupInfoAmfFailure)
		}

		if backupInfoAmfRemoval, ok := nfprofile.AmfInfo.GetBackupInfoAmfRemovalOk(); ok {
			a.SetBackupInfoAmfRemoval(backupInfoAmfRemoval)
		}

		if nfprofile.AmfInfo.N2InterfaceAmfInfo.IsSet() {
			a.N2InterfaceAmfInfo = nfprofile.AmfInfo.N2InterfaceAmfInfo
		}
		nf.SetAmfInfo(*a)
	}
}

func copySmfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// smfInfo
	if nfprofile.SmfInfo != nil {
		a := models.NewSmfInfoWithDefaults()

		if sNssaiSmfInfoList, ok := nfprofile.SmfInfo.GetSNssaiSmfInfoListOk(); ok {
			a.SetSNssaiSmfInfoList(sNssaiSmfInfoList)
		}

		if taiList, ok := nfprofile.SmfInfo.GetTaiListOk(); ok {
			a.SetTaiList(taiList)
		}

		if taiRangeList, ok := nfprofile.SmfInfo.GetTaiRangeListOk(); ok {
			a.SetTaiRangeList(taiRangeList)
		}

		if pgwFqdn, ok := nfprofile.SmfInfo.GetPgwFqdnOk(); ok {
			a.SetPgwFqdn(*pgwFqdn)
		}

		if accessType, ok := nfprofile.SmfInfo.GetAccessTypeOk(); ok {
			a.SetAccessType(accessType)
		}
		nf.SetSmfInfo(*a)
	}
}

func copyUpfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// upfInfo
	if nfprofile.UpfInfo != nil {
		a := models.NewUpfInfoWithDefaults()

		if sNssaiUpfInfoList, ok := nfprofile.UpfInfo.GetSNssaiUpfInfoListOk(); ok {
			a.SetSNssaiUpfInfoList(sNssaiUpfInfoList)
		}

		if smfServingArea, ok := nfprofile.UpfInfo.GetSmfServingAreaOk(); ok {
			a.SetSmfServingArea(smfServingArea)
		}

		if interfaceUpfInfoList, ok := nfprofile.UpfInfo.GetInterfaceUpfInfoListOk(); ok {
			a.SetInterfaceUpfInfoList(interfaceUpfInfoList)
		}

		a.SetIwkEpsInd(nfprofile.UpfInfo.GetIwkEpsInd())

		nf.SetUpfInfo(*a)
	}
}

func copyPcfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// pcfInfo
	if nfprofile.PcfInfo != nil {
		a := models.NewPcfInfo()

		if dnnList, ok := nfprofile.PcfInfo.GetDnnListOk(); ok {
			a.SetDnnList(dnnList)
		}

		if supiRanges, ok := nfprofile.PcfInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if rxDiamHost, ok := nfprofile.PcfInfo.GetRxDiamHostOk(); ok {
			a.SetRxDiamHost(*rxDiamHost)
		}

		if rxDiamRealm, ok := nfprofile.PcfInfo.GetRxDiamRealmOk(); ok {
			a.SetRxDiamRealm(*rxDiamRealm)
		}
		nf.SetPcfInfo(*a)
	}
}

func copyBsfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// bsfInfo
	if nfprofile.BsfInfo != nil {
		a := models.NewBsfInfo()

		if dnnList, ok := nfprofile.BsfInfo.GetDnnListOk(); ok {
			a.SetDnnList(dnnList)
		}

		if ipDomainList, ok := nfprofile.BsfInfo.GetIpDomainListOk(); ok {
			a.SetIpDomainList(ipDomainList)
		}

		if ipv4AddressRanges, ok := nfprofile.BsfInfo.GetIpv4AddressRangesOk(); ok {
			b := make([]models.Ipv4AddressRange, len(ipv4AddressRanges))
			for i, rang := range ipv4AddressRanges {
				b[i].SetStart(strconv.FormatInt(Ipv4ToInt(rang.GetStart()), 10))
				b[i].SetEnd(strconv.FormatInt(Ipv4ToInt(rang.GetEnd()), 10))
			}
			a.SetIpv4AddressRanges(b)
		}

		if ipv6PrefixRanges, ok := nfprofile.BsfInfo.GetIpv6PrefixRangesOk(); ok {
			b := make([]models.Ipv6PrefixRange, len(ipv6PrefixRanges))
			for i, rang := range ipv6PrefixRanges {
				b[i].SetStart(Ipv6ToInt(rang.GetStart()).String())
				b[i].SetEnd(Ipv6ToInt(rang.GetEnd()).String())
			}
			a.SetIpv6PrefixRanges(b)
		}
		nf.SetBsfInfo(*a)
	}
}

func copyChfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// chfInfo
	if chfInfo, ok := nfprofile.GetChfInfoOk(); ok {
		a := models.NewChfInfo()

		if supiRangeList, ok := chfInfo.GetSupiRangeListOk(); ok {
			a.SetSupiRangeList(supiRangeList)
		}

		if gpsiRangeList, ok := chfInfo.GetGpsiRangeListOk(); ok {
			a.SetGpsiRangeList(gpsiRangeList)
		}

		if plmnRangeList, ok := chfInfo.GetPlmnRangeListOk(); ok {
			a.SetPlmnRangeList(plmnRangeList)
		}
		nf.SetChfInfo(*a)
	}
}

func copyNrfInfo(nf *models.NFProfile, nfprofile models.NFProfile) {
	// nrfInfo
	if nrfInfo, ok := nfprofile.GetNrfInfoOk(); ok {
		nf.SetNrfInfo(*nrfInfo)
	}
}

func copyRecoveryTime(nf *models.NFProfile, nfprofile models.NFProfile) {
	// recoveryTime
	if recoveryTime, ok := nfprofile.GetRecoveryTimeOk(); ok {
		// Update when restart (Setting by NF itself)
		nf.SetRecoveryTime(*recoveryTime)
	}
}

func copyNfServicePersistence(nf *models.NFProfile, nfprofile models.NFProfile) {
	// nfServicePersistence
	nf.SetNfServicePersistence(nfprofile.GetNfServicePersistence())
}

func copyNfServices(nf *models.NFProfile, nfprofile models.NFProfile) {
	// nfServices (deprecated in TS 29.510 Rel-16 in favor of nfServiceList, kept for backward compatibility)
	if nfServices, ok := nfprofile.GetNfServicesOk(); ok {
		a := make([]models.NFService, len(nfServices))
		copy(a, nfServices)
		nf.SetNfServices(a)
	}

	// nfServiceList: TS 29.510 Rel-16 replacement for nfServices
	if nfServiceList, ok := nfprofile.GetNfServiceListOk(); ok {
		a := make(map[string]models.NFService, len(*nfServiceList))
		for k, v := range *nfServiceList {
			a[k] = v
		}
		nf.SetNfServiceList(a)
	}
}

func GetNfInstanceURI(nfInstID string) string {
	return factory.NrfConfig.GetSbiUri() + "/nnrf-nfm/v1/nf-instances/" + nfInstID
}

func SetLocationHeader(nfprofile models.NFProfile) string {
	var modifyUL UriList
	var locationHeader []string

	// set nfprofile location
	locationHeader = append(locationHeader, GetNfInstanceURI(nfprofile.GetNfInstanceId()))

	collName := "urilist"
	nfType := nfprofile.GetNfType()
	filter := bson.M{fieldNfType: nfType}

	ul, err := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)
	if err != nil {
		logger.ManagementLog.Error(err)
		return locationHeader[0]
	}

	var originalUL UriList
	err = mapstructure.Decode(ul, &originalUL)
	if err != nil {
		logger.ManagementLog.Error(err)
		return locationHeader[0]
	}

	// obtain location header = NF URI
	nnrfUriList(&originalUL, &modifyUL, locationHeader)
	modifyUL.NfType = nfprofile.GetNfType()

	tmp, err := json.Marshal(modifyUL)
	if err != nil {
		logger.ManagementLog.Error(err)
	}
	putData := bson.M{}
	err = json.Unmarshal(tmp, &putData)
	if err != nil {
		logger.ManagementLog.Error(err)
	}

	ok, err := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, putData)
	if err != nil {
		logger.ManagementLog.Error(err)
	}
	if ok {
		logger.ManagementLog.Info("urilist update")
	} else {
		logger.ManagementLog.Info("urilist create")
	}

	return locationHeader[0]
}

func setUriListByFilter(filter bson.M, uriList *[]string) {
	filterNfTypeResultsRaw, err := dbadapter.DBClient.RestfulAPIGetMany("Subscriptions", filter)
	if err != nil {
		logger.ManagementLog.Error(err)
	}
	var filterNfTypeResults []models.SubscriptionData
	stringToDateTimeHook := func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t == reflect.TypeOf(time.Time{}) && f == reflect.TypeOf("") {
			return time.Parse(time.RFC3339, data.(string))
		}
		return data, nil
	}

	config := mapstructure.DecoderConfig{
		DecodeHook: stringToDateTimeHook,
		Result:     &filterNfTypeResults,
	}

	decoder, decoderErr := mapstructure.NewDecoder(&config)
	if decoderErr != nil {
		logger.ManagementLog.Errorf("converter setup failed: %v", decoderErr)
		return
	}

	if decodeErr := decoder.Decode(filterNfTypeResultsRaw); decodeErr != nil {
		logger.ManagementLog.Error(decodeErr)
	}

	for _, subscr := range filterNfTypeResults {
		*uriList = append(*uriList, subscr.GetNfStatusNotificationUri())
	}
}

func nnrfUriList(originalUL *UriList, UL *UriList, location []string) {
	var b *Links
	var flag bool
	var c []models.Link
	flag = true
	b = new(Links)
	items := originalUL.Link.Item
	size := len(location) + len(items)

	// check duplicate
	for _, item := range items {
		if item.GetHref() == location[0] {
			flag = false
			break
		}
	}

	if flag {
		c = make([]models.Link, size)
		copy(c, items)
		for i, loc := range location {
			c[len(items)+i].SetHref(loc)
		}
	} else {
		c = make([]models.Link, size-1)
		copy(c, items)
	}

	b.Item = c
	UL.Link = *b
}

func GetNotificationUri(nfProfile models.NFProfile) []string {
	var uriList []string

	addNfTypeCond(nfProfile, &uriList)
	addNfInstanceIDCond(nfProfile, &uriList)
	addServiceNameCond(nfProfile, &uriList)
	addAmfCond(nfProfile, &uriList)
	addGuamiListCond(nfProfile, &uriList)
	addNetworkSliceCond(nfProfile, &uriList)
	addNfGroupCond(nfProfile, &uriList)

	return uriList
}

func addNfTypeCond(nfProfile models.NFProfile, uriList *[]string) {
	// nfTypeCond
	nfTypeCond := bson.M{
		fieldSubscrCond: bson.M{
			fieldNfType: nfProfile.GetNfType(),
		},
	}
	setUriListByFilter(nfTypeCond, uriList)
}

func addNfInstanceIDCond(nfProfile models.NFProfile, uriList *[]string) {
	// NfInstanceIdCond
	nfInstanceIDCond := bson.M{
		fieldSubscrCond: bson.M{
			"nfInstanceId": nfProfile.GetNfInstanceId(),
		},
	}
	setUriListByFilter(nfInstanceIDCond, uriList)
}

func addServiceNameCond(nfProfile models.NFProfile, uriList *[]string) {
	// ServiceNameCond
	serviceNames := collectServiceNames(nfProfile)
	if len(serviceNames) > 0 {
		ServiceNameCond := bson.M{
			"subscrCond.serviceName": bson.M{
				"$in": serviceNames,
			},
		}
		setUriListByFilter(ServiceNameCond, uriList)
	}
}

// collectServiceNames gathers service names from both the deprecated
// nfServices array and its TS 29.510 Rel-16 replacement, nfServiceList, so
// notification matching works regardless of which field the NF used.
func collectServiceNames(nfProfile models.NFProfile) bson.A {
	var serviceNames bson.A
	if nfServices, ok := nfProfile.GetNfServicesOk(); ok {
		for _, nfService := range nfServices {
			serviceNames = append(serviceNames, string(nfService.ServiceName))
		}
	}
	if nfServiceList, ok := nfProfile.GetNfServiceListOk(); ok {
		for _, nfService := range *nfServiceList {
			serviceNames = append(serviceNames, string(nfService.ServiceName))
		}
	}
	return serviceNames
}

func addAmfCond(nfProfile models.NFProfile, uriList *[]string) {
	// AmfCond
	if amfInfo, ok := nfProfile.GetAmfInfoOk(); ok {
		amfCond := bson.M{
			fieldSubscrCond: bson.M{
				"amfSetId":    amfInfo.GetAmfSetId(),
				"amfRegionId": amfInfo.GetAmfRegionId(),
			},
		}
		setUriListByFilter(amfCond, uriList)
	}
}

func addGuamiListCond(nfProfile models.NFProfile, uriList *[]string) {
	if amfInfo, ok := nfProfile.GetAmfInfoOk(); ok {
		var guamiListFilter bson.M
		if guamiList, ok := amfInfo.GetGuamiListOk(); ok && len(guamiList) > 0 {
			var guamiListBsonArray bson.A
			for _, guami := range guamiList {
				tmp, err := json.Marshal(guami)
				if err != nil {
					logger.ManagementLog.Error(err)
				}
				guamiMarshal := bson.M{}
				err = json.Unmarshal(tmp, &guamiMarshal)
				if err != nil {
					logger.ManagementLog.Error(err)
				}

				guamiListBsonArray = append(guamiListBsonArray, bson.M{fieldSubscrCond: bson.M{"$elemMatch": guamiMarshal}})
			}
			guamiListFilter = bson.M{
				mongoOpOr: guamiListBsonArray,
			}
			setUriListByFilter(guamiListFilter, uriList)
		}
	}
}

func addNetworkSliceCond(nfProfile models.NFProfile, uriList *[]string) {
	// NetworkSliceCond
	if sNssais, ok := nfProfile.GetSNssaisOk(); ok && len(sNssais) > 0 {
		var networkSliceFilter bson.M
		var snssaisBsonArray bson.A
		for _, snssai := range sNssais {
			tmp, err := json.Marshal(snssai)
			if err != nil {
				logger.ManagementLog.Error(err)
			}
			snssaiMarshal := bson.M{}
			err = json.Unmarshal(tmp, &snssaiMarshal)
			if err != nil {
				logger.ManagementLog.Error(err)
			}

			snssaisBsonArray = append(snssaisBsonArray, bson.M{fieldSubscrCond: bson.M{"$elemMatch": snssaiMarshal}})
		}

		var nsiListBsonArray bson.A
		if nsiList, ok := nfProfile.GetNsiListOk(); ok {
			for _, nsi := range nsiList {
				nsiListBsonArray = append(nsiListBsonArray, nsi)
			}
		}

		if nsiListBsonArray != nil {
			networkSliceFilter = bson.M{
				"$and": bson.A{
					bson.M{
						"subscrCond.nsiList": bson.M{
							"$in": nsiListBsonArray,
						},
					},
					bson.M{
						mongoOpOr: snssaisBsonArray,
					},
				},
			}
		} else {
			networkSliceFilter = bson.M{
				"$and": bson.A{
					bson.M{
						mongoOpOr: snssaisBsonArray,
					},
				},
			}
		}
		setUriListByFilter(networkSliceFilter, uriList)
	}
}

func addNfGroupCond(nfProfile models.NFProfile, uriList *[]string) {
	// NfGroupCond
	nfType := nfProfile.GetNfType()
	udrInfo, okUdr := nfProfile.GetUdrInfoOk()
	udmInfo, okUdm := nfProfile.GetUdmInfoOk()
	ausfInfo, okAusf := nfProfile.GetAusfInfoOk()
	switch {
	case okUdr:
		nfGroupCond := bson.M{
			fieldSubscrCond: bson.M{
				fieldNfType:    nfType,
				fieldNfGroupId: udrInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, uriList)
	case okUdm:
		nfGroupCond := bson.M{
			fieldSubscrCond: bson.M{
				fieldNfType:    nfType,
				fieldNfGroupId: udmInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, uriList)
	case okAusf:
		nfGroupCond := bson.M{
			fieldSubscrCond: bson.M{
				fieldNfType:    nfType,
				fieldNfGroupId: ausfInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, uriList)
	}
}

func NnrfUriListLimit(originalUL *UriList, limit int) {
	// response limit

	if limit < len(originalUL.Link.Item) {
		b := new(Links)
		c := make([]models.Link, limit)
		copy(c, originalUL.Link.Item[:limit])
		b.Item = c
		originalUL.Link = *b
	}
}
