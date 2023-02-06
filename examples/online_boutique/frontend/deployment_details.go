// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frontend

import (
	"net/http"
	"os"

	"cloud.google.com/go/compute/metadata"
	"github.com/ServiceWeaver/weaver"
)

type deploymentDetails = map[string]string

func loadDeploymentDetails(log weaver.Logger, env string) deploymentDetails {
	host, err := os.Hostname()
	if err != nil {
		log.Error("Failed to fetch the frontend hostname", err)
	}
	var cluster, zone string
	if env == "gcp" {
		var metaServerClient = metadata.NewClient(&http.Client{})

		if cluster, err = metaServerClient.InstanceAttributeValue("cluster-name"); err != nil {
			log.Error("Failed to fetch the name of the cluster in which the frontend is running", err)
		}

		if zone, err = metaServerClient.Zone(); err != nil {
			log.Error("Failed to fetch the zone in which the frontend is running", err)
		}
	}

	log.Debug("Loaded deployment details",
		"cluster", cluster,
		"zone", zone,
		"hostname", host,
	)

	return deploymentDetails{
		"HOSTNAME":    host,
		"CLUSTERNAME": cluster,
		"ZONE":        zone,
	}
}
