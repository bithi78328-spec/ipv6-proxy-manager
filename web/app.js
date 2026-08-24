const base = window.IP6PM_BASE;
const $ = (id) => document.getElementById(id);
const output = (message) => { $("output").textContent = typeof message === "string" ? message : JSON.stringify(message, null, 2); };

async function api(path, options = {}) {
  const response = await fetch(`${base}/api/${path}`, {
    ...options,
    headers: {"Content-Type": "application/json", ...(options.headers || {})}
  });
  const type = response.headers.get("content-type") || "";
  const body = type.includes("application/json") ? await response.json() : await response.text();
  if (!response.ok) throw new Error(body.error || body || `HTTP ${response.status}`);
  return body;
}

async function refresh() {
  try {
    const [summary, info] = await Promise.all([api("summary"), api("info")]);
    for (const key of ["total","live","disabled","failed","checking"]) $(key).textContent = summary[key];
    $("environment").textContent = `${info.public_ipv4} • ${info.ipv6_prefix} • ${info.interface}`;
  } catch (error) { output(`Refresh failed: ${error.message}`); }
}

async function busy(button, work) {
  button.disabled = true;
  try { await work(); } catch (error) { output(`Error: ${error.message}`); }
  finally { button.disabled = false; await refresh(); }
}

$("credential-mode").addEventListener("change", (event) => {
  document.querySelectorAll(".custom-field").forEach((item) => item.classList.toggle("hidden", event.target.value !== "custom"));
});
$("refresh").addEventListener("click", refresh);
$("check").addEventListener("click", () => busy($("check"), async () => {
  output(await api("check", {method:"POST", body:"{}"}));
}));
$("repair").addEventListener("click", () => busy($("repair"), async () => {
  if (!confirm("Saved state থেকে IPv6, 3proxy config এবং proxy service আবার তৈরি করা হবে। চালাবেন?")) return;
  output(await api("repair", {method:"POST", body:"{}"}));
}));
$("create").addEventListener("click", () => busy($("create"), async () => {
  const payload = {
    count: Number($("count").value),
    credential_mode: $("credential-mode").value,
    username: $("username").value.trim(),
    password: $("password").value.trim()
  };
  output(await api("create", {method:"POST", body:JSON.stringify(payload)}));
}));

async function loadList() {
  const filter = $("list-filter").value;
  $("proxy-list").value = await api(`list?filter=${encodeURIComponent(filter)}`);
}
$("load-list").addEventListener("click", () => busy($("load-list"), loadList));
$("list-filter").addEventListener("change", loadList);
$("copy-list").addEventListener("click", async () => {
  if (!$("proxy-list").value) await loadList();
  await navigator.clipboard.writeText($("proxy-list").value);
  output("Proxy list copied.");
});
$("download-list").addEventListener("click", () => {
  const filter = $("list-filter").value;
  window.location.href = `${base}/api/download?filter=${encodeURIComponent(filter)}`;
});
$("apply-action").addEventListener("click", () => busy($("apply-action"), async () => {
  const action = $("action").value;
  const text = $("action-list").value.trim();
  if (!text) throw new Error("Proxy list paste করুন।");
  if (!confirm(`${action.toUpperCase()} actionটি চালাবেন?`)) return;
  output(await api("action", {method:"POST", body:JSON.stringify({action, list:text})}));
  await loadList();
}));

refresh();
setInterval(refresh, 5000);

