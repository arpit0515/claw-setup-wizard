// steps.js — Step 0-4 logic: system check, LLM, Telegram, Soul, Launch

const providerModels = {
  openrouter: {
    hint: 'Get your free key at <a href="https://openrouter.ai/keys" target="_blank" style="color:var(--accent2)">openrouter.ai/keys</a>',
    models: [
      ['openrouter/auto',                       '⚡ Auto — let OpenRouter choose best model'],
      ['anthropic/claude-haiku-4-5-20251001',   'Claude Haiku 4.5 (Fast · Recommended for tools)'],
      ['anthropic/claude-sonnet-4-6',           'Claude Sonnet 4.6 (Best quality)'],
      ['google/gemini-flash-1.5',               'Gemini Flash 1.5'],
      ['meta-llama/llama-3.1-8b-instruct:free', 'Llama 3.1 8B (Free)'],
      ['openai/gpt-4o-mini',                    'OpenAI GPT-4o Mini'],
    ]
  },
  anthropic: {
    hint: 'Get your key at <a href="https://console.anthropic.com" target="_blank" style="color:var(--accent2)">console.anthropic.com</a>',
    models: [
      ['claude-haiku-4-5-20251001', 'Claude Haiku 4.5 (Fast)'],
      ['claude-sonnet-4-6',         'Claude Sonnet 4.6 (Best quality)'],
    ]
  },
  gemini: {
    hint: 'Get your free key at <a href="https://aistudio.google.com/api-keys" target="_blank" style="color:var(--accent2)">aistudio.google.com</a>',
    models: [
      ['gemini-1.5-flash', 'Gemini 1.5 Flash (Free tier)'],
      ['gemini-1.5-pro',   'Gemini 1.5 Pro'],
    ]
  },
  groq: {
    hint: 'Get your free key at <a href="https://console.groq.com" target="_blank" style="color:var(--accent2)">console.groq.com</a>',
    models: [
      ['llama-3.1-8b-instant', 'Llama 3.1 8B (Fast + Free)'],
      ['mixtral-8x7b-32768',   'Mixtral 8x7B'],
    ]
  }
};

// ── Step 0: System Check ──────────────────────────────────────────────────────

async function runSystemCheck() {
  document.getElementById('system-rows').innerHTML = `
    <div class="status-row">
      <span class="status-label">Checking system...</span>
      <span><div class="spinner"></div></span>
    </div>`;

  const r = await fetch('/api/system-check');
  const data = await r.json();
  systemData = data;

  const isMac = data.os === 'mac';
  const elSvc = document.getElementById('service-desc');
  if (elSvc) elSvc.textContent = isMac
    ? 'This installs a launchd agent so PicoClaw starts automatically on login.'
    : 'This installs a systemd service so PicoClaw starts automatically on boot.';

  const rows = [
    ['PicoClaw',     data.picoclaw_installed, data.picoclaw_version || 'Not found'],
    ['Disk Space',   true,                    data.disk_space],
    ['RAM',          true,                    data.ram],
    ['LLM Provider', data.has_provider,       data.active_model ? `${data.active_model} (${data.active_provider})` : 'Not set'],
    ['Telegram',     data.has_telegram,       data.telegram_token ? `Token: ${data.telegram_token}` : 'Not set'],
    ['SOUL.md',      data.has_soul,           data.has_soul ? 'Found' : 'Not created'],
    ['Service',      data.service_status === 'active', data.service_status],
  ];

  document.getElementById('system-rows').innerHTML = rows.map(([label, ok, val]) => `
    <div class="status-row">
      <span class="status-label">${label}</span>
      <div class="status-right">
        <span class="status-value" title="${val}">${val}</span>
        <span class="badge ${ok ? 'ok' : ['LLM Provider','Telegram','SOUL.md','Service'].includes(label) ? 'warn' : 'fail'}">
          ${ok ? '✓ OK' : '○ Pending'}
        </span>
      </div>
    </div>`).join('');

  if (!data.picoclaw_installed) {
    showAlert('sys-alert', 'error', 'PicoClaw not found on this device.');
    document.getElementById('install-picoclaw-section').style.display = 'block';
    markError(0);
  } else {
    document.getElementById('install-picoclaw-section').style.display = 'none';
    hideAlert('sys-alert');
    document.getElementById('btn-sys-next').disabled = false;
    document.getElementById('quick-actions').style.display = 'block';
    if (data.checklist.system)   markDone(0);
    if (data.checklist.provider) { markDone(1); state.llm = true; }
    if (data.checklist.telegram) { markDone(2); state.telegram = true; }
    if (data.checklist.soul)     { markDone(3); state.soul = true; }
    if (data.checklist.service)  { markDone(4); state.service = true; }
    state.system = true;
  }
}

async function installPicoclaw() {
  const btn = document.getElementById('btn-install-picoclaw');
  btn.innerHTML = '<div class="spinner"></div> Installing...';
  btn.disabled = true;
  showAlert('install-alert', 'info', 'Downloading PicoClaw for your device...');
  const r = await fetch('/api/install-picoclaw', { method: 'POST' });
  const data = await r.json();
  if (data.ok) {
    showAlert('install-alert', 'success', '✓ ' + data.message);
    document.getElementById('install-picoclaw-section').style.display = 'none';
    setTimeout(() => runSystemCheck(), 1000);
  } else {
    showAlert('install-alert', 'error', '✗ ' + data.message);
    btn.innerHTML = '⬇ Retry Install'; btn.disabled = false;
  }
}

async function checkPicoClawVersion() {
  const r = await fetch('/api/picoclaw/version');
  const data = await r.json();
  if (data.update_available) {
    document.getElementById('picoclaw-update-card').style.display = 'block';
    document.getElementById('update-info').textContent = `Current: ${data.current}  →  Latest: ${data.latest}`;
  }
}

async function updatePicoclaw() {
  const btn = document.getElementById('btn-update-picoclaw');
  btn.innerHTML = '<div class="spinner"></div> Updating...'; btn.disabled = true;
  const r = await fetch('/api/picoclaw/update', { method: 'POST' });
  const data = await r.json();
  if (data.ok) {
    showAlert('update-alert', 'success', '✓ ' + data.message);
  } else {
    showAlert('update-alert', 'error', '✗ ' + data.message);
    btn.innerHTML = '⬆ Retry Update'; btn.disabled = false;
  }
}

async function pingTelegramFrom(alertId, tileId) {
  const tile = document.getElementById(tileId);
  if (tile) tile.classList.add('loading');
  const chatID = document.getElementById('tg-userid')?.value?.trim() || systemData?.telegram_user || '';
  if (!chatID) {
    showAlert(alertId, 'error', 'No Telegram user ID saved — complete Step 2 first');
    if (tile) tile.classList.remove('loading');
    return;
  }
  const fd = new FormData(); fd.append('chat_id', chatID);
  const r = await fetch('/api/ping-telegram', { method: 'POST', body: fd });
  const data = await r.json();
  if (tile) tile.classList.remove('loading');
  showAlert(alertId, data.ok ? 'success' : 'error',
    data.ok ? '🏓 Ping sent — check your Telegram!' : '✗ ' + data.message);
}

async function restartServiceFrom(alertId, tileId) {
  const tile = document.getElementById(tileId);
  if (tile) tile.classList.add('loading');
  const r = await fetch('/api/restart-service', { method: 'POST' });
  const data = await r.json();
  if (tile) tile.classList.remove('loading');
  showAlert(alertId, data.ok ? 'success' : 'error',
    data.ok ? '✓ Agent restarted' : '✗ ' + (data.message || 'Restart failed'));
}

// ── Step 1: LLM ───────────────────────────────────────────────────────────────

let allModels = [];

function selectProvider(p) {
  selectedProvider = p;
  document.querySelectorAll('.provider-card').forEach(c => c.classList.remove('selected'));
  document.getElementById(`pcard-${p}`).classList.add('selected');
  const info = providerModels[p];
  document.getElementById('llm-key-hint').innerHTML = info.hint;
  const isOR = p === 'openrouter';
  document.getElementById('btn-load-models').style.display = isOR ? 'inline-flex' : 'none';
  document.getElementById('free-only').parentElement.style.display = isOR ? 'flex' : 'none';
  if (!isOR) {
    document.getElementById('llm-model').innerHTML = info.models
      .map(([v, l]) => `<option value="${v}">${l}</option>`).join('');
  }
  updateKeyStatus();
  hideAlert('llm-alert');
}

function hasSavedKey() {
  return systemData.has_provider && systemData.active_provider === selectedProvider;
}

function updateKeyStatus() {
  const saved = hasSavedKey();
  document.getElementById('key-status').className = saved ? 'key-status visible' : 'key-status';
  const typed = document.getElementById('llm-key').value.trim().length >= 10;
  document.getElementById('btn-load-models').disabled = !saved && !typed;
  document.getElementById('btn-validate-llm').textContent = saved ? 'Save Model' : 'Validate Key';
}

function onKeyInput() { updateKeyStatus(); }

async function loadModels() {
  const key = document.getElementById('llm-key').value.trim();
  const btn = document.getElementById('btn-load-models');
  btn.innerHTML = '<div class="spinner"></div>'; btn.disabled = true;
  const fd = new FormData();
  fd.append('provider', selectedProvider);
  if (key) fd.append('api_key', key);
  const r = await fetch('/api/models', { method: 'POST', body: fd });
  const data = await r.json();
  btn.innerHTML = '↻ Load Models'; btn.disabled = false;
  if (!data.ok) { showAlert('llm-alert', 'error', '✗ ' + data.message); return; }
  allModels = data.models; filterModels();
  showAlert('llm-alert', 'success', `✓ Loaded ${data.models.length} models`);
}

function filterModels() {
  const freeOnly = document.getElementById('free-only').checked;
  const sel = document.getElementById('llm-model');
  let filtered = freeOnly ? allModels.filter(m => m.free) : allModels;
  filtered = [...filtered].sort((a, b) => a.name.localeCompare(b.name));
  const autoOption = `<option value="openrouter/auto">⚡ Auto — let OpenRouter choose best model</option>`;
  sel.innerHTML = autoOption + filtered.map(m => `<option value="${m.id}">${m.name}${m.free ? ' 🆓' : ''}</option>`).join('');
}

async function validateLLM() {
  const key = document.getElementById('llm-key').value.trim();
  const model = document.getElementById('llm-model').value;
  if (!model) { showAlert('llm-alert', 'error', 'Please select a model'); return; }
  if (!key && !hasSavedKey()) { showAlert('llm-alert', 'error', 'Please enter your API key'); return; }

  const btn = document.getElementById('btn-validate-llm');
  btn.innerHTML = '<div class="spinner"></div> Saving...'; btn.disabled = true;

  const fd = new FormData();
  fd.append('provider', selectedProvider || 'openrouter');
  fd.append('model', model);
  if (key) fd.append('api_key', key);

  const r = await fetch('/api/validate-llm', { method: 'POST', body: fd });
  const data = await r.json();
  btn.innerHTML = hasSavedKey() ? 'Save Model' : 'Validate Key'; btn.disabled = false;

  if (data.ok) {
    systemData.has_provider = true; systemData.active_provider = selectedProvider; systemData.active_model = model;
    updateKeyStatus(); markDone(1); state.llm = true;
    document.getElementById('btn-llm-next').disabled = false;
    if (state.service) {
      showAlert('llm-alert', 'info', `✓ Model saved — restarting agent to apply ${model}...`);
      btn.innerHTML = '<div class="spinner"></div> Restarting...'; btn.disabled = true;
      const rr = await fetch('/api/restart-service', { method: 'POST' });
      const rd = await rr.json();
      btn.innerHTML = hasSavedKey() ? 'Save Model' : 'Validate Key'; btn.disabled = false;
      showAlert('llm-alert', rd.ok ? 'success' : 'warn',
        rd.ok ? `✓ Model updated & agent restarted — now using ${model}` : `✓ Model saved but restart failed: ${rd.message}`);
    } else {
      showAlert('llm-alert', 'success', '✓ ' + data.message + ` — using ${model}`);
    }
  } else {
    showAlert('llm-alert', 'error', '✗ ' + data.message); markError(1);
  }
}

function populateLLM() {
  if (systemData.active_provider) selectProvider(systemData.active_provider);
  if (systemData.active_model) {
    const sel = document.getElementById('llm-model');
    let found = false;
    for (const opt of sel.options) {
      if (opt.value === systemData.active_model) { opt.selected = true; found = true; break; }
    }
    if (!found) {
      const opt = document.createElement('option');
      opt.value = systemData.active_model;
      opt.text = systemData.active_model + ' (current)';
      opt.selected = true;
      sel.insertBefore(opt, sel.firstChild);
    }
  }
  if (systemData.has_provider) {
    showAlert('llm-alert', 'success', '✓ Already configured — model: ' + systemData.active_model);
    document.getElementById('btn-llm-next').disabled = false;
    markDone(1);
  }
  updateKeyStatus();
}

// ── Step 2: Telegram ──────────────────────────────────────────────────────────

async function validateTelegram() {
  const token = document.getElementById('tg-token').value.trim();
  if (!token) { showAlert('tg-alert', 'error', 'Please enter your bot token'); return; }
  const fd = new FormData(); fd.append('token', token);
  showAlert('tg-alert', 'info', 'Validating token...');
  const r = await fetch('/api/validate-telegram', { method: 'POST', body: fd });
  const data = await r.json();
  if (data.ok) {
    showAlert('tg-alert', 'success', '✓ ' + data.message);
    document.getElementById('tg-userid-section').style.display = 'block';
  } else {
    showAlert('tg-alert', 'error', '✗ ' + data.message);
  }
}

async function saveTelegramUser() {
  const uid = document.getElementById('tg-userid').value.trim();
  if (!uid) { showAlert('tg-userid-alert', 'error', 'Please enter your User ID'); return; }
  const fd = new FormData(); fd.append('user_id', uid);
  const r = await fetch('/api/save-telegram-user', { method: 'POST', body: fd });
  const data = await r.json();
  if (data.ok) {
    showAlert('tg-userid-alert', 'success', '✓ User ID saved');
    document.getElementById('tg-ping-section').style.display = 'block';
  }
}

async function sendPing() {
  const uid = document.getElementById('tg-userid').value.trim();
  showAlert('tg-ping-alert', 'info', 'Sending ping...');
  const fd = new FormData(); fd.append('chat_id', uid);
  const r = await fetch('/api/ping-telegram', { method: 'POST', body: fd });
  const data = await r.json();
  if (data.ok) {
    showAlert('tg-ping-alert', 'success', '✓ Ping sent! Check your Telegram — if you got it, continue.');
    document.getElementById('btn-tg-next').disabled = false;
    markDone(2); state.telegram = true;
  } else {
    showAlert('tg-ping-alert', 'error', '✗ ' + data.message + ' — Make sure you sent /start to your bot first.');
  }
}

function populateTelegram() {
  if (!systemData.has_telegram) return;
  showAlert('tg-alert', 'success', '✓ Telegram already configured — token: ' + systemData.telegram_token);
  document.getElementById('tg-userid-section').style.display = 'block';
  document.getElementById('tg-ping-section').style.display = 'block';
  if (systemData.telegram_user) {
    document.getElementById('tg-userid').value = systemData.telegram_user;
    showAlert('tg-userid-alert', 'success', '✓ User ID already saved: ' + systemData.telegram_user);
  }
  document.getElementById('btn-tg-next').disabled = false;
  markDone(2);
}

// ── Step 3: Soul ──────────────────────────────────────────────────────────────

async function generateSoul() {
  const fields = ['username','name','role','expertise','style','goals','dislikes','decisions'];
  for (const f of fields) {
    if (!document.getElementById(`soul-${f}`).value.trim()) {
      showAlert('soul-alert', 'error', `Please fill in all fields — "${f}" is empty`); return;
    }
  }
  showAlert('soul-alert', 'info', 'Generating your SOUL.md...');
  const fd = new FormData();
  fields.forEach(f => fd.append(f === 'username' ? 'user_name' : f, document.getElementById(`soul-${f}`).value.trim()));
  const r = await fetch('/api/generate-soul', { method: 'POST', body: fd });
  const data = await r.json();
  hideAlert('soul-alert');
  document.getElementById('soul-result').style.display = 'block';
  const preview = document.getElementById('soul-preview');
  preview.textContent = data.soul;
  preview.style.display = 'block';
  preview.dataset.soul = data.soul;
}

async function saveSoul() {
  const soul = document.getElementById('soul-preview').dataset.soul;
  if (!soul) return;
  const fd = new FormData(); fd.append('soul_content', soul);
  const r = await fetch('/api/save-soul', { method: 'POST', body: fd });
  const data = await r.json();
  if (data.ok) {
    showAlert('soul-save-alert', 'success', '✓ SOUL.md saved to your Pi at ' + data.message.split('to ')[1]);
    document.getElementById('btn-soul-next').disabled = false;
    markDone(3); state.soul = true;
  } else {
    showAlert('soul-save-alert', 'error', '✗ ' + data.message);
  }
}

function populateSoul() {
  if (!systemData.has_soul) return;
  showAlert('soul-alert', 'success', '✓ SOUL.md already exists — fill the form and regenerate to update it.');
}

// ── Step 4: Launch ────────────────────────────────────────────────────────────

async function loadFinalChecklist() {
  const r = await fetch('/api/system-check');
  const data = await r.json();
  const items = [
    ['PicoClaw installed',       data.picoclaw_installed],
    ['LLM provider configured',  data.has_provider],
    ['Telegram connected',       data.has_telegram],
    ['SOUL.md created',          data.has_soul],
    ['Service running',          data.service_status === 'active'],
  ];
  document.getElementById('checklist-rows').innerHTML = items.map(([label, ok]) => `
    <div class="final-item">
      <span class="check">${ok ? '✅' : '⭕'}</span>
      <span>${label}</span>
    </div>`).join('');

  if (data.service_status === 'active') {
    document.getElementById('service-install-card').style.display = 'none';
    document.getElementById('launch-success').style.display = 'block';
    document.getElementById('progress').style.width = '100%';
    populateOSCommands(data.os === 'mac');
    markDone(4);
  }
}

async function installService() {
  showAlert('service-alert', 'info', 'Installing systemd service...');
  const r = await fetch('/api/install-service', { method: 'POST' });
  const data = await r.json();
  if (data.ok) {
    document.getElementById('service-install-card').style.display = 'none';
    document.getElementById('launch-success').style.display = 'block';
    document.getElementById('progress').style.width = '100%';
    markDone(4); state.service = true;
    setTimeout(() => loadFinalChecklist(), 2000);
  } else {
    showAlert('service-alert', 'error', '✗ ' + data.message);
    document.getElementById('service-install-card').style.display = 'block';
  }
}

function populateOSCommands(isMac) {
  const hint = document.getElementById('cmd-hint');
  const status = document.getElementById('cmd-status');
  const restart = document.getElementById('cmd-restart');
  if (!hint) return;
  hint.textContent = isMac ? 'Useful commands on your Mac:' : 'Useful commands on your Pi:';
  status.textContent = isMac ? 'launchctl list | grep picoclaw' : 'systemctl --user status picoclaw';
  restart.textContent = isMac
    ? 'launchctl unload ~/Library/LaunchAgents/com.picoclaw.agent.plist && launchctl load ~/Library/LaunchAgents/com.picoclaw.agent.plist'
    : 'systemctl --user restart picoclaw';
}

// ── Uninstall modal ───────────────────────────────────────────────────────────

function openUninstallModal() {
  document.getElementById('uninstall-modal').classList.add('open');
  document.getElementById('uninstall-confirm-input').value = '';
  document.getElementById('uninstall-confirm-btn').disabled = true;
  document.getElementById('uninstall-steps-list').innerHTML = '';
  hideAlert('uninstall-modal-alert');
}

function closeUninstallModal() {
  document.getElementById('uninstall-modal').classList.remove('open');
}

async function confirmUninstall() {
  const btn = document.getElementById('uninstall-confirm-btn');
  btn.innerHTML = '<div class="spinner" style="display:inline-block;width:12px;height:12px;border-width:2px;margin-right:6px"></div> Uninstalling...';
  btn.disabled = true;
  const r = await fetch('/api/uninstall', { method: 'POST' });
  const data = await r.json();
  const list = document.getElementById('uninstall-steps-list');
  list.innerHTML = (data.steps || []).map(s => `
    <div style="display:flex;align-items:flex-start;gap:8px;padding:6px 0;border-bottom:1px solid var(--border)">
      <span style="font-size:13px;flex-shrink:0">${s.ok ? '✅' : '⚠️'}</span>
      <div>
        <div style="font-size:13px;font-weight:600">${s.label}</div>
        <div style="font-size:11px;color:var(--text2)">${s.detail}</div>
      </div>
    </div>`).join('');
  if (data.ok) {
    showAlert('uninstall-modal-alert', 'success', '✓ ' + data.message);
    btn.innerHTML = 'Done';
    setTimeout(() => { closeUninstallModal(); goTo(0); }, 3000);
  } else {
    showAlert('uninstall-modal-alert', 'error', data.message);
    btn.innerHTML = 'Retry'; btn.disabled = false;
  }
}

// Input listener for uninstall confirm
document.addEventListener('DOMContentLoaded', () => {
  const input = document.getElementById('uninstall-confirm-input');
  if (input) {
    input.addEventListener('input', function() {
      const btn = document.getElementById('uninstall-confirm-btn');
      const ok = this.value.trim() === 'uninstall';
      btn.disabled = !ok;
      btn.style.opacity = ok ? '0.9' : '0.5';
    });
  }
});
