# IPv6 Proxy Manager

Ubuntu VPS-এ routed IPv6 prefix ব্যবহার করে 3proxy SOCKS5 proxies তৈরি ও পরিচালনার জন্য ছোট, database-free dashboard।

## কী পাওয়া যাবে

- GitHub Release থেকে one-command installation
- প্রতিবার installer চালালে নতুন secret dashboard URL; আগের URL সঙ্গে সঙ্গে invalid
- কোনো username/password login form নেই
- Random অথবা Custom proxy credentials
- আগের port ও IPv6-এর পর থেকে আরও proxy তৈরি
- Total, Live, Disabled, Failed ও Checking summary
- সম্পূর্ণ `HOST:PORT:USERNAME:PASSWORD` text list, Copy ও TXT download
- full proxy lines paste করে Disable, Enable বা permanent Delete
- `/root/proxies.txt` এবং `/root/proxies-live.txt`
- PDF/manual setup থেকে existing proxy import
- reboot-এর সময় IPv6/config স্বয়ংক্রিয় restore
- One-click Repair All এবং failed config update rollback
- MySQL/PostgreSQL/Docker/paid API প্রয়োজন নেই

## VPS requirements

- Ubuntu 22.04, 24.04 অথবা 26.04
- amd64 অথবা arm64 CPU
- root access
- সরাসরি assigned public IPv4
- provider-routed multi-address IPv6 prefix (সাধারণত `/64`, `/56` বা `/48`)
- inbound TCP `80`, `443` এবং proxy ports provider firewall-এ allowed
- provider-এর policy অনুযায়ী proxy চালানোর অনুমতি

একটি `/64` route দেখা গেলেই tool Supported বলবে না। Prefix থেকে অস্থায়ী আলাদা IPv6 bind করে outbound Internet test সফল হতে হবে। শুধু `/128` বা provider source-filter করলে installation পরিষ্কার Unsupported error দিয়ে থামবে।

## Installation

GitHub-এ প্রথম release publish হওয়ার পর:

```bash
curl -fsSL https://github.com/bithi78328-spec/ipv6-proxy-manager/releases/latest/download/install.sh | sudo bash
```

শেষে পাওয়া secret HTTPS URL সরাসরি browser-এ খুলবে:

```text
https://VPS_IPV4/p/LONG_RANDOM_SECRET/
```

কোনো login prompt নেই। URL-টি যার কাছে থাকবে সে dashboard নিয়ন্ত্রণ করতে পারবে, তাই link private রাখতে হবে। Nginx access logging বন্ধ রাখা হয়েছে যাতে secret path access log-এ না থাকে।

## একই VPS-এ installer আবার চালালে

- saved proxies বা credentials মুছবে না
- existing state/config পরীক্ষা ও repair করবে
- application/service files refresh করবে
- নতুন secret URL তৈরি করবে
- আগের dashboard URL invalid করবে

পরেরবার proxy বাড়াতে installer চালানো প্রয়োজন নেই। আগের dashboard খুলে **নতুন Proxy তৈরি / আরও Proxy যোগ** ব্যবহার করতে হবে। URL হারালে একই installer আবার চালালে repair করে নতুন URL পাওয়া যাবে।

## Existing manual VPS

প্রথম installation-এ tool নিচের data import করার চেষ্টা করে:

- `/root/proxies.txt`
- `/usr/local/3proxy/conf/3proxy.cfg`
- `/etc/3proxy/3proxy.cfg`
- `/etc/3proxy/conf/3proxy.cfg`

Original config installation-এর আগে `/var/lib/ipv6-proxy-manager/import/3proxy.cfg`-এ copy করা হয়। Import কেবল matching `socks -p... -i... -e...` mapping পেলে সম্পন্ন হয়; অনুমান করে ভুল mapping তৈরি করা হয় না।

## Runtime files

```text
/usr/local/bin/proxy-manager
/var/lib/ipv6-proxy-manager/state.json
/var/lib/ipv6-proxy-manager/state.json.backup
/etc/ipv6-proxy-manager/3proxy.cfg
/root/proxies.txt
/root/proxies-live.txt
```

State ও credential files permission `0600`। State write atomic এবং আগের copy backup হিসেবে থাকে।

## Recovery

Dashboard-এর **One-click Repair All**:

1. saved enabled IPv6 addresses পুনরায় bind করে
2. 3proxy config সম্পূর্ণ regenerate করে
3. proxy engine restart ও active status যাচাই করে
4. text lists পুনর্গঠন করে
5. health check শুরু করে
6. restart ব্যর্থ হলে আগের working config restore করে

Dashboard/service package নিজেই অনুপস্থিত বা নষ্ট হলে একই GitHub installer পুনরায় চালাতে হবে। Provider routing, provider firewall, account suspension বা datacenter outage VPS-এর ভেতরের software ঠিক করতে পারে না; tool সেগুলোকে success না দেখিয়ে নির্দিষ্ট failure হিসেবে জানায়।

## Development and tests

```bash
go test ./...
go vet ./...
shellcheck -x install.sh
```

GitHub Actions:

- race-enabled Go tests চালায়
- installer ShellCheck চালায়
- official verified 3proxy package দিয়ে generated config startup পরীক্ষা করে
- Linux amd64 ও arm64 static binaries তৈরি করে
- binary SHA-256 files তৈরি করে
- `main` branch-এর প্রতিটি verified update-এ versioned GitHub Release publish করে

ইচ্ছা করলে semantic tag দিয়েও Release তৈরি করা যায়:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Installer-এর `__REPOSITORY__` placeholder GitHub Actions release asset বানানোর সময় actual `OWNER/REPO` দিয়ে প্রতিস্থাপিত হয়।

## Security model

ব্যবহারকারীর অনুরোধ অনুযায়ী আলাদা dashboard login নেই। Random 256-bit URL token-ই access key। প্রতিবার installer run-এ এটি rotate হয়। HTTPS-এর জন্য Let’s Encrypt short-lived IP certificate এবং automatic renewal ব্যবহার করা হয়।

## Cost

Software components free/open-source। VPS-এর নিজের মূল্য ছাড়া domain, database, hosting বা AI API লাগে না।
