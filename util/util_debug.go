// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

//+build debug

package util

import (
	"github.com/free5gc/path_util"
)

// Path of HTTP2 key and log file

var (
	NrfLogPath = path_util.Free5gcPath("free5gc/nrfsslkey.log")
	NrfPemPath = path_util.Free5gcPath("free5gc/support/TLS/_debug.pem")
	NrfKeyPath = path_util.Free5gcPath("free5gc/support/TLS/_debug.key")
)
