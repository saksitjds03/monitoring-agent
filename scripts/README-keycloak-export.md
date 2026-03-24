# Keycloak realm export

Export a realm (clients, roles, client secrets) from a running Keycloak container for import into the Keycloak service in docker-compose.

## Option 1: Script (recommended – includes client secrets)

From the `agent-service-monitoring` directory:

```bash
# Export realm "web-frontend" from container named "keycloak" into realm-web-frontend-export.json
./scripts/export-keycloak-realm.sh keycloak web-frontend

# Custom output file
./scripts/export-keycloak-realm.sh keycloak web-frontend my-realm.json

# If Keycloak runs on the host (e.g. http://localhost:8080)
KEYCLOAK_URL=http://localhost:8080 ./scripts/export-keycloak-realm.sh none web-frontend
```

Requires: `curl`, `jq`. Uses `KEYCLOAK_ADMIN` / `KEYCLOAK_ADMIN_PASSWORD` (default `admin`/`admin`) if not set.

## Option 2: Manual docker exec (realm only; secrets not included)

Replace `KEYCLOAK_CONTAINER` with your Keycloak container name and `web-frontend` with your realm name.

**1. Get an admin token**

```bash
TOKEN=$(docker exec KEYCLOAK_CONTAINER curl -s -X POST http://localhost:8080/realms/master/protocol/openid-connect/token \
  -d "client_id=admin-cli" \
  -d "username=admin" \
  -d "password=admin" \
  -d "grant_type=password" | jq -r '.access_token')
```

**2. Export realm to a file on the host**

```bash
docker exec KEYCLOAK_CONTAINER curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/admin/realms/web-frontend" > realm-export.json
```

Client secrets are **not** included in this export; you will need to set them again in the new Keycloak or use the script above to include secrets.

## Import into the new Keycloak (docker-compose)

1. Start the stack so the Keycloak service is running.
2. Create the realm (or use the same realm name) and use **Realm → Import** in the Admin UI, or copy the JSON into the container and use the import/start-import command depending on your Keycloak version.
