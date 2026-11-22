#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="${ROOT_DIR}/source"
PACK_DIR="${ROOT_DIR}/packaging/baby-tracker"
ASSETS_DIR="${ROOT_DIR}/packaging/build-assets"
SERVER_DIR="${PACK_DIR}/app/server"
BIN_DIR="${SERVER_DIR}/bin"
TOOLS_DIR="${ROOT_DIR}/tools"
DIST_DIR="${ROOT_DIR}/dist"
FNPACK_VERSION="1.2.3"
REPO_URL="https://github.com/FNOSP/Baby_tracker-fnos.git"

log() {
    printf '[build] %s\n' "$1"
}

die() {
    printf '[build] ERROR: %s\n' "$1" >&2
    exit 1
}

ensure_fnpack() {
    if [ -n "${FNPACK_BIN:-}" ]; then
        [ -x "${FNPACK_BIN}" ] || die "FNPACK_BIN is not executable: ${FNPACK_BIN}"
        printf '%s' "${FNPACK_BIN}"
        return 0
    fi

    local os arch name url target
    os="$(uname -s)"
    arch="$(uname -m)"

    case "${os}:${arch}" in
        Darwin:arm64) name="fnpack-${FNPACK_VERSION}-darwin-arm64" ;;
        Darwin:x86_64) name="fnpack-${FNPACK_VERSION}-darwin-amd64" ;;
        Linux:aarch64|Linux:arm64) name="fnpack-${FNPACK_VERSION}-linux-arm64" ;;
        Linux:x86_64|Linux:amd64) name="fnpack-${FNPACK_VERSION}-linux-amd64" ;;
        *) die "Unsupported build host: ${os} ${arch}" ;;
    esac

    target="${TOOLS_DIR}/${name}"
    if [ ! -x "${target}" ]; then
        url="https://static2.fnnas.com/fnpack/${name}"
        printf '[build] Downloading %s\n' "${name}" >&2
        mkdir -p "${TOOLS_DIR}"
        curl -fL --retry 3 "${url}" -o "${target}"
        chmod +x "${target}"
    fi
    printf '%s' "${target}"
}

stage_payload() {
    log "Staging web assets"
    rm -rf "${SERVER_DIR}"
    mkdir -p "${SERVER_DIR}"

    cp "${SOURCE_DIR}/LICENSE" "${SERVER_DIR}/LICENSE"
    cp "${ASSETS_DIR}/THIRD_PARTY_NOTICES.md" "${SERVER_DIR}/THIRD_PARTY_NOTICES.md"
    cp -R "${SOURCE_DIR}/templates" "${SERVER_DIR}/templates"
    cp -R "${SOURCE_DIR}/static" "${SERVER_DIR}/static"
    mkdir -p "${SERVER_DIR}/static/vendor"
    cp "${ASSETS_DIR}/vendor-tailwindcss.min.js" "${SERVER_DIR}/static/vendor/tailwindcss.min.js"
    cp "${ASSETS_DIR}/vendor-lucide.min.js" "${SERVER_DIR}/static/vendor/lucide.min.js"
    cp "${ASSETS_DIR}/vendor-chart.js-4.4.7.min.js" "${SERVER_DIR}/static/vendor/chart.js-4.4.7.min.js"

    perl -0pi -e 's#https://cdn\.tailwindcss\.com#/static/vendor/tailwindcss.min.js#g; s#https://unpkg\.com/lucide\@0\.460\.0#/static/vendor/lucide.min.js#g' "${SERVER_DIR}/templates/base.html"
    perl -0pi -e 's#https://cdn\.jsdelivr\.net/npm/chart\.js\@4\.4\.7#/static/vendor/chart.js-4.4.7.min.js#g' "${SERVER_DIR}/templates/trends.html"
    perl -ni -e 'print unless /fonts\.(?:googleapis|gstatic)\.com/' "${SERVER_DIR}/templates/base.html"
    perl -ni -e 'print unless /<link rel="dns-prefetch"/' "${SERVER_DIR}/templates/base.html"

    mkdir -p "${PACK_DIR}/app/ui/images"
    cp "${ASSETS_DIR}/icons/icon_256.png" "${PACK_DIR}/ICON_256.PNG"
    cp "${ASSETS_DIR}/icons/icon_64.png" "${PACK_DIR}/ICON.PNG"
    cp "${ASSETS_DIR}/icons/icon_256.png" "${PACK_DIR}/app/ui/images/icon_256.png"
    cp "${ASSETS_DIR}/icons/icon_64.png" "${PACK_DIR}/app/ui/images/icon_64.png"
}

build_binaries() {
    local arch output
    command -v go >/dev/null 2>&1 || die "Go is required to build Baby Tracker."
    mkdir -p "${BIN_DIR}"
    for arch in amd64 arm64; do
        output="${BIN_DIR}/baby-tracker-linux-${arch}"
        log "Building linux/${arch}"
        (
            cd "${ROOT_DIR}"
            CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" \
                go build -trimpath -ldflags="-s -w" -o "${output}" ./cmd/baby-tracker
        )
    done
    chmod 0755 "${BIN_DIR}/baby-tracker-linux-amd64" "${BIN_DIR}/baby-tracker-linux-arm64"
}

validate_package() {
    log "Validating package files"
    find "${PACK_DIR}/app" -name '.DS_Store' -type f -delete
    [ -s "${PACK_DIR}/manifest" ] || die "manifest is missing."
    stable_desc='<p>Baby Tracker 用于记录新生儿与婴幼儿的喂养、排泄、成长和健康数据，所有数据保存在飞牛本地。</p><p>应用支持 Home Assistant 集成：在“管理 > 管理配置”生成 API 密钥，然后在“Home Assistant 集成”中配置服务地址并生成 YAML。</p><p><strong>重置管理密码：</strong>从飞牛桌面重新进入应用，通过统一网关一键登录，在“管理 > 管理配置”点击“修改管理密码”。</p>'
    grep -Fq "desc                  = ${stable_desc}" "${PACK_DIR}/manifest" || die "Manifest description must remain the stable product introduction; put release notes in CHANGELOG.md."
    [ -s "${ROOT_DIR}/CHANGELOG.md" ] || die "CHANGELOG.md is missing."
    grep -q '^changelog[[:space:]]*=' "${PACK_DIR}/manifest" || die "Manifest changelog summary is missing."
    if grep -q 'install_dep_apps' "${PACK_DIR}/manifest"; then
        die "Manifest still requires an external runtime."
    fi
    if grep -q 'reset-password.txt' "${PACK_DIR}/manifest"; then
        die "Manifest description still contains the legacy reset-password flow."
    fi

    [ -s "${SERVER_DIR}/bin/baby-tracker-linux-amd64" ] || die "linux/amd64 binary is missing."
    [ -s "${SERVER_DIR}/bin/baby-tracker-linux-arm64" ] || die "linux/arm64 binary is missing."
    [ -x "${SERVER_DIR}/bin/baby-tracker-linux-amd64" ] || die "linux/amd64 binary is not executable."
    [ -x "${SERVER_DIR}/bin/baby-tracker-linux-arm64" ] || die "linux/arm64 binary is not executable."
    if [ -e "${SERVER_DIR}/wsgi.py" ] || [ -e "${SERVER_DIR}/bootstrap.py" ] || [ -e "${SERVER_DIR}/wheelhouse" ]; then
        die "Legacy Python runtime files are still packaged."
    fi
    [ -s "${SERVER_DIR}/templates/setup-password.html" ] || die "First-login password setup page is missing."
    [ -s "${SERVER_DIR}/static/js/motion.js" ] || die "Shared motion layer is missing."
    [ -s "${SERVER_DIR}/static/vendor/tailwindcss.min.js" ] || die "Tailwind runtime is missing."
    [ -s "${SERVER_DIR}/static/vendor/lucide.min.js" ] || die "Lucide runtime is missing."
    [ -s "${SERVER_DIR}/static/vendor/chart.js-4.4.7.min.js" ] || die "Chart.js runtime is missing."
    [ -s "${SERVER_DIR}/static/icons/icon-180.png" ] || die "PWA 180px icon is missing."
    [ -s "${SERVER_DIR}/static/icons/icon-192.png" ] || die "PWA 192px icon is missing."
    [ -s "${SERVER_DIR}/static/icons/icon-512.png" ] || die "PWA 512px icon is missing."
    [ -s "${PACK_DIR}/ICON.PNG" ] || die "ICON.PNG is missing."
    [ -s "${PACK_DIR}/ICON_256.PNG" ] || die "ICON_256.PNG is missing."

    grep -q 'BABY_TRACKER_GATEWAY_SOCKET' "${ROOT_DIR}/internal/app/config.go" || die "Gateway socket configuration is missing."
    grep -q 'X-Trim-Userid' "${ROOT_DIR}/internal/app/auth.go" || die "Gateway identity handling is missing."
    grep -q 'gateway-login' "${ROOT_DIR}/internal/app/routes.go" || die "Gateway sign-in endpoint is missing."
    grep -q 'admin_password_initialized' "${ROOT_DIR}/internal/app/auth.go" || die "Admin password setup state is missing."
    grep -q '/api/auth/setup-password' "${ROOT_DIR}/internal/app/routes.go" || die "Admin password setup endpoint is missing."
    grep -q 'defaultGatewayPrefix' "${ROOT_DIR}/internal/app/config.go" || die "Gateway prefix configuration is missing."
    grep -q 'trustGateway' "${ROOT_DIR}/internal/app/gateway.go" || die "Gateway header trust boundary is missing."
    grep -q '国家免疫规划疫苗儿童免疫程序表（2026年版）' "${ROOT_DIR}/internal/app/handlers_growth.go" || die "2026 vaccine schedule is missing."
    if grep -q '国家免疫规划疫苗儿童免疫程序表（2024年版）' "${ROOT_DIR}/internal/app/handlers_growth.go"; then
        die "Legacy 2024 vaccine schedule is still present."
    fi
    grep -q '"2vHPV"' "${ROOT_DIR}/internal/app/handlers_growth.go" || die "HPV vaccine schedule is missing."

    grep -q '管理配置' "${SERVER_DIR}/templates/admin.html" || die "Admin configuration card is missing."
    grep -q 'id="admin-bind-address"' "${SERVER_DIR}/templates/admin.html" || die "Admin bind-address input is missing."
    grep -q 'id="admin-service-port"' "${SERVER_DIR}/templates/admin.html" || die "Admin service-port input is missing."
    grep -q 'data-save-service-config' "${SERVER_DIR}/templates/admin.html" || die "Admin service-config save control is missing."
    grep -q '<details class="admin-details">' "${SERVER_DIR}/templates/admin.html" || die "Admin collapsible sections are missing."
    if [ "$(grep -c '<details class="admin-details">' "${SERVER_DIR}/templates/admin.html")" != "8" ]; then
        die "Admin page must contain eight collapsible sections."
    fi
    grep -q 'id="quick-buttons-panel"' "${SERVER_DIR}/templates/admin.html" || die "Quick-record collapsible panel is missing."
    grep -q 'id="home-assistant-panel"' "${SERVER_DIR}/templates/admin.html" || die "Home Assistant collapsible panel is missing."
    grep -q 'class="admin-details-header"' "${SERVER_DIR}/templates/admin.html" || die "Native collapsible summary is missing."
    [ "$(grep -c '<span class="admin-details-heading">' "${SERVER_DIR}/templates/admin.html")" = "8" ] || die "Admin collapsible headers are not structurally uniform."
    [ "$(grep -c '<span class="admin-details-description">' "${SERVER_DIR}/templates/admin.html")" = "8" ] || die "Admin collapsible descriptions are missing."
    grep -Fq '.admin-details[open] .admin-details-icon' "${SERVER_DIR}/static/css/style.css" || die "Native collapsible expansion styling is missing."
    grep -Fq '.admin-details-content' "${SERVER_DIR}/static/css/style.css" || die "Unified collapsible content spacing is missing."
    grep -Fq '@media (prefers-reduced-motion: reduce)' "${SERVER_DIR}/static/css/style.css" || die "Reduced-motion styling is missing."
    grep -q '/static/js/motion.js' "${SERVER_DIR}/templates/base.html" || die "Shared motion layer is not loaded."
    grep -q '/static/js/motion.js' "${SERVER_DIR}/static/sw.js" || die "Shared motion layer is not cached by the PWA."
    if grep -q 'data-admin-collapse\|admin-collapse-panel\|toggleAdminCollapse' "${SERVER_DIR}/templates/admin.html" "${SERVER_DIR}/static/css/style.css" "${SERVER_DIR}/static/js/admin.js"; then
        die "Legacy JavaScript-dependent admin collapse code is still packaged."
    fi
    grep -q 'window.location.href = appUrl('"'"'/api/export/csv'"'"')' "${SERVER_DIR}/static/js/admin.js" || die "Gateway-aware CSV download URL is missing."
    grep -q 'window.location.href = appUrl('"'"'/api/backup/export'"'"')' "${SERVER_DIR}/static/js/admin.js" || die "Gateway-aware backup download URL is missing."
    grep -q 'function appUrl' "${SERVER_DIR}/static/js/app.js" || die "Gateway-aware URL helper is missing."
    grep -q 'data-app-base="{{ app_base }}"' "${SERVER_DIR}/templates/base.html" || die "Gateway base path is missing from the page."
    grep -q 'mcpEndpoint' "${ROOT_DIR}/internal/app/mcp.go" || die "MCP endpoint definition is missing."
    grep -q '/api/mcp/api-key' "${ROOT_DIR}/internal/app/routes.go" || die "MCP key management API is missing."
    grep -q 'mcpDirectAccessAllowed' "${ROOT_DIR}/internal/app/mcp.go" || die "MCP direct-access boundary is missing."
    grep -q 'mcpEnabledSetting' "${ROOT_DIR}/internal/app/mcp.go" || die "MCP persisted switch is missing."
    grep -q '/api/mcp/config' "${ROOT_DIR}/internal/app/routes.go" || die "MCP config API is missing."
    grep -q 'id="mcp-service-state"' "${SERVER_DIR}/templates/admin.html" || die "MCP status control is missing."
    grep -q 'role="switch"' "${SERVER_DIR}/templates/admin.html" || die "MCP switch control is missing."
    grep -q 'id="mcp-settings-details"' "${SERVER_DIR}/templates/admin.html" || die "MCP collapsible config is missing."
    grep -q 'data-generate-mcp-key' "${SERVER_DIR}/templates/admin.html" || die "MCP key rotation control is missing."
    grep -q 'function loadMcpApiKey' "${SERVER_DIR}/static/js/admin.js" || die "MCP key loader is missing."
    grep -q 'function toggleMcpService' "${SERVER_DIR}/static/js/admin.js" || die "MCP switch handler is missing."
    grep -q 'function updateMcpServiceStatus' "${SERVER_DIR}/static/js/admin.js" || die "MCP service status synchronization is missing."

    grep -q 'TRIM_DATA_SHARE_PATHS' "${PACK_DIR}/cmd/main" || die "data-share mapping is missing."
    grep -q 'BABY_TRACKER_GATEWAY_SOCKET' "${PACK_DIR}/cmd/main" || die "Gateway Unix Socket is missing."
    grep -q 'baby-tracker-linux-amd64' "${PACK_DIR}/cmd/main" || die "amd64 Go runtime selection is missing."
    grep -q 'baby-tracker-linux-arm64' "${PACK_DIR}/cmd/main" || die "arm64 Go runtime selection is missing."
    grep -q 'validate_service_values' "${PACK_DIR}/cmd/main" || die "Optional service configuration command is missing."
    if grep -q 'gunicorn\|virtualenv\|python312\|wheelhouse\|wsgi:application' "${PACK_DIR}/cmd/main"; then
        die "Launcher still contains the legacy Python runtime."
    fi

    jq empty "${PACK_DIR}/config/privilege"
    jq empty "${PACK_DIR}/config/resource"
    jq -e '."data-share".shares | any(.name == "baby-tracker/data")' "${PACK_DIR}/config/resource" >/dev/null || die "data-share resource is incomplete."
    jq empty "${PACK_DIR}/app/ui/config"
    jq -e '.".url"."baby-tracker.main" | .gatewayPrefix == "/app/baby-tracker" and .gatewaySocket == "app.sock" and .url == "/app/baby-tracker"' "${PACK_DIR}/app/ui/config" >/dev/null || die "Unified gateway desktop entry is incomplete."
    bash -n "${PACK_DIR}/cmd/main"
    bash -n "${PACK_DIR}/cmd/install_callback"
    bash -n "${PACK_DIR}/cmd/upgrade_callback"
    bash -n "${PACK_DIR}/cmd/uninstall_init"
    chmod +x "${PACK_DIR}/cmd/"*
}

main() {
    local fnpack_bin commit
    fnpack_bin="$(ensure_fnpack)"
    stage_payload
    build_binaries
    validate_package

    mkdir -p "${DIST_DIR}"
    commit="$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || printf 'unknown')"
    {
        printf 'repository=%s\n' "${REPO_URL}"
        printf 'commit=%s\n' "${commit}"
        printf 'built_at=%s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
        printf 'fnpack_version=%s\n' "${FNPACK_VERSION}"
        printf 'runtime=go\n'
    } > "${DIST_DIR}/SOURCE_INFO.txt"

    log "Building FPK with fnpack"
    (cd "${ROOT_DIR}" && "${fnpack_bin}" build --directory "${PACK_DIR}")
    rm -f "${ROOT_DIR}/app.tgz"

    local generated
    generated="$(find "${ROOT_DIR}" -maxdepth 1 -type f -name 'baby-tracker*.fpk' -print -quit)"
    if [ -z "${generated}" ]; then
        generated="$(find "${ROOT_DIR}/packaging" -maxdepth 1 -type f -name 'baby-tracker*.fpk' -print -quit)"
    fi
    [ -n "${generated}" ] || die "fnpack completed but no FPK was found."
    mv "${generated}" "${DIST_DIR}/baby-tracker.fpk"
    log "Created ${DIST_DIR}/baby-tracker.fpk"
}

main "$@"
