// tools.js — Step 5: Connected Tools (OAuth, Gmail, GCal)

let oauthPollInterval = null;
const categoryIcon = { email: '✉️', calendar: '📅', utility: '🔧' };

async function loadTools() {
  document.getElementById('tools-loading').style.display = 'block';
  document.getElementById('tools-grid').style.display = 'none';

  const r = await fetch('/api/tools');
  const data = await r.json();

  document.getElementById('tools-loading').style.display = 'none';

  if (!data.ok) {
    document.getElementById('tools-loading').innerHTML =
      `<span style="color:var(--danger)">✗ Could not load tools registry: ${data.message}</span>`;
    document.getElementById('tools-loading').style.display = 'block';
    // Still load weather status even if tools registry fails
    loadWeatherStatus();
    return;
  }

  const grid = document.getElementById('tools-grid');
  grid.style.display = 'block';
  grid.innerHTML = data.tools.map(t => {
    const available = t.status === 'available';
    const icon = categoryIcon[t.category] || '🔧';
    const authBadge = t.requires_auth?.length > 0
      ? `<span style="font-size:10px;background:rgba(108,99,255,0.15);color:var(--accent);padding:2px 7px;border-radius:20px;margin-left:6px">OAuth</span>`
      : '';
    const connectedBadge = t.connected
      ? `<span style="font-size:10px;background:rgba(0,212,170,0.15);color:var(--success);padding:2px 7px;border-radius:20px;margin-left:4px">✓ Connected</span>`
      : '';
    const comingSoon = !available
      ? `<span style="font-size:10px;background:var(--surface2);color:var(--text2);padding:2px 7px;border-radius:20px;margin-left:4px">Coming soon</span>`
      : '';

    return `
      <div style="padding:12px 0;border-bottom:1px solid var(--border);">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">
          <span style="font-size:16px">${icon}</span>
          <span style="font-size:14px;font-weight:600">${t.name}</span>
          ${authBadge}${connectedBadge}${comingSoon}
        </div>
        <p style="font-size:12px;color:var(--text2);margin:0 0 6px 24px">${t.description}</p>
        <div style="margin-left:24px;display:flex;gap:6px;flex-wrap:wrap">
          ${t.mcp_tools.map(m => `<code style="font-size:10px;background:var(--surface2);color:var(--accent2);padding:1px 6px;border-radius:4px">${m}</code>`).join('')}
        </div>
      </div>`;
  }).join('');

  const hasGoogle = data.tools.some(t => t.requires_auth?.includes('google_oauth2'));
  if (hasGoogle) {
    document.getElementById('accounts-card').style.display = 'block';
    renderAccounts(data.tools[0]?.accounts || []);
    if (data.tools.some(t => t.connected)) {
      markDone(5); state.tools = true;
    }
  }

  // Always load weather status after tools
  loadWeatherStatus();
}

function renderAccounts(accounts) {
  const list = document.getElementById('accounts-list');
  if (!accounts || accounts.length === 0) {
    list.innerHTML = `<p style="font-size:13px;color:var(--text2);padding:8px 0">No accounts connected yet.</p>`;
    return;
  }
  list.innerHTML = accounts.map(a => `
    <div style="display:flex;align-items:center;justify-content:space-between;padding:10px 0;border-bottom:1px solid var(--border)">
      <div>
        <div style="font-size:13px;font-weight:600">${a.email}</div>
        <div style="font-size:11px;color:var(--text2)">Added ${new Date(a.added_at).toLocaleDateString()}</div>
      </div>
      <button class="btn btn-secondary" style="font-size:11px;padding:5px 10px" onclick="revokeAccount('${a.email}')">Revoke</button>
    </div>`).join('');
}

async function startOAuth() {
  document.getElementById('oauth-progress-card').style.display = 'block';
  hideAlert('oauth-alert');

  const r = await fetch('/api/oauth/start', { method: 'POST' });
  const data = await r.json();
  if (!data.ok) {
    showAlert('oauth-alert', 'error', '✗ ' + data.message);
    document.getElementById('oauth-progress-card').style.display = 'none';
    return;
  }
  oauthPollInterval = setInterval(pollOAuthStatus, 2000);
}

async function pollOAuthStatus() {
  const r = await fetch('/api/oauth/status');
  const data = await r.json();
  if (data.status === 'pending') return;

  clearInterval(oauthPollInterval);
  document.getElementById('oauth-progress-card').style.display = 'none';

  if (data.ok && data.status === 'connected') {
    showAlert('oauth-alert', 'success', `✓ ${data.email} connected`);
    document.getElementById('oauth-progress-card').style.display = 'block';
    setTimeout(() => {
      document.getElementById('oauth-progress-card').style.display = 'none';
      loadTools();
    }, 2000);
  } else {
    showAlert('oauth-alert', 'error', '✗ ' + (data.message || 'Authorization failed'));
    document.getElementById('oauth-progress-card').style.display = 'block';
  }
}

async function revokeAccount(email) {
  if (!confirm(`Remove ${email}?\n\nThe saved token will be deleted. You can reconnect anytime.`)) return;
  const fd = new FormData(); fd.append('email', email);
  const r = await fetch('/api/oauth/revoke', { method: 'POST', body: fd });
  const data = await r.json();
  if (data.ok) { loadTools(); } else { alert('Failed to revoke: ' + data.message); }
}
