# IPv6 Proxy Manager

Ubuntu VPS-à¦ routed IPv6 prefix à¦¬à§à¦¯à¦¬à¦¹à¦¾à¦° à¦•à¦°à§‡ 3proxy SOCKS5 proxies à¦¤à§ˆà¦°à¦¿ à¦“ à¦ªà¦°à¦¿à¦šà¦¾à¦²à¦¨à¦¾à¦° à¦œà¦¨à§à¦¯ à¦›à§‹à¦Ÿ, database-free dashboardà¥¤

## à¦•à§€ à¦ªà¦¾à¦“à§Ÿà¦¾ à¦¯à¦¾à¦¬à§‡

- GitHub Release à¦¥à§‡à¦•à§‡ one-command installation
- à¦ªà§à¦°à¦¤à¦¿à¦¬à¦¾à¦° installer à¦šà¦¾à¦²à¦¾à¦²à§‡ à¦¨à¦¤à§à¦¨ secret dashboard URL; à¦†à¦—à§‡à¦° URL à¦¸à¦™à§à¦—à§‡ à¦¸à¦™à§à¦—à§‡ invalid
- à¦•à§‹à¦¨à§‹ username/password login form à¦¨à§‡à¦‡
- Random à¦…à¦¥à¦¬à¦¾ Custom proxy credentials
- à¦†à¦—à§‡à¦° port à¦“ IPv6-à¦à¦° à¦ªà¦° à¦¥à§‡à¦•à§‡ à¦†à¦°à¦“ proxy à¦¤à§ˆà¦°à¦¿
- Total, Live, Disabled, Failed à¦“ Checking summary
- à¦¸à¦®à§à¦ªà§‚à¦°à§à¦£ `HOST:PORT:USERNAME:PASSWORD` text list, Copy à¦“ TXT download
- full proxy lines paste à¦•à¦°à§‡ Disable, Enable à¦¬à¦¾ permanent Delete
- `/root/proxies.txt` à¦à¦¬à¦‚ `/root/proxies-live.txt`
- PDF/manual setup à¦¥à§‡à¦•à§‡ existing proxy import
- reboot-à¦à¦° à¦¸à¦®à§Ÿ IPv6/config à¦¸à§à¦¬à§Ÿà¦‚à¦•à§à¦°à¦¿à§Ÿ restore
- One-click Repair All à¦à¦¬à¦‚ failed config update rollback
- MySQL/PostgreSQL/Docker/paid API à¦ªà§à¦°à§Ÿà§‹à¦œà¦¨ à¦¨à§‡à¦‡

## VPS requirements

- Ubuntu 22.04 à¦¬à¦¾ 24.04
- amd64 à¦…à¦¥à¦¬à¦¾ arm64 CPU
- root access
- à¦¸à¦°à¦¾à¦¸à¦°à¦¿ assigned public IPv4
- provider-routed multi-address IPv6 prefix (à¦¸à¦¾à¦§à¦¾à¦°à¦£à¦¤ `/64`, `/56` à¦¬à¦¾ `/48`)
- inbound TCP `80`, `443` à¦à¦¬à¦‚ proxy ports provider firewall-à¦ allowed
- provider-à¦à¦° policy à¦…à¦¨à§à¦¯à¦¾à§Ÿà§€ proxy à¦šà¦¾à¦²à¦¾à¦¨à§‹à¦° à¦…à¦¨à§à¦®à¦¤à¦¿

à¦à¦•à¦Ÿà¦¿ `/64` route à¦¦à§‡à¦–à¦¾ à¦—à§‡à¦²à§‡à¦‡ tool Supported à¦¬à¦²à¦¬à§‡ à¦¨à¦¾à¥¤ Prefix à¦¥à§‡à¦•à§‡ à¦…à¦¸à§à¦¥à¦¾à§Ÿà§€ à¦†à¦²à¦¾à¦¦à¦¾ IPv6 bind à¦•à¦°à§‡ outbound Internet test à¦¸à¦«à¦² à¦¹à¦¤à§‡ à¦¹à¦¬à§‡à¥¤ à¦¶à§à¦§à§ `/128` à¦¬à¦¾ provider source-filter à¦•à¦°à¦²à§‡ installation à¦ªà¦°à¦¿à¦·à§à¦•à¦¾à¦° Unsupported error à¦¦à¦¿à§Ÿà§‡ à¦¥à¦¾à¦®à¦¬à§‡à¥¤

## Installation

GitHub-à¦ à¦ªà§à¦°à¦¥à¦® release publish à¦¹à¦“à§Ÿà¦¾à¦° à¦ªà¦°:

```bash
curl -fsSL https://github.com/bithi78328-spec/ipv6-proxy-manager/releases/latest/download/install.sh | sudo bash
```

à¦¶à§‡à¦·à§‡ à¦ªà¦¾à¦“à§Ÿà¦¾ secret HTTPS URL à¦¸à¦°à¦¾à¦¸à¦°à¦¿ browser-à¦ à¦–à§à¦²à¦¬à§‡:

```text
https://VPS_IPV4/p/LONG_RANDOM_SECRET/
```

à¦•à§‹à¦¨à§‹ login prompt à¦¨à§‡à¦‡à¥¤ URL-à¦Ÿà¦¿ à¦¯à¦¾à¦° à¦•à¦¾à¦›à§‡ à¦¥à¦¾à¦•à¦¬à§‡ à¦¸à§‡ dashboard à¦¨à¦¿à§Ÿà¦¨à§à¦¤à§à¦°à¦£ à¦•à¦°à¦¤à§‡ à¦ªà¦¾à¦°à¦¬à§‡, à¦¤à¦¾à¦‡ link private à¦°à¦¾à¦–à¦¤à§‡ à¦¹à¦¬à§‡à¥¤ Nginx access logging à¦¬à¦¨à§à¦§ à¦°à¦¾à¦–à¦¾ à¦¹à§Ÿà§‡à¦›à§‡ à¦¯à¦¾à¦¤à§‡ secret path access log-à¦ à¦¨à¦¾ à¦¥à¦¾à¦•à§‡à¥¤

## à¦à¦•à¦‡ VPS-à¦ installer à¦†à¦¬à¦¾à¦° à¦šà¦¾à¦²à¦¾à¦²à§‡

- saved proxies à¦¬à¦¾ credentials à¦®à§à¦›à¦¬à§‡ à¦¨à¦¾
- existing state/config à¦ªà¦°à§€à¦•à§à¦·à¦¾ à¦“ repair à¦•à¦°à¦¬à§‡
- application/service files refresh à¦•à¦°à¦¬à§‡
- à¦¨à¦¤à§à¦¨ secret URL à¦¤à§ˆà¦°à¦¿ à¦•à¦°à¦¬à§‡
- à¦†à¦—à§‡à¦° dashboard URL invalid à¦•à¦°à¦¬à§‡

à¦ªà¦°à§‡à¦°à¦¬à¦¾à¦° proxy à¦¬à¦¾à§œà¦¾à¦¤à§‡ installer à¦šà¦¾à¦²à¦¾à¦¨à§‹ à¦ªà§à¦°à§Ÿà§‹à¦œà¦¨ à¦¨à§‡à¦‡à¥¤ à¦†à¦—à§‡à¦° dashboard à¦–à§à¦²à§‡ **à¦¨à¦¤à§à¦¨ Proxy à¦¤à§ˆà¦°à¦¿ / à¦†à¦°à¦“ Proxy à¦¯à§‹à¦—** à¦¬à§à¦¯à¦¬à¦¹à¦¾à¦° à¦•à¦°à¦¤à§‡ à¦¹à¦¬à§‡à¥¤ URL à¦¹à¦¾à¦°à¦¾à¦²à§‡ à¦à¦•à¦‡ installer à¦†à¦¬à¦¾à¦° à¦šà¦¾à¦²à¦¾à¦²à§‡ repair à¦•à¦°à§‡ à¦¨à¦¤à§à¦¨ URL à¦ªà¦¾à¦“à§Ÿà¦¾ à¦¯à¦¾à¦¬à§‡à¥¤

## Existing manual VPS

à¦ªà§à¦°à¦¥à¦® installation-à¦ tool à¦¨à¦¿à¦šà§‡à¦° data import à¦•à¦°à¦¾à¦° à¦šà§‡à¦·à§à¦Ÿà¦¾ à¦•à¦°à§‡:

- `/root/proxies.txt`
- `/usr/local/3proxy/conf/3proxy.cfg`
- `/etc/3proxy/3proxy.cfg`
- `/etc/3proxy/conf/3proxy.cfg`

Original config installation-à¦à¦° à¦†à¦—à§‡ `/var/lib/ipv6-proxy-manager/import/3proxy.cfg`-à¦ copy à¦•à¦°à¦¾ à¦¹à§Ÿà¥¤ Import à¦•à§‡à¦¬à¦² matching `socks -p... -i... -e...` mapping à¦ªà§‡à¦²à§‡ à¦¸à¦®à§à¦ªà¦¨à§à¦¨ à¦¹à§Ÿ; à¦…à¦¨à§à¦®à¦¾à¦¨ à¦•à¦°à§‡ à¦­à§à¦² mapping à¦¤à§ˆà¦°à¦¿ à¦•à¦°à¦¾ à¦¹à§Ÿ à¦¨à¦¾à¥¤

## Runtime files

```text
/usr/local/bin/proxy-manager
/var/lib/ipv6-proxy-manager/state.json
/var/lib/ipv6-proxy-manager/state.json.backup
/etc/ipv6-proxy-manager/3proxy.cfg
/root/proxies.txt
/root/proxies-live.txt
```

State à¦“ credential files permission `0600`à¥¤ State write atomic à¦à¦¬à¦‚ à¦†à¦—à§‡à¦° copy backup à¦¹à¦¿à¦¸à§‡à¦¬à§‡ à¦¥à¦¾à¦•à§‡à¥¤

## Recovery

Dashboard-à¦à¦° **One-click Repair All**:

1. saved enabled IPv6 addresses à¦ªà§à¦¨à¦°à¦¾à§Ÿ bind à¦•à¦°à§‡
2. 3proxy config à¦¸à¦®à§à¦ªà§‚à¦°à§à¦£ regenerate à¦•à¦°à§‡
3. proxy engine restart à¦“ active status à¦¯à¦¾à¦šà¦¾à¦‡ à¦•à¦°à§‡
4. text lists à¦ªà§à¦¨à¦°à§à¦—à¦ à¦¨ à¦•à¦°à§‡
5. health check à¦¶à§à¦°à§ à¦•à¦°à§‡
6. restart à¦¬à§à¦¯à¦°à§à¦¥ à¦¹à¦²à§‡ à¦†à¦—à§‡à¦° working config restore à¦•à¦°à§‡

Dashboard/service package à¦¨à¦¿à¦œà§‡à¦‡ à¦…à¦¨à§à¦ªà¦¸à§à¦¥à¦¿à¦¤ à¦¬à¦¾ à¦¨à¦·à§à¦Ÿ à¦¹à¦²à§‡ à¦à¦•à¦‡ GitHub installer à¦ªà§à¦¨à¦°à¦¾à§Ÿ à¦šà¦¾à¦²à¦¾à¦¤à§‡ à¦¹à¦¬à§‡à¥¤ Provider routing, provider firewall, account suspension à¦¬à¦¾ datacenter outage VPS-à¦à¦° à¦­à§‡à¦¤à¦°à§‡à¦° software à¦ à¦¿à¦• à¦•à¦°à¦¤à§‡ à¦ªà¦¾à¦°à§‡ à¦¨à¦¾; tool à¦¸à§‡à¦—à§à¦²à§‹à¦•à§‡ success à¦¨à¦¾ à¦¦à§‡à¦–à¦¿à§Ÿà§‡ à¦¨à¦¿à¦°à§à¦¦à¦¿à¦·à§à¦Ÿ failure à¦¹à¦¿à¦¸à§‡à¦¬à§‡ à¦œà¦¾à¦¨à¦¾à§Ÿà¥¤

## Development and tests

```bash
go test ./...
go vet ./...
shellcheck -x install.sh
```

GitHub Actions:

- race-enabled Go tests à¦šà¦¾à¦²à¦¾à§Ÿ
- installer ShellCheck à¦šà¦¾à¦²à¦¾à§Ÿ
- official verified 3proxy package à¦¦à¦¿à§Ÿà§‡ generated config startup à¦ªà¦°à§€à¦•à§à¦·à¦¾ à¦•à¦°à§‡
- Linux amd64 à¦“ arm64 static binaries à¦¤à§ˆà¦°à¦¿ à¦•à¦°à§‡
- binary SHA-256 files à¦¤à§ˆà¦°à¦¿ à¦•à¦°à§‡
- `main` branch-à¦à¦° à¦ªà§à¦°à¦¤à¦¿à¦Ÿà¦¿ verified update-à¦ versioned GitHub Release publish à¦•à¦°à§‡

à¦‡à¦šà§à¦›à¦¾ à¦•à¦°à¦²à§‡ semantic tag à¦¦à¦¿à§Ÿà§‡à¦“ Release à¦¤à§ˆà¦°à¦¿ à¦•à¦°à¦¾ à¦¯à¦¾à§Ÿ:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Installer-à¦à¦° `__REPOSITORY__` placeholder GitHub Actions release asset à¦¬à¦¾à¦¨à¦¾à¦¨à§‹à¦° à¦¸à¦®à§Ÿ actual `OWNER/REPO` à¦¦à¦¿à§Ÿà§‡ à¦ªà§à¦°à¦¤à¦¿à¦¸à§à¦¥à¦¾à¦ªà¦¿à¦¤ à¦¹à§Ÿà¥¤

## Security model

à¦¬à§à¦¯à¦¬à¦¹à¦¾à¦°à¦•à¦¾à¦°à§€à¦° à¦…à¦¨à§à¦°à§‹à¦§ à¦…à¦¨à§à¦¯à¦¾à§Ÿà§€ à¦†à¦²à¦¾à¦¦à¦¾ dashboard login à¦¨à§‡à¦‡à¥¤ Random 256-bit URL token-à¦‡ access keyà¥¤ à¦ªà§à¦°à¦¤à¦¿à¦¬à¦¾à¦° installer run-à¦ à¦à¦Ÿà¦¿ rotate à¦¹à§Ÿà¥¤ HTTPS-à¦à¦° à¦œà¦¨à§à¦¯ Letâ€™s Encrypt short-lived IP certificate à¦à¦¬à¦‚ automatic renewal à¦¬à§à¦¯à¦¬à¦¹à¦¾à¦° à¦•à¦°à¦¾ à¦¹à§Ÿà¥¤

## Cost

Software components free/open-sourceà¥¤ VPS-à¦à¦° à¦¨à¦¿à¦œà§‡à¦° à¦®à§‚à¦²à§à¦¯ à¦›à¦¾à§œà¦¾ domain, database, hosting à¦¬à¦¾ AI API à¦²à¦¾à¦—à§‡ à¦¨à¦¾à¥¤

