#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="${IP6PM_REPOSITORY:-__REPOSITORY__}"
SOURCE_PLACEHOLDER="__REPO""SITORY__"
RELEASE_BASE="${IP6PM_RELEASE_BASE:-https://github.com/${REPOSITORY}/releases/latest/download}"
APP=/usr/local/bin/proxy-manager
STATE_DIR=/var/lib/ipv6-proxy-manager
CONFIG_DIR=/etc/ipv6-proxy-manager
LOG=/var/log/ipv6-proxy-manager-install.log
THREEPROXY_VERSION=0.9.5

if [[ ${EUID} -ne 0 ]]; then
  echo "Run this installer as root: curl ... | sudo bash" >&2
  exit 1
fi
if [[ ${REPOSITORY} == "${SOURCE_PLACEHOLDER}" ]]; then
  echo "This source installer has not been packaged by GitHub Releases yet." >&2
  echo "Set IP6PM_REPOSITORY=OWNER/REPO or download install.sh from a release." >&2
  exit 1
fi

mkdir -p "$(dirname "${LOG}")"
exec > >(tee -a "${LOG}") 2>&1
on_error() {
  local code=$?
  echo
  echo "Installation failed at line ${BASH_LINENO[0]} (exit ${code})."
  echo "Log: ${LOG}"
  exit "${code}"
}
trap on_error ERR

step() { echo; echo "==> $*"; }
retry() {
  local attempts=0
  local max=4
  until "$@"; do
    attempts=$((attempts + 1))
    if (( attempts >= max )); then return 1; fi
    echo "Command failed; retrying (${attempts}/${max})..."
    sleep $((attempts * 2))
  done
}

step "Checking the VPS"
[[ -r /etc/os-release ]] || { echo "Unsupported OS: /etc/os-release is missing"; exit 2; }
# shellcheck disable=SC1091
source /etc/os-release
[[ ${ID:-} == ubuntu ]] || { echo "Unsupported OS: Ubuntu 22.04, 24.04, or 26.04 is required"; exit 2; }
case "${VERSION_ID:-}" in
  22.04|24.04|26.04) ;;
  *) echo "Unsupported Ubuntu ${VERSION_ID:-unknown}. Use Ubuntu 22.04, 24.04, or 26.04."; exit 2 ;;
esac
case "$(dpkg --print-architecture)" in
  amd64) ARCH=amd64; THREEPROXY_ASSET="3proxy-${THREEPROXY_VERSION}.x86_64.deb"; THREEPROXY_SHA="5c8c2119c19cb26c7fb3c81a79f8a278f19d39a24189ad5c719479b3414f734a" ;;
  arm64) ARCH=arm64; THREEPROXY_ASSET="3proxy-${THREEPROXY_VERSION}.aarch64.deb"; THREEPROXY_SHA="8652c2a9e1e24ba48b3566396e9c3c3a3644211490c8a7e7cabcc8df1a4681a7" ;;
  *) echo "Unsupported CPU architecture. Only amd64 and arm64 are supported."; exit 2 ;;
esac

if command -v cloud-init >/dev/null 2>&1; then
  step "Waiting for first-boot cloud initialization"
  timeout 600 cloud-init status --wait || {
    echo "cloud-init did not finish successfully within 10 minutes."
    echo "Check it with: cloud-init status --long"
    exit 2
  }
fi

export DEBIAN_FRONTEND=noninteractive
step "Installing base packages"
retry apt-get -o Acquire::Retries=3 update
retry apt-get -o Acquire::Retries=3 install -y ca-certificates curl iproute2 nginx openssl python3 python3-venv

mkdir -p "${STATE_DIR}/import" "${CONFIG_DIR}" /var/www/ipv6-proxy-manager/.well-known/acme-challenge
chmod 700 "${STATE_DIR}" "${CONFIG_DIR}"
for old_config in /usr/local/3proxy/conf/3proxy.cfg /etc/3proxy/3proxy.cfg /etc/3proxy/conf/3proxy.cfg; do
  if [[ -s ${old_config} ]] && grep -qE '^[[:space:]]*socks[[:space:]]' "${old_config}"; then
    cp -a "${old_config}" "${STATE_DIR}/import/3proxy.cfg"
    chmod 600 "${STATE_DIR}/import/3proxy.cfg"
    break
  fi
done

step "Installing IPv6 Proxy Manager"
TMP_DIR=$(mktemp -d /tmp/ipv6-proxy-manager.XXXXXX)
cleanup() {
  if [[ -n ${TMP_DIR:-} && ${TMP_DIR} == /tmp/ipv6-proxy-manager.* && -d ${TMP_DIR} ]]; then
    rm -rf -- "${TMP_DIR}"
  fi
}
trap cleanup EXIT
MANAGER_ASSET="proxy-manager-linux-${ARCH}"
retry curl -fL --retry 3 --connect-timeout 15 -o "${TMP_DIR}/${MANAGER_ASSET}" "${RELEASE_BASE}/${MANAGER_ASSET}"
retry curl -fL --retry 3 --connect-timeout 15 -o "${TMP_DIR}/${MANAGER_ASSET}.sha256" "${RELEASE_BASE}/${MANAGER_ASSET}.sha256"
(cd "${TMP_DIR}" && sha256sum -c "${MANAGER_ASSET}.sha256")
install -m 0755 "${TMP_DIR}/${MANAGER_ASSET}" "${APP}"

step "Installing verified 3proxy ${THREEPROXY_VERSION}"
THREEPROXY_URL="https://github.com/3proxy/3proxy/releases/download/${THREEPROXY_VERSION}/${THREEPROXY_ASSET}"
retry curl -fL --retry 3 --connect-timeout 15 -o "${TMP_DIR}/${THREEPROXY_ASSET}" "${THREEPROXY_URL}"
echo "${THREEPROXY_SHA}  ${TMP_DIR}/${THREEPROXY_ASSET}" | sha256sum -c -
dpkg -i "${TMP_DIR}/${THREEPROXY_ASSET}"
systemctl disable --now 3proxy.service 2>/dev/null || true

step "Detecting IPv4, routed IPv6, and existing proxies"
"${APP}" bootstrap

step "Installing reboot restore and dashboard services"
CURRENT_THREADS_MAX=$(sysctl -n kernel.threads-max)
TARGET_THREADS_MAX=${CURRENT_THREADS_MAX}
if (( TARGET_THREADS_MAX < 20000 )); then
  TARGET_THREADS_MAX=20000
fi
CONNTRACK_MAX=$(sysctl -n net.netfilter.nf_conntrack_max 2>/dev/null || echo 0)
TARGET_CONNTRACK_MAX=${CONNTRACK_MAX}
if (( TARGET_CONNTRACK_MAX < 131072 )); then
  TARGET_CONNTRACK_MAX=131072
fi
printf 'kernel.threads-max = %s\nnet.netfilter.nf_conntrack_max = %s\n' \
  "${TARGET_THREADS_MAX}" "${TARGET_CONNTRACK_MAX}" > /etc/sysctl.d/90-ipv6-proxy-manager.conf
sysctl -w kernel.threads-max="${TARGET_THREADS_MAX}"
sysctl -w net.netfilter.nf_conntrack_max="${TARGET_CONNTRACK_MAX}"
cat > /etc/systemd/system/ipv6-proxy-engine.service <<'UNIT'
[Unit]
Description=IPv6 SOCKS5 Proxy Engine
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStartPre=/usr/sbin/sysctl -w net.netfilter.nf_conntrack_max=131072
ExecStartPre=/usr/local/bin/proxy-manager prepare
ExecStart=/usr/local/bin/proxy-manager engine
Restart=always
RestartSec=3
LimitNOFILE=1048576
LimitNPROC=20000
TasksMax=20000

[Install]
WantedBy=multi-user.target
UNIT
chmod 0644 /etc/systemd/system/ipv6-proxy-engine.service

cat > /etc/systemd/system/ipv6-proxy-manager.service <<'UNIT'
[Unit]
Description=IPv6 Proxy Manager Dashboard
Wants=network-online.target
After=network-online.target ipv6-proxy-engine.service

[Service]
Type=simple
ExecStart=/usr/local/bin/proxy-manager serve
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT
chmod 0644 /etc/systemd/system/ipv6-proxy-manager.service

systemctl daemon-reload
systemctl enable ipv6-proxy-engine.service ipv6-proxy-manager.service
# Keep the dashboard stopped while the installer changes the engine and its
# durable state. It is started after the final repair, at which point it safely
# launches a fresh health scan.
systemctl stop ipv6-proxy-manager.service 2>/dev/null || true
systemctl restart ipv6-proxy-engine.service

PUBLIC_IPV4=$(python3 -c 'import json; print(json.load(open("/var/lib/ipv6-proxy-manager/state.json"))["public_ipv4"])')

step "Preparing public HTTPS dashboard"
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then
  ufw allow 80/tcp comment 'IPv6 Proxy Manager HTTP validation'
  ufw allow 443/tcp comment 'IPv6 Proxy Manager dashboard'
fi
rm -f /etc/nginx/sites-enabled/default
cat > /etc/nginx/sites-available/ipv6-proxy-manager <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${PUBLIC_IPV4};
    access_log off;

    location ^~ /.well-known/acme-challenge/ {
        root /var/www/ipv6-proxy-manager;
        default_type text/plain;
    }
    location / { return 404; }
}
NGINX
ln -sfn /etc/nginx/sites-available/ipv6-proxy-manager /etc/nginx/sites-enabled/ipv6-proxy-manager
nginx -t
systemctl enable --now nginx
systemctl reload nginx

CERTBOT=/opt/ipv6-proxy-manager-certbot/bin/certbot
if [[ ! -x ${CERTBOT} ]]; then
  python3 -m venv /opt/ipv6-proxy-manager-certbot
  /opt/ipv6-proxy-manager-certbot/bin/pip install --disable-pip-version-check --upgrade pip
  /opt/ipv6-proxy-manager-certbot/bin/pip install --disable-pip-version-check 'certbot==5.7.0'
fi

CERT_DIR=/etc/letsencrypt/live/ipv6-proxy-manager
NEED_CERT=1
if [[ -s ${CERT_DIR}/fullchain.pem ]] && openssl x509 -in "${CERT_DIR}/fullchain.pem" -noout -checkip "${PUBLIC_IPV4}" >/dev/null 2>&1; then
  NEED_CERT=0
fi
if (( NEED_CERT )); then
  "${CERTBOT}" certonly --webroot -w /var/www/ipv6-proxy-manager \
    --non-interactive --agree-tos --register-unsafely-without-email \
    --required-profile shortlived --ip-address "${PUBLIC_IPV4}" \
    --cert-name ipv6-proxy-manager
fi

cat > /etc/nginx/sites-available/ipv6-proxy-manager <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${PUBLIC_IPV4};
    access_log off;
    location ^~ /.well-known/acme-challenge/ { root /var/www/ipv6-proxy-manager; }
    location / { return 301 https://\$host\$request_uri; }
}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name ${PUBLIC_IPV4};
    access_log off;
    server_tokens off;
    client_max_body_size 3m;

    ssl_certificate ${CERT_DIR}/fullchain.pem;
    ssl_certificate_key ${CERT_DIR}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_read_timeout 900s;
        proxy_send_timeout 900s;
    }
}
NGINX
nginx -t
systemctl reload nginx

cat > /etc/systemd/system/ipv6-proxy-manager-cert-renew.service <<'UNIT'
[Unit]
Description=Renew IPv6 Proxy Manager IP certificate
After=network-online.target nginx.service

[Service]
Type=oneshot
ExecStart=/opt/ipv6-proxy-manager-certbot/bin/certbot renew --quiet --deploy-hook "systemctl reload nginx"
UNIT
chmod 0644 /etc/systemd/system/ipv6-proxy-manager-cert-renew.service

cat > /etc/systemd/system/ipv6-proxy-manager-cert-renew.timer <<'UNIT'
[Unit]
Description=Renew IPv6 Proxy Manager IP certificate twice daily

[Timer]
OnBootSec=20min
OnUnitActiveSec=12h
RandomizedDelaySec=20min
Persistent=true

[Install]
WantedBy=timers.target
UNIT
chmod 0644 /etc/systemd/system/ipv6-proxy-manager-cert-renew.timer

systemctl daemon-reload
systemctl enable --now ipv6-proxy-manager-cert-renew.timer

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then
  ufw allow 10000:65534/tcp comment 'IPv6 SOCKS5 proxies'
fi

step "Running final repair and rotating the dashboard URL"
"${APP}" repair
"${APP}" rotate-token >/dev/null
systemctl restart ipv6-proxy-manager.service
DASHBOARD_URL=$("${APP}" show-url)

echo
echo "============================================================"
echo " IPv6 Proxy Manager is ready"
echo
echo " Dashboard: ${DASHBOARD_URL}"
echo
echo " This URL opens directly; there is no login form."
echo " Running this installer again repairs the same VPS and"
echo " generates a new URL without deleting saved proxies."
echo "============================================================"

