// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

/*
 * NRF UriList
 */

package context

import "github.com/omec-project/openapi/models"

type Links struct {
	Item []models.Link `json:"item" bson:"item"`
}
