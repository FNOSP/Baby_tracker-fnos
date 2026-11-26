#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="${ROOT_DIR}/source"
PACK_DIR="${ROOT_DIR}/packaging/baby-tracker"
BUILD_DIR="${ROOT_DIR}/.build"
TEST_BINARY="${BUILD_DIR}/baby-tracker-test"
SMOKE_DIR="${BUILD_DIR}/go-smoke"
SMOKE_DATA_DIR="${SMOKE_DIR}/share/baby-tracker/data"
SMOKE_SOCKET="${SMOKE_DIR}/app/app.sock"
SMOKE_COOKIE="${SMOKE_DIR}/cookies.txt"
SMOKE_LOG="${SMOKE_DIR}/var/app.log"
SMOKE_PORT="${SMOKE_PORT:-18964}"
SERVER_PID=""

log() {
    printf '[test] %s\n' "$1"
}

cleanup() {
    if [ -n "${SERVER_PID}" ] && kill -0 "${SERVER_PID}" 2>/dev/null; then
        kill "${SERVER_PID}" 2>/dev/null || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
}
trap cleanup EXIT

command -v go >/dev/null 2>&1 || {
    printf '[test] ERROR: Go is required.\n' >&2
    exit 1
}
command -v curl >/dev/null 2>&1 || {
    printf '[test] ERROR: curl is required.\n' >&2
    exit 1
}
command -v jq >/dev/null 2>&1 || {
    printf '[test] ERROR: jq is required.\n' >&2
    exit 1
}

mkdir -p "${BUILD_DIR}"

log "Checking Go formatting"
unformatted="$(gofmt -l "${ROOT_DIR}/cmd" "${ROOT_DIR}/internal")"
if [ -n "${unformatted}" ]; then
    printf '%s\n' "${unformatted}" >&2
    exit 1
fi

log "Running go vet, Go tests, and race tests"
(
    cd "${ROOT_DIR}"
    go vet ./...
    go test ./...
    go test -race ./...
)

log "Checking JavaScript syntax"
if command -v node >/dev/null 2>&1; then
    while IFS= read -r -d '' file; do
        node --check "${file}"
    done < <(find "${SOURCE_DIR}/static" -type f -name '*.js' ! -path '*/vendor/*' -print0)
    node - "${SOURCE_DIR}/static/js/app.js" <<'JS'
const assert = require('assert');
const fs = require('fs');
global.document = {
    documentElement: { dataset: { appBase: '/app/baby-tracker' } },
    addEventListener() {},
};
eval(fs.readFileSync(process.argv[2], 'utf8'));
assert.equal(appUrl('/api/export/csv'), '/app/baby-tracker/api/export/csv');
assert.equal(appUrl('/app/baby-tracker/trends'), '/app/baby-tracker/trends');
assert.equal(appUrl('https://example.com/test'), 'https://example.com/test');
JS
fi

log "Validating frontend download and navigation URLs"
grep -q "window.location.href = appUrl('/api/export/csv')" "${SOURCE_DIR}/static/js/admin.js"
grep -q "window.location.href = appUrl('/api/backup/export')" "${SOURCE_DIR}/static/js/admin.js"
grep -q "normalized.startsWith(\`\${base}/\`)" "${SOURCE_DIR}/static/js/app.js"
grep -q 'data-app-base="{{ app_base }}"' "${SOURCE_DIR}/templates/base.html"
[ "$(grep -c '<details class="admin-details">' "${SOURCE_DIR}/templates/admin.html")" = "8" ]
[ "$(grep -c '<span class="admin-details-heading">' "${SOURCE_DIR}/templates/admin.html")" = "8" ]
[ "$(grep -c '<span class="admin-details-description">' "${SOURCE_DIR}/templates/admin.html")" = "8" ]
grep -Fq '.admin-details[open] .admin-details-icon' "${SOURCE_DIR}/static/css/style.css"
grep -Fq '.admin-details-content' "${SOURCE_DIR}/static/css/style.css"
grep -Fq '@media (prefers-reduced-motion: reduce)' "${SOURCE_DIR}/static/css/style.css"
grep -Fq '@media (prefers-reduced-transparency: reduce)' "${SOURCE_DIR}/static/css/style.css"
grep -Fq '@media (prefers-contrast: more)' "${SOURCE_DIR}/static/css/style.css"
grep -q '/static/js/motion.js' "${SOURCE_DIR}/templates/base.html"
grep -q '/static/js/motion.js' "${SOURCE_DIR}/static/sw.js"
grep -q 'function loadMcpApiKey' "${SOURCE_DIR}/static/js/admin.js"
grep -q 'function generateMcpApiKey' "${SOURCE_DIR}/static/js/admin.js"
grep -q 'function toggleMcpService' "${SOURCE_DIR}/static/js/admin.js"
grep -q 'role="switch"' "${SOURCE_DIR}/templates/admin.html"
grep -q 'id="mcp-settings-details"' "${SOURCE_DIR}/templates/admin.html"
grep -q 'data-generate-mcp-key' "${SOURCE_DIR}/templates/admin.html"
grep -q '/api/mcp/config' "${ROOT_DIR}/internal/app/routes.go"
grep -q 'mcpEnabledSetting' "${ROOT_DIR}/internal/app/mcp.go"
if grep -q 'data-admin-collapse\|admin-collapse-panel\|toggleAdminCollapse' \
    "${SOURCE_DIR}/templates/admin.html" "${SOURCE_DIR}/static/css/style.css" "${SOURCE_DIR}/static/js/admin.js"; then
    printf '[test] ERROR: legacy JavaScript admin collapse code remains.\n' >&2
    exit 1
fi

log "Validating Go-only package configuration"
stable_desc='<p>Baby Tracker 用于记录新生儿与婴幼儿的喂养、排泄、成长和健康数据，所有数据保存在飞牛本地。</p><p>应用支持 Home Assistant 集成：在“管理 > 管理配置”生成 API 密钥，然后在“Home Assistant 集成”中配置服务地址并生成 YAML。</p><p><strong>重置管理密码：</strong>从飞牛桌面重新进入应用，通过统一网关一键登录，在“管理 > 管理配置”点击“修改管理密码”。</p>'
grep -Fq "desc                  = ${stable_desc}" "${PACK_DIR}/manifest" || {
    printf '[test] ERROR: manifest description must remain the stable product introduction; put release notes in CHANGELOG.md.\n' >&2
    exit 1
}
[ -s "${ROOT_DIR}/CHANGELOG.md" ] || {
    printf '[test] ERROR: CHANGELOG.md is missing.\n' >&2
    exit 1
}
grep -q '^changelog[[:space:]]*=' "${PACK_DIR}/manifest"
if grep -q 'install_dep_apps' "${PACK_DIR}/manifest"; then
    printf '[test] ERROR: manifest still installs a Python runtime.\n' >&2
    exit 1
fi
if grep -q 'gunicorn\|virtualenv\|python312\|wheelhouse\|wsgi:application' "${PACK_DIR}/cmd/main"; then
    printf '[test] ERROR: launcher still references the legacy Python runtime.\n' >&2
    exit 1
fi
grep -q 'baby-tracker-linux-amd64' "${PACK_DIR}/cmd/main"
grep -q 'baby-tracker-linux-arm64' "${PACK_DIR}/cmd/main"
grep -q 'BABY_TRACKER_GATEWAY_SOCKET' "${PACK_DIR}/cmd/main"
grep -q 'validate_service_values' "${PACK_DIR}/cmd/main"
bash -n "${PACK_DIR}/cmd/main"

log "Validating data-share mapping and listener parser"
rm -rf "${BUILD_DIR}/launcher-test"
mkdir -p "${BUILD_DIR}/launcher-test/share/baby-tracker/data" "${BUILD_DIR}/launcher-test/pkgvar/data"
(
    export TRIM_APPDEST="${BUILD_DIR}/launcher-test/app"
    export TRIM_PKGVAR="${BUILD_DIR}/launcher-test/pkgvar"
    export TRIM_PKGETC="${BUILD_DIR}/launcher-test/etc"
    export TRIM_PKGTMP="${BUILD_DIR}/launcher-test/tmp"
    export TRIM_DATA_SHARE_PATHS="${BUILD_DIR}/launcher-test/share/baby-tracker/data"
    source "${PACK_DIR}/cmd/main"
    map_data_dir
    [ -L "${TRIM_PKGVAR}/data" ]
    [ "$(readlink "${TRIM_PKGVAR}/data")" = "${TRIM_DATA_SHARE_PATHS}" ]

    for address in '0.0.0.0' '127.0.0.1' '8.8.8.8' '192.168.1.10' '01.02.03.004'; do
        normalized="$(normalize_bind_address "  ${address}  ")"
        [ -n "${normalized}" ]
    done
    [ "$(normalize_bind_address '01.02.03.004')" = '1.2.3.4' ]
    for address in '999.1.1.1' '224.0.0.1' '239.1.1.1' 'not-an-ip'; do
        if normalize_bind_address "${address}" >/dev/null 2>&1; then
            exit 1
        fi
    done

    wizard_bind_address='0.0.0.0'
    wizard_port='18965'
    validate_service_values
    [ "$(cat "${TRIM_PKGETC}/bind_address")" = '0.0.0.0' ]
    [ "$(cat "${TRIM_PKGETC}/service_port")" = '18965' ]
    wizard_bind_address=''
    wizard_port=''
    validate_service_values
    [ ! -e "${TRIM_PKGETC}/bind_address" ]
    [ ! -e "${TRIM_PKGETC}/service_port" ]
    if (wizard_bind_address='192.168.1.20'; wizard_port=''; validate_service_values) >/dev/null 2>&1; then
        exit 1
    fi
)

log "Building local smoke-test binary"
(
    cd "${ROOT_DIR}"
    CGO_ENABLED=0 go build -trimpath -o "${TEST_BINARY}" ./cmd/baby-tracker
)

rm -rf "${SMOKE_DIR}"
mkdir -p "${SMOKE_DIR}/app/server" "${SMOKE_DIR}/var" "${SMOKE_DIR}/etc" "${SMOKE_DIR}/tmp"
ln -s "${SOURCE_DIR}/templates" "${SMOKE_DIR}/app/server/templates"
ln -s "${SOURCE_DIR}/static" "${SMOKE_DIR}/app/server/static"
export TRIM_APPDEST="${SMOKE_DIR}/app"
export TRIM_PKGVAR="${SMOKE_DIR}/var"
export TRIM_PKGETC="${SMOKE_DIR}/etc"
export TRIM_PKGTMP="${SMOKE_DIR}/tmp"
export TRIM_DATA_SHARE_PATHS="${SMOKE_DATA_DIR}"
export BABY_TRACKER_BINARY="${TEST_BINARY}"
export BABY_TRACKER_CREATE_DEFAULT_ADMIN=1
export wizard_bind_address="127.0.0.1"
export wizard_port="${SMOKE_PORT}"
source "${PACK_DIR}/cmd/main"

log "Starting Go service through the fnOS launcher"
start_process
SERVER_PID="$(cat "${TRIM_PKGVAR}/app.pid")"

ready=0
for _ in $(seq 1 40); do
    if curl -fsS "http://127.0.0.1:${SMOKE_PORT}/login" >/dev/null 2>&1; then
        ready=1
        break
    fi
    sleep 0.25
done
if [ "${ready}" != "1" ]; then
    cat "${SMOKE_LOG}" >&2
    exit 1
fi

login_page="$(curl -fsS "http://127.0.0.1:${SMOKE_PORT}/login")"
grep -q '<title>登录 - Baby Tracker' <<< "${login_page}"
if grep -q 'gateway-login-button' <<< "${login_page}"; then
    printf '[test] ERROR: direct login page exposes the gateway-only button.\n' >&2
    exit 1
fi

curl -fsS -c "${SMOKE_COOKIE}" \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
    "http://127.0.0.1:${SMOKE_PORT}/api/auth/login" >/dev/null
curl -fsS -b "${SMOKE_COOKIE}" -c "${SMOKE_COOKIE}" \
    -H 'Content-Type: application/json' \
    -d '{"password":"ChangedAdmin456!","confirm_password":"ChangedAdmin456!"}' \
    "http://127.0.0.1:${SMOKE_PORT}/api/auth/setup-password" >/dev/null

curl -fsS -b "${SMOKE_COOKIE}" "http://127.0.0.1:${SMOKE_PORT}/admin" -o "${SMOKE_DIR}/admin.html"
[ "$(grep -c '<details class="admin-details">' "${SMOKE_DIR}/admin.html")" = "8" ]
curl -fsS -b "${SMOKE_COOKIE}" -D "${SMOKE_DIR}/csv.headers" \
    "http://127.0.0.1:${SMOKE_PORT}/api/export/csv" -o "${SMOKE_DIR}/direct.csv"
grep -qi '^content-disposition: attachment;' "${SMOKE_DIR}/csv.headers"
[ "$(od -An -tx1 -N3 "${SMOKE_DIR}/direct.csv" | tr -d '[:space:]')" = "efbbbf" ]

curl -fsS -b "${SMOKE_COOKIE}" -D "${SMOKE_DIR}/backup.headers" \
    "http://127.0.0.1:${SMOKE_PORT}/api/backup/export" -o "${SMOKE_DIR}/direct-backup.json"
grep -qi '^content-disposition: attachment;' "${SMOKE_DIR}/backup.headers"
[ "$(jq -r '.version' "${SMOKE_DIR}/direct-backup.json")" = "1" ]

log "Testing gateway-prefixed downloads and HTML rewriting"
curl -fsS --unix-socket "${SMOKE_SOCKET}" -c "${SMOKE_DIR}/gateway.cookies" \
    -H 'X-Trim-Userid: 1000' \
    -H 'X-Trim-Username: fnosadmin' \
    -H 'X-Trim-Isadmin: true' \
    "http://localhost/app/baby-tracker/admin" -o "${SMOKE_DIR}/gateway-admin.html"
[ "$(grep -c '<details class="admin-details">' "${SMOKE_DIR}/gateway-admin.html")" = "8" ]
if grep -q '/app/baby-tracker/app/baby-tracker' "${SMOKE_DIR}/gateway-admin.html"; then
    printf '[test] ERROR: gateway-prefixed HTML was rewritten twice.\n' >&2
    exit 1
fi
curl -fsS --unix-socket "${SMOKE_SOCKET}" -b "${SMOKE_DIR}/gateway.cookies" \
    "http://localhost/app/baby-tracker/api/export/csv" -o "${SMOKE_DIR}/gateway.csv"
[ "$(od -An -tx1 -N3 "${SMOKE_DIR}/gateway.csv" | tr -d '[:space:]')" = "efbbbf" ]
curl -fsS --unix-socket "${SMOKE_SOCKET}" -b "${SMOKE_DIR}/gateway.cookies" \
    "http://localhost/app/baby-tracker/api/backup/export" -o "${SMOKE_DIR}/gateway-backup.json"
[ "$(jq -r '.version' "${SMOKE_DIR}/gateway-backup.json")" = "1" ]

spoof_status="$(curl -sS -o /dev/null -w '%{http_code}' \
    -H 'X-Trim-Userid: 1000' \
    -H 'X-Trim-Username: fnosadmin' \
    -H 'X-Trim-Isadmin: true' \
    "http://127.0.0.1:${SMOKE_PORT}/app/baby-tracker/api/records")"
[ "${spoof_status}" = "401" ]

stop_process
SERVER_PID=""
trap - EXIT

log "Go tests and service smoke test passed"
