// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/omec-project/nrf/logger"
)

// Ipv6ToInt - Convert Ipv6 string to *bigInt
func Ipv6ToInt(ipv6 string) *big.Int {
	ipv6 = ipv6 + "/32"
	ip, _, err := net.ParseCIDR(ipv6)
	if err != nil {
		logger.UtilLog.Errorln("error", ip, err)
	}
	IPv6Int := big.NewInt(0)
	IPv6Int.SetBytes(ip)
	return IPv6Int
}

// Ipv4ToInt - Convert Ipv4 string to int64
func Ipv4ToInt(ipv4 string) int64 {
	ipv4 = ipv4 + "/24"
	ip, _, err := net.ParseCIDR(ipv4)
	if err != nil {
		logger.UtilLog.Errorln("error", ip, err)
	}
	IPv4Int := big.NewInt(0)
	IPv4Int.SetBytes(ip)
	return IPv4Int.Int64()
}

// Ipv4IntToIpv4String - Convert Ipv4 int64 to string
func Ipv4IntToIpv4String(ip int64) string {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)).String()
}

// Ipv6IntToIpv6String - Convert Ipv6 *big.Int to string
func Ipv6IntToIpv6String(ip *big.Int) string {
	// FillBytes (not ip.Bytes, which strips leading zero bytes) guarantees a
	// full 16-byte address, since a leading zero byte is a normal IPv6 value.
	var buf [net.IPv6len]byte
	ip.FillBytes(buf[:])
	// netip.Addr.String (unlike net.IP.String) keeps IPv4-mapped values in
	// IPv6 notation instead of collapsing them to dotted-decimal IPv4.
	return netip.AddrFrom16(buf).String()
}

// EncodeGroupId - Encode GroupId (hex segments [10]-[3]-[3]-[25]) to a
// fixed-width decimal string ([13][3][3][31]) so lexicographic ordering of
// the result matches numeric ordering of the two converted hex components:
// 16^10-1 needs 13 decimal digits and 16^25-1 needs 31.
func EncodeGroupId(groupId string) string {
	externalGroupIdentitySplit := strings.Split(groupId, "-")

	groupServiceIdentifier, err := strconv.ParseInt(externalGroupIdentitySplit[0], 16, 64)
	if err != nil {
		logger.UtilLog.Errorln("invalid group service identifier", externalGroupIdentitySplit[0], err)
	}
	// 25-digit local-group component can exceed int64, so parse with big.Int.
	localGroupId, ok := new(big.Int).SetString(externalGroupIdentitySplit[3], 16)
	if !ok {
		logger.UtilLog.Errorln("invalid local group id", externalGroupIdentitySplit[3])
		localGroupId = new(big.Int)
	}

	networkComponent := externalGroupIdentitySplit[2]
	if len(networkComponent) == 2 {
		networkComponent = "0" + networkComponent
	}
	return fmt.Sprintf("%013d%s%s%031d",
		groupServiceIdentifier, externalGroupIdentitySplit[1], networkComponent, localGroupId)
}
