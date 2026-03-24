#!/usr/bin/env bash
#
# Export a Keycloak realm to JSON (clients, roles, and client secrets).
# Run this on your host. Requires: curl, jq.
#
# Usage:
#   ./scripts/export-keycloak-realm.sh [keycloak_container_name] [realm_name]
#
# If Keycloak is not in Docker, set KEYCLOAK_URL (e.g. http://localhost:8080)
# and use container_name "none" or leave empty and set KEYCLOAK_URL only.
#
# Examples:
#   ./scripts/export-keycloak-realm.sh keycloak web-frontend
#   KEYCLOAK_URL=http://localhost:8080 ./scripts/export-keycloak-realm.sh none web-frontend
#

set -e

CONTAINER_NAME="${1:-keycloak}"
REALM_NAME="${2:-web-frontend}"
KEYCLOAK_URL="${KEYCLOAK_URL:-}"
ADMIN_USER="${KEYCLOAK_ADMIN:-admin}"
ADMIN_PASS="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
OUTPUT_FILE="${3:-realm-${REALM_NAME}-export.json}"

# Resolve base URL: from container or from KEYCLOAK_URL
if [ -n "$KEYCLOAK_URL" ]; then
  BASE_URL="$KEYCLOAK_URL"
  echo "Using KEYCLOAK_URL=$BASE_URL"
else
  if [ "$CONTAINER_NAME" = "none" ] || [ -z "$CONTAINER_NAME" ]; then
    echo "Set KEYCLOAK_URL (e.g. http://localhost:8080) or pass keycloak container name as first argument."
    exit 1
  fi
  BASE_URL="http://localhost:8080"
  echo "Using container: $CONTAINER_NAME (ensure Keycloak is published on port 8080)"
fi

get_token() {
  local url="$1"
  if [ -n "$KEYCLOAK_URL" ]; then
    curl -s -X POST "${url}/realms/master/protocol/openid-connect/token" \
      -H "Content-Type: application/x-www-form-urlencoded" \
      -d "client_id=admin-cli" \
      -d "username=${ADMIN_USER}" \
      -d "password=${ADMIN_PASS}" \
      -d "grant_type=password" | jq -r '.access_token'
  else
    docker exec "$CONTAINER_NAME" curl -s -X POST "http://localhost:8080/realms/master/protocol/openid-connect/token" \
      -H "Content-Type: application/x-www-form-urlencoded" \
      -d "client_id=admin-cli" \
      -d "username=${ADMIN_USER}" \
      -d "password=${ADMIN_PASS}" \
      -d "grant_type=password" | jq -r '.access_token'
  fi
}

get_realm() {
  local url="$1"
  local token="$2"
  if [ -n "$KEYCLOAK_URL" ]; then
    curl -s -H "Authorization: Bearer ${token}" "${url}/admin/realms/${REALM_NAME}"
  else
    docker exec "$CONTAINER_NAME" curl -s -H "Authorization: Bearer ${token}" "http://localhost:8080/admin/realms/${REALM_NAME}"
  fi
}

get_client_secret() {
  local url="$1"
  local token="$2"
  local client_id_uuid="$3"
  if [ -n "$KEYCLOAK_URL" ]; then
    curl -s -H "Authorization: Bearer ${token}" "${url}/admin/realms/${REALM_NAME}/clients/${client_id_uuid}/client-secret"
  else
    docker exec "$CONTAINER_NAME" curl -s -H "Authorization: Bearer ${token}" "http://localhost:8080/admin/realms/${REALM_NAME}/clients/${client_id_uuid}/client-secret"
  fi
}

echo "Fetching admin token..."
TOKEN=$(get_token "$BASE_URL")
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "Failed to get token. Check admin user/password and that Keycloak is reachable."
  exit 1
fi

echo "Exporting realm: $REALM_NAME ..."
REALM_JSON=$(get_realm "$BASE_URL" "$TOKEN")
if echo "$REALM_JSON" | jq -e .realm >/dev/null 2>&1; then
  : # ok
else
  echo "Realm not found or invalid response. Check realm name and token."
  echo "$REALM_JSON" | head -c 200
  exit 1
fi

# Enrich clients with secrets (for confidential clients)
CLIENTS=$(echo "$REALM_JSON" | jq -c '.clients // []')
NEW_CLIENTS="[]"
if [ "$CLIENTS" != "[]" ]; then
  COUNT=$(echo "$CLIENTS" | jq 'length')
  for i in $(seq 0 $((COUNT - 1))); do
    CLIENT=$(echo "$REALM_JSON" | jq -c ".clients[$i]")
    ID=$(echo "$CLIENT" | jq -r '.id')
    # Only fetch secret for clients that typically have one (confidential)
    if echo "$CLIENT" | jq -e '.publicClient == false or .publicClient == null' >/dev/null 2>&1; then
      SECRET_JSON=$(get_client_secret "$BASE_URL" "$TOKEN" "$ID")
      SECRET=$(echo "$SECRET_JSON" | jq -r '.value // empty')
      if [ -n "$SECRET" ]; then
        CLIENT=$(echo "$CLIENT" | jq --arg s "$SECRET" '. + {secret: $s}')
      fi
    fi
    NEW_CLIENTS=$(echo "$NEW_CLIENTS" | jq --argjson c "$CLIENT" '. + [$c]')
  done
  REALM_JSON=$(echo "$REALM_JSON" | jq --argjson nc "$NEW_CLIENTS" '.clients = $nc')
fi

echo "$REALM_JSON" > "$OUTPUT_FILE"
echo "Saved to: $OUTPUT_FILE"
