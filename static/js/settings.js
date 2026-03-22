// settings.js — Step 6: Settings, workspace files, tools refresh

async function loadSettings() {
  if (systemData.picoclaw_version) {
    document.getElementById('settings-picoclaw-ver').textContent = systemData.picoclaw_version;
  }
  checkWorkspaceFiles();
}

async function checkWorkspaceFiles() {
  const r = await fetch('/api/system-check');
  const data = await r.json();

  const files = [
    ['soul',      data.has_soul],
    ['identity',  null],
    ['agents',    null],
    ['tools',     null],
    ['heartbeat', null],
    ['user',      null],
    ['memory',    null],
  ];

  files.forEach(([name, known]) => {
    const el = document.getElementById(`ws-${name}-badge`);
    if (!el) return;
    const present = known !== null ? known : data.has_soul;
    el.className = `badge ${present ? 'ok' : 'warn'}`;
    el.textContent = present ? '✓ Found' : '○ Missing';
  });
}

async function refreshTools() {
  const btn = event.target;
  btn.innerHTML = '<div class="spinner" style="display:inline-block;width:12px;height:12px;border-width:2px;margin-right:6px"></div> Refreshing...';
  btn.disabled = true;
  const r = await fetch('/api/tools/refresh', { method: 'POST' });
  const data = await r.json();
  btn.innerHTML = '↻ Refresh Tools Registry'; btn.disabled = false;
  showAlert('uninstall-alert', data.ok ? 'success' : 'error', data.message);
}
