#!/usr/bin/env bash
# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


set -euo pipefail

main() {
  psql -U postgres -c "CREATE USER admin WITH PASSWORD 'admin';"
  psql -U postgres -c "CREATE DATABASE postgresdb WITH OWNER admin;"
  psql -U postgres -c "CREATE DATABASE accountsdb WITH OWNER admin;"
  psql postgresdb admin -f /app/postgresdb.sql
  psql accountsdb admin -f /app/accountsdb.sql
  POSTGRES_DB=postgresdb POSTGRES_USER=admin POSTGRES_PASSWORD=admin LOCAL_ROUTING_NUM=883745000 USE_DEMO_DATA=True /app/1_create_transactions.sh
}

main "$@"
