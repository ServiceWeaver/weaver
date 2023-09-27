#!/usr/bin/env bash

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
