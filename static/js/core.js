// core.js — shared state, navigation, alerts, QR modal, network bar

let currentStep = 0;
let selectedProvider = 'openrouter';
let systemData = {};
let state = { system: false, llm: false, telegram: false, soul: false, service: false, tools: false };
const progress = [10, 30, 50, 70, 88, 100, 100];

// ── Navigation ────────────────────────────────────────────────────────────────

function goTo(n) {
  document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
  document.querySelectorAll('.step-item').forEach(s => s.classList.remove('active'));
  document.querySelectorAll('.bottom-nav-item').forEach(s => s.classList.remove('active'));

  document.getElementById(`step-${n}`).classList.add('active');
  document.getElementById(`nav-${n}`).classList.add('active');
  document.getElementById(`bnav-${n}`).classList.add('active');
  document.getElementById('progress').style.width = progress[n] + '%';
  currentStep = n;

  document.querySelector('.main').scrollTo({ top: 0, behavior: 'smooth' });
  window.scrollTo({ top: 0, behavior: 'smooth' });

  if (n === 0) runSystemCheck();
  if (n === 1) populateLLM();
  if (n === 2) populateTelegram();
  if (n === 3) populateSoul();
  if (n === 4) loadFinalChecklist();
  if (n === 5) loadTools();
  if (n === 6) loadSettings();
}

function markDone(n) {
  document.getElementById(`icon-${n}`).textContent = '✓';
  document.getElementById(`bicon-${n}`).textContent = '✓';
  document.getElementById(`nav-${n}`).classList.add('done');
  document.getElementById(`bnav-${n}`).classList.add('done');
}

function markError(n) {
  document.getElementById(`nav-${n}`).classList.add('error');
  document.getElementById(`bnav-${n}`).classList.add('error');
}

// ── Alerts ────────────────────────────────────────────────────────────────────

function showAlert(id, type, msg) {
  const el = document.getElementById(id);
  el.className = `alert ${type} visible`;
  el.textContent = msg;
}

function hideAlert(id) {
  document.getElementById(id).className = 'alert';
}

// ── Network bar ───────────────────────────────────────────────────────────────

let localIP = '';

async function initNetBar() {
  try {
    const r = await fetch('/api/local-ip');
    const data = await r.json();
    localIP = data.ip || window.location.hostname;
  } catch(e) {
    localIP = window.location.hostname;
  }
  document.getElementById('net-url-text').textContent = `http://${localIP}:3000`;
  document.getElementById('net-bar').style.display = 'flex';
}

function copyNetUrl() {
  const url = `http://${localIP}:3000`;
  navigator.clipboard.writeText(url).then(() => {
    const btn = document.getElementById('net-copy-btn');
    btn.textContent = 'Copied!';
    setTimeout(() => btn.textContent = 'Copy', 1800);
  }).catch(() => prompt('Copy this URL:', url));
}

// ── QR Modal ──────────────────────────────────────────────────────────────────

function openQR() {
  const url = `http://${localIP}:3000`;
  document.getElementById('qr-url-label').textContent = url;
  document.getElementById('qr-overlay').classList.add('open');
  const container = document.getElementById('qr-container');
  container.innerHTML = '<div class="spinner"></div>';
  const img = new Image();
  img.width = 200; img.height = 200;
  img.style.cssText = 'border-radius:8px;display:block';
  img.onload = () => { container.innerHTML = ''; container.appendChild(img); };
  img.onerror = () => { container.innerHTML = ''; renderQRCanvas(container, url); };
  img.src = `https://api.qrserver.com/v1/create-qr-code/?size=200x200&color=ffffff&bgcolor=1a1d27&data=${encodeURIComponent(url)}`;
}

function renderQRCanvas(container, text) {
  const canvas = document.createElement('canvas');
  canvas.width = 200; canvas.height = 200;
  canvas.style.borderRadius = '8px';
  container.appendChild(canvas);
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#1a1d27'; ctx.fillRect(0, 0, 200, 200);
  ctx.fillStyle = '#ffffff'; ctx.font = '11px monospace'; ctx.textAlign = 'center';
  ctx.fillText('QR needs internet', 100, 90);
  ctx.fillText('to generate.', 100, 108);
  ctx.fillStyle = '#00d4aa'; ctx.font = '10px monospace';
  ctx.fillText('URL copied to clipboard', 100, 135);
  navigator.clipboard.writeText(text).catch(() => {});
}

function closeQR() { document.getElementById('qr-overlay').classList.remove('open'); }
function closeQRIfOutside(e) { if (e.target === document.getElementById('qr-overlay')) closeQR(); }
function closeUninstallIfOutside(e) { if (e.target === document.getElementById('uninstall-modal')) closeUninstallModal(); }
