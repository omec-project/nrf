<!--
SPDX-FileCopyrightText: 2025 Canonical Ltd
SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
Copyright 2019 free5GC.org

SPDX-License-Identifier: Apache-2.0
-->
[![Go Report Card](https://goreportcard.com/badge/github.com/omec-project/nrf)](https://goreportcard.com/report/github.com/omec-project/nrf)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/omec-project/nrf/badge)](https://scorecard.dev/viewer/?uri=github.com/omec-project/nrf)

# NRF

Network Repository Function provides service discovery functionality in the 5G
core network. Each network function registers with NRF with a set of properties
called as NF Profile. Implements 3gpp specification 29.510. NRF Keeps the
profile data for each network function in the MongoDB. Supports Discovery and
registration procedure. When the discovery API is called, NRF fetches a matching
profile from the database and returns it to the caller.


## NRF flow diagram
![NRF Flow Diagram](/docs/images/README-NRF.png)

## Supported Features
- Registration of Network Functions
- Searching of matching Network functions
- Handling multiple instances of registration from Network Functions
- Supporting keepalive functionality to check the health of network functions


## Upcoming changes in NRF
- Supporting callbacks to send notification when a network function is added/removed/modified.
- Subscription management callbacks to network functions.
- NRF cache library which can be used by modules to avoid frequent queries to NRF

Compliance of the 5G Network functions can be found at [5G Compliance](https://docs.sd-core.opennetworking.org/main/overview/3gpp-compliance-5g.html)

## Dynamic Network configuration (via webconsole)

NRF fetches the latest PLMN configuration from Webconsole whenever a network function registers without providing
a list of supported PLMNs.
If a network function does not provide a list of supported PLMNs and NRF is not able to fetch any PLMN from Webconsole (or
Webconsole is unreachable), registration fails.
If a network function provides a list of supported PLMNs, it is registered without NRF fetching the configuration from Webconsole.

### Setting Up PLMN configuration fetch

Include the `webuiUri` of the Webconsole in the configuration file
```
configuration:
  ...
  webuiUri: https://webui:5001 # or http://webui:5001
  ...
```
The scheme (http:// or https://) must be explicitly specified.

## Reach out to us through

1. #sdcore-dev channel in [ONF Community Slack](https://aether5g-project.slack.com/)
2. Raise Github issues
