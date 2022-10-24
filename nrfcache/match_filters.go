// SPDX-FileCopyrightText: 2022 Infosys Limited
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nrf_cache

import (
	"encoding/json"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
	"regexp"
)

type MatchFilter func(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool

type MatchFilters map[models.NfType]MatchFilter

var matchFilters = MatchFilters{
	models.NfType_SMF:  MatchSmfProfile,
	models.NfType_AUSF: MatchAusfProfile,
	models.NfType_PCF:  MatchPcfProfile,
	models.NfType_NSSF: MatchNssfProfile,
	models.NfType_UDM:  MatchUdmProfile,
	models.NfType_AMF:  MatchAmfProfile,
}

func MatchSmfProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {

	matchFound := true

	if opts.ServiceNames.IsSet() {
		reqServiceNames := opts.ServiceNames.Value().([]models.ServiceName)
		matchCount := 0
		for _, sn := range reqServiceNames {
			for i := 0; i < len(*profile.NfServices); i++ {
				if (*profile.NfServices)[i].ServiceName == sn {
					matchCount++
					break
				}
			}
		}

		if matchCount == 0 {
			matchFound = false
		}
	}

	if matchFound && opts.Snssais.IsSet() {
		reqSnssais := opts.Snssais.Value().([]string)
		matchCount := 0

		for _, reqSnssai := range reqSnssais {
			var snssai models.Snssai
			err := json.Unmarshal([]byte(reqSnssai), &snssai)
			if err != nil {
				return false
			}

			// Snssai in the smfInfo has priority
			if profile.SmfInfo != nil && profile.SmfInfo.SNssaiSmfInfoList != nil {
				for _, s := range *profile.SmfInfo.SNssaiSmfInfoList {
					if s.SNssai != nil && (*s.SNssai) == snssai {
						matchCount++
					}
				}
			} else if profile.AllowedNssais != nil {
				for _, s := range *profile.AllowedNssais {
					if s == snssai {
						matchCount++
					}
				}
			}

		}

		// if at least one matching snssai has been found
		if matchCount == 0 {
			matchFound = false
		}

	}

	// validate dnn
	if matchFound && opts.Dnn.IsSet() {
		// if a dnn is provided by the upper layer, check for the exact match
		// or wild card match
		dnnMatched := false

		if profile.SmfInfo != nil && profile.SmfInfo.SNssaiSmfInfoList != nil {
		matchDnnLoop:
			for _, s := range *profile.SmfInfo.SNssaiSmfInfoList {
				if s.DnnSmfInfoList != nil {
					for _, d := range *s.DnnSmfInfoList {
						if d.Dnn == opts.Dnn.Value() || d.Dnn == "*" {
							dnnMatched = true
							break matchDnnLoop
						}
					}
				}
			}
		}
		matchFound = dnnMatched
	}
	logger.UtilLog.Tracef("SMF match found = %v", matchFound)
	return matchFound
}

func MatchSupiRange(supi string, supiRange []models.SupiRange) bool {
	matchCount := 0
	for _, s := range supiRange {
		if len(s.Pattern) > 0 {
			r, _ := regexp.Compile(s.Pattern)
			if r.MatchString(supi) {
				matchCount++
			}

		} else if s.Start <= supi && supi <= s.End {
			matchCount++
		}
	}

	return matchCount > 0
}

func MatchAusfProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {
	matchFound := true
	if opts.Supi.IsSet() {
		if profile.AusfInfo != nil && len(profile.AusfInfo.SupiRanges) > 0 {
			matchFound = MatchSupiRange(opts.Supi.Value(), profile.AusfInfo.SupiRanges)
		}
	}
	logger.UtilLog.Tracef("Ausf match found = %v", matchFound)
	return matchFound
}

func MatchNssfProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {
	logger.UtilLog.Traceln("Nssf match found ")
	return true
}

func MatchAmfProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {
	matchFound := true

	if opts.TargetPlmnList.IsSet() {
		if profile.PlmnList != nil {
			plmnMatchCount := 0

			targetPlmnList := opts.TargetPlmnList.Value().([]string)
			for _, targetPlmn := range targetPlmnList {
				var plmn models.PlmnId
				err := json.Unmarshal([]byte(targetPlmn), &plmn)

				if err != nil {
					return false
				}

				for _, profilePlmn := range *profile.PlmnList {
					if profilePlmn == plmn {
						plmnMatchCount++
						break
					}
				}
			}
			matchFound = plmnMatchCount > 0
		}
	}

	if matchFound && profile.AmfInfo != nil {
		if opts.Guami.IsSet() {
			if profile.AmfInfo.GuamiList != nil {
				guamiMatchCount := 0

				guamiList := opts.Guami.Value().([]string)
				for _, guami := range guamiList {
					var guamiOpt models.Guami
					err := json.Unmarshal([]byte(guami), &guamiOpt)

					if err != nil {
						return false
					}

					for _, guami := range *profile.AmfInfo.GuamiList {
						if guamiOpt == guami {
							guamiMatchCount++
							break
						}
					}
				}
				matchFound = guamiMatchCount > 0
			}
		}

		if matchFound && opts.AmfRegionId.IsSet() {
			if len(profile.AmfInfo.AmfRegionId) > 0 {
				if profile.AmfInfo.AmfRegionId != opts.AmfRegionId.Value() {
					matchFound = false
				}
			}
		}

		if matchFound && opts.AmfSetId.IsSet() {
			if len(profile.AmfInfo.AmfSetId) > 0 {
				if profile.AmfInfo.AmfSetId != opts.AmfSetId.Value() {
					matchFound = false
				}
			}
		}
	}

	logger.UtilLog.Tracef("Amf match found = %v", matchFound)
	return matchFound
}

func MatchPcfProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {
	matchFound := true
	if opts.Supi.IsSet() {
		if profile.PcfInfo != nil && len(profile.PcfInfo.SupiRanges) > 0 {
			matchFound = MatchSupiRange(opts.Supi.Value(), profile.PcfInfo.SupiRanges)
		}
	}
	logger.UtilLog.Tracef("PCF match found = %v", matchFound)
	return matchFound
}

func MatchUdmProfile(profile *models.NfProfile, opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) bool {
	matchFound := true
	if opts.Supi.IsSet() {
		if profile.UdmInfo != nil && len(profile.UdmInfo.SupiRanges) > 0 {
			matchFound = MatchSupiRange(opts.Supi.Value(), profile.UdmInfo.SupiRanges)
		}
	}
	logger.UtilLog.Tracef("UDM match found = %v", matchFound)
	return matchFound
}
