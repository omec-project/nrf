// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

/*
 * NRF UriList
 */

package context

import (
	"github.com/go-viper/mapstructure/v2"
	"github.com/omec-project/openapi/v2/models"
)

// UriList is the stored form of the NF Instance URIs of one NF type. Its "_links" member follows the
// TS 29.510 clause 6.1.6.2.25 attribute name; the response of the NF instances query is built from it
// with BuildUriListResponse.
type UriList struct {
	NfType models.NFType `json:"nfType" bson:"nfType"`
	Links  Links         `json:"_links" bson:"_links" mapstructure:"_links"`

	// LegacyLink is read only: documents stored before the "_links" name was adopted carry their items
	// under "_link". It is never written back (json:"-"); DecodeUriList moves it into Links.
	LegacyLink Links `json:"-" bson:"-" mapstructure:"_link"`
}

// DecodeUriList decodes a stored urilist document, accepting documents written with the pre-TS 29.510
// "_link" member. The returned list always holds its items in Links.
func DecodeUriList(raw map[string]any) (*UriList, error) {
	uriList := &UriList{}
	if err := mapstructure.Decode(raw, uriList); err != nil {
		return nil, err
	}
	if len(uriList.Links.Item) == 0 {
		uriList.Links = uriList.LegacyLink
	}
	uriList.LegacyLink = Links{}
	return uriList, nil
}

// BuildUriListResponse builds the TS 29.510 UriList response body. totalItemCount is the number of
// items matching the query before any limit or paging; items is what is returned. With no items,
// "_links" holds only "self" and totalItemCount is 0 (clause 6.1.6.2.25).
func BuildUriListResponse(items []models.Link, total int, selfHref string) *models.UriList {
	links := make(map[string]models.LinksValueSchema, 2)
	self := models.NewLink()
	self.SetHref(selfHref)
	links["self"] = models.LinkAsLinksValueSchema(self)
	if len(items) > 0 {
		links["item"] = models.ArrayOfLinkAsLinksValueSchema(&items)
	}
	response := models.NewUriList()
	response.SetLinks(links)
	response.SetTotalItemCount(int32(total))
	return response
}

// PaginateLinks returns the items of the 1-based page of the given size (clause 6.1.3.2.3.1, query
// parameters "page-number" and "page-size"). A page past the end is empty.
func PaginateLinks(items []models.Link, pageNumber, pageSize int) []models.Link {
	if pageNumber < 1 || pageSize < 1 {
		return items
	}
	// Compare against the collection length before multiplying or adding: both parameters can be
	// arbitrarily large, so start and end must never be computed from unchecked products or sums.
	pages := len(items) / pageSize
	if len(items)%pageSize != 0 {
		pages++
	}
	if pageNumber > pages {
		return nil
	}
	start := (pageNumber - 1) * pageSize
	end := start + min(pageSize, len(items)-start)
	return items[start:end]
}
