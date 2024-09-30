// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 Canonical Ltd.

/*
 *  Metrics package is used to expose the metrics of the NRF service.
 */

package metrics

import (
	"net/http"

	"github.com/omec-project/nrf/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NrfStats captures NRF stats
type NrfStats struct {
	nrfRegistrations *prometheus.CounterVec
	nrfSubscriptions *prometheus.CounterVec
	nrfNfInstances   *prometheus.CounterVec
}

var nrfStats *NrfStats

func initNrfStats() *NrfStats {
	return &NrfStats{
		nrfRegistrations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nrf_registrations",
			Help: "Counter of total NRF registration events",
		}, []string{"query_type", "nf_type", "result"}),
		nrfSubscriptions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nrf_subscriptions",
			Help: "Counter of total NRF subscription events",
		}, []string{"query_type", "request_nf_type", "result"}),
		nrfNfInstances: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nrf_nf_instances",
			Help: "Counter of total NRF instances queries",
		}, []string{"request_nf_type", "target_nf_type", "result"}),
	}
}

func (ps *NrfStats) register() error {
	if err := prometheus.Register(ps.nrfRegistrations); err != nil {
		return err
	}
	if err := prometheus.Register(ps.nrfSubscriptions); err != nil {
		return err
	}
	if err := prometheus.Register(ps.nrfNfInstances); err != nil {
		return err
	}
	return nil
}

func init() {
	nrfStats = initNrfStats()

	if err := nrfStats.register(); err != nil {
		logger.InitLog.Errorln("NRF Stats register failed")
	}
}

// InitMetrics initialises NRF metrics
func InitMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.InitLog.Errorf("Could not open metrics port: %v", err)
	}
}

// IncrementNrfRegistrationsStats increments number of total NRF registrations
func IncrementNrfRegistrationsStats(queryType, nfType, result string) {
	nrfStats.nrfRegistrations.WithLabelValues(queryType, nfType, result).Inc()
}

// IncrementNrfSubscriptionsStats increments number of total NRF subscriptions
func IncrementNrfSubscriptionsStats(queryType, requestNfType, result string) {
	nrfStats.nrfSubscriptions.WithLabelValues(queryType, requestNfType, result).Inc()
}

// IncrementNrfNfInstancesStats increments number of total NRF queries
func IncrementNrfNfInstancesStats(requestNfType, targetNfType, result string) {
	nrfStats.nrfNfInstances.WithLabelValues(requestNfType, targetNfType, result).Inc()
}
