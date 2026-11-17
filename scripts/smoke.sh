#!/usr/bin/env bash
# End-to-end smoke of the saas fixture.
#
#   scripts/smoke.sh <base-url-web> [<base-url-api>]
#
# The web URL is required: the item is created through the web app, exactly like
# the form does. The api URL is optional; when given, the api is probed directly
# (health, X-Signature from the sealed SIGNING_SECRET, full CRUD).
set -euo pipefail

webUrl="${1:?usage: scripts/smoke.sh <base-url-web> [<base-url-api>]}"
webUrl="${webUrl%/}"
apiUrl="${2:-}"
if [[ -n "$apiUrl" ]]; then
  apiUrl="${apiUrl%/}"
fi

for command in curl jq; do
  command -v "$command" > /dev/null || { echo "FAIL $command is required" >&2; exit 1; }
done

responseFile="$(mktemp)"
headersFile="$(mktemp)"
trap 'rm -f "$responseFile" "$headersFile"' EXIT

expectStatus() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL $label: expected HTTP $expected, got $actual" >&2
    echo "     body: $(head -c 400 "$responseFile")" >&2
    exit 1
  fi
  echo "ok   $label ($actual)"
}

expectField() {
  local label="$1" filter="$2"
  jq -e "$filter" "$responseFile" > /dev/null || {
    echo "FAIL $label: unexpected body $(head -c 400 "$responseFile")" >&2
    exit 1
  }
  echo "ok   $label"
}

request() {
  curl -s -m 20 -o "$responseFile" -D "$headersFile" -w '%{http_code}' "$@"
}

itemName="smoke-$(date +%s)"
renamedName="${itemName}-renamed"

# 1. Health: web, and the api behind it (web's /health pings the api).
expectStatus "GET web /health" 200 "$(request "$webUrl/health")"
expectField "web health is ok" '.status == "ok"'
expectField "api behind web is ok" '.api.status == "ok"'

# 2. The api replica pair answered the web probe; probe it directly as well.
if [[ -n "$apiUrl" ]]; then
  expectStatus "GET api /health" 200 "$(request "$apiUrl/health")"
  expectField "api health is ok" '.status == "ok"'
  expectField "api sees Postgres" '.database == "ok"'
  expectField "api sees Redis" '.redis == "ok"'
  if grep -qi '^x-signature: sha256=' "$headersFile"; then
    echo "ok   api signs responses with SIGNING_SECRET"
  else
    echo "FAIL api did not send X-Signature (is SIGNING_SECRET set?)" >&2
    exit 1
  fi
fi

# 3. Create through the web app, the same path the form uses.
expectStatus "POST web /api/items" 201 "$(request -X POST -H 'content-type: application/json' \
  -d "{\"name\":\"$itemName\"}" "$webUrl/api/items")"
itemIdentifier="$(jq -r '.id' "$responseFile")"
[[ "$(jq -r '.name' "$responseFile")" == "$itemName" ]] || { echo "FAIL created item returned another name" >&2; exit 1; }
[[ "$(jq -r '.status' "$responseFile")" == "queued" ]] || { echo "FAIL new item is not queued" >&2; exit 1; }
echo "ok   web created item $itemIdentifier as queued"

# 4. The worker must flip it to processed within 60 seconds.
deadline=$(( SECONDS + 60 ))
status=""
while (( SECONDS < deadline )); do
  if [[ "$(request "$webUrl/api/items/$itemIdentifier")" == "200" ]]; then
    status="$(jq -r '.status' "$responseFile")"
    if [[ "$status" == "processed" ]]; then
      break
    fi
  fi
  sleep 2
done
if [[ "$status" != "processed" ]]; then
  echo "FAIL item $itemIdentifier is still '$status' after 60s (worker did not drain the queue)" >&2
  exit 1
fi
echo "ok   worker processed item $itemIdentifier"

# 5. Stats reflect the processed item.
expectStatus "GET web /api/stats" 200 "$(request "$webUrl/api/stats")"
expectField "stats count at least one item" '.total >= 1'
expectField "stats count at least one processed item" '.processed >= 1'
echo "ok   stats: $(jq -c . "$responseFile")"

# 6. Full CRUD, directly against the api when its URL is known, otherwise
#    through the web proxy (which forwards to the same endpoints).
crudBase="${apiUrl:-$webUrl}"

expectStatus "POST $crudBase/api/items" 201 "$(request -X POST -H 'content-type: application/json' \
  -d "{\"name\":\"$itemName\"}" "$crudBase/api/items")"
crudIdentifier="$(jq -r '.id' "$responseFile")"

expectStatus "GET $crudBase/api/items/$crudIdentifier" 200 "$(request "$crudBase/api/items/$crudIdentifier")"
[[ "$(jq -r '.name' "$responseFile")" == "$itemName" ]] || { echo "FAIL read returned another name" >&2; exit 1; }

expectStatus "GET $crudBase/api/items lists it" 200 "$(request "$crudBase/api/items")"
jq -e --arg wanted "$crudIdentifier" 'any(.[]; (.id | tostring) == $wanted)' "$responseFile" > /dev/null \
  || { echo "FAIL list does not contain item $crudIdentifier" >&2; exit 1; }

expectStatus "PUT $crudBase/api/items/$crudIdentifier" 200 "$(request -X PUT -H 'content-type: application/json' \
  -d "{\"name\":\"$renamedName\"}" "$crudBase/api/items/$crudIdentifier")"
[[ "$(jq -r '.name' "$responseFile")" == "$renamedName" ]] || { echo "FAIL update did not rename" >&2; exit 1; }

expectStatus "DELETE $crudBase/api/items/$crudIdentifier" 204 "$(request -X DELETE "$crudBase/api/items/$crudIdentifier")"
expectStatus "GET deleted item" 404 "$(request "$crudBase/api/items/$crudIdentifier")"

echo "smoke ok on $webUrl${apiUrl:+ and $apiUrl}"
echo "OK"
