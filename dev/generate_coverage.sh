#!/bin/sh
# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# Generate coverage information for Service Weaver source code by running tests
# in coverage mode. Produces
#
#    /tmp/$USER-files/serviceweaver-coverage.html
#    /tmp/$USER-files/serviceweaver-coverage-functions.txt
#
# Usage:
#       dev/generate_coverage.sh

tmpdir="/tmp/$USER-files"
covfile="$tmpdir/serviceweaver.cov"
covstripped="$tmpdir/serviceweaver-stripped.cov"
htmlfile="$tmpdir/serviceweaver-coverage.html"
funcfile="$tmpdir/serviceweaver-coverage-functions.txt"

set -e
mkdir -p "$tmpdir"

# Run tests and collect coverage info.
# To run GKE tests as well: go test -coverprofile=$covfile -coverpkg=./... ./...
rm -f "$covfile" "$covstripped"
go test -coverprofile="$covfile" -coverpkg=./... $(go list ./... | grep -v gke)

# Filter out generated code from coverage reports.
egrep -v '/weavertest/|/gke/|weaver_gen\.go:|\.pb\.go:' "$covfile" > "$covstripped"

# Create annotated HTML.
go tool cover -html="$covstripped" -o "$htmlfile"
>&2 echo "HTML report:   $htmlfile"
>&2 echo "Function list: $funcfile"

# Create function list.
go tool cover -func="$covstripped" | sort -k3n > "$funcfile"
