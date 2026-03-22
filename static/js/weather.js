// weather.js — Weather tool logic for Step 5

async function loadWeatherStatus() {
  try {
    const r = await fetch('/api/weather/status');
    const d = await r.json();

    // Binary badge
    const binBadge = document.getElementById('weather-bin-badge');
    binBadge.className = d.bin_installed ? 'badge ok' : 'badge warn';
    binBadge.textContent = d.bin_installed ? '✓ Installed' : '○ Not installed';

    // Service badge
    const svcBadge = document.getElementById('weather-svc-badge');
    svcBadge.className = d.svc_running ? 'badge ok' : 'badge warn';
    svcBadge.textContent = d.svc_running ? '✓ Running' : '○ Not running';

    // Location badge
    const locBadge = document.getElementById('weather-loc-badge');
    const locValue = document.getElementById('weather-loc-value');
    if (d.location_set) {
      locBadge.className = 'badge ok';
      locBadge.textContent = '✓ Set';
      locValue.textContent = d.label || '';
      if (document.getElementById('weather-city-input').value === '') {
        document.getElementById('weather-city-input').value = d.label || '';
      }
      loadWeatherPreview();
    } else {
      locBadge.className = 'badge warn';
      locBadge.textContent = '○ Not set';
    }

    // Show install section only if not fully set up
    const fullyReady = d.bin_installed && d.svc_running && d.scripts_written && d.skill_written;
    document.getElementById('weather-install-wrap').style.display = fullyReady ? 'none' : 'block';
    document.getElementById('weather-done-wrap').style.display    = fullyReady ? 'block' : 'none';

  } catch(e) {
    // API not reachable — silently skip
  }
}

async function setWeatherLocation() {
  const city = document.getElementById('weather-city-input').value.trim();
  if (!city) { showAlert('weather-alert', 'error', 'Please enter a city name'); return; }

  const btn = document.getElementById('btn-set-loc');
  btn.innerHTML = '<div class="spinner" style="display:inline-block;width:12px;height:12px;border-width:2px"></div>';
  btn.disabled = true;
  hideAlert('weather-alert');

  const fd = new FormData();
  fd.append('city', city);
  const r = await fetch('/api/weather/location', { method: 'POST', body: fd });
  const d = await r.json();

  btn.innerHTML = 'Set';
  btn.disabled = false;

  if (d.ok) {
    document.getElementById('weather-loc-badge').className = 'badge ok';
    document.getElementById('weather-loc-badge').textContent = '✓ Set';
    document.getElementById('weather-loc-value').textContent = d.label;
    showAlert('weather-alert', 'success', `✓ Location set: ${d.label} (${d.lat.toFixed(4)}, ${d.lon.toFixed(4)})`);
    loadWeatherPreview();
  } else {
    showAlert('weather-alert', 'error', '✗ ' + (d.message || d.error || 'Unknown error'));
  }
}

async function installWeatherTool() {
  const btn = document.getElementById('btn-install-weather');
  btn.innerHTML = '<div class="spinner" style="display:inline-block;width:12px;height:12px;border-width:2px;margin-right:6px"></div> Installing...';
  btn.disabled = true;
  hideAlert('weather-alert');

  const r = await fetch('/api/weather/install', { method: 'POST' });
  const d = await r.json();

  if (d.ok) {
    showAlert('weather-alert', 'success', '✓ ' + d.message);
    await loadWeatherStatus();
    if (d.message && d.message.includes('restarted')) {
      showAlert('weather-alert', 'success', '✓ Agent restarted — say "what\'s the weather?" in Telegram to test it.');
    }
  } else {
    showAlert('weather-alert', 'error', '✗ ' + d.message);
    btn.innerHTML = '⚡ Install &amp; Activate Weather Tool';
    btn.disabled = false;
  }
}

async function loadWeatherPreview() {
  const wrap = document.getElementById('weather-preview-wrap');
  const text = document.getElementById('weather-preview-text');
  wrap.style.display = 'block';
  text.textContent = 'Fetching forecast...';

  try {
    const r = await fetch('/api/weather/forecast');
    const d = await r.json();

    if (!d.ok) {
      text.textContent = '⚠ Could not fetch forecast: ' + (d.message || 'unknown error');
      return;
    }

    // weather-mcp HTTP server returned a pre-formatted summary
    if (d.summary) {
      text.textContent = d.summary;
      return;
    }

    // Fallback: build preview from raw Open-Meteo hourly data
    text.textContent = buildForecastPreview(d);
  } catch(e) {
    text.textContent = '⚠ Forecast unavailable';
  }
}

function buildForecastPreview(data) {
  const wmo = (code) => {
    if (code === 0) return 'Clear sky ☀️';
    if (code === 1) return 'Mainly clear 🌤';
    if (code === 2) return 'Partly cloudy ⛅';
    if (code === 3) return 'Overcast ☁️';
    if (code >= 45 && code <= 48) return 'Foggy 🌫';
    if (code >= 51 && code <= 55) return 'Drizzle 🌦';
    if (code >= 61 && code <= 65) return 'Rain 🌧';
    if (code >= 71 && code <= 77) return 'Snow ❄️';
    if (code >= 80 && code <= 82) return 'Showers 🌧';
    if (code >= 95) return 'Thunderstorm ⛈';
    return '—';
  };

  const h = data.hourly;
  if (!h?.time) return 'No forecast data.';

  const buckets = {
    '🌅 Morning (6–12)':    [],
    '☀️  Afternoon (12–18)': [],
    '🌙 Evening (18–23)':   [],
  };

  h.time.forEach((t, i) => {
    const hr = new Date(t).getHours();
    const e = {
      temp: h.temperature_2m?.[i],
      rain: h.precipitation_probability?.[i] ?? 0,
      code: h.weathercode?.[i] ?? 0,
    };
    if (hr >= 6  && hr < 12) buckets['🌅 Morning (6–12)'].push(e);
    if (hr >= 12 && hr < 18) buckets['☀️  Afternoon (12–18)'].push(e);
    if (hr >= 18 && hr < 23) buckets['🌙 Evening (18–23)'].push(e);
  });

  const fmt = (entries) => {
    if (!entries.length) return '—';
    const temps = entries.map(e => e.temp).filter(t => t != null);
    const mn = Math.min(...temps).toFixed(0);
    const mx = Math.max(...temps).toFixed(0);
    const rain = Math.max(...entries.map(e => e.rain));
    const code = entries[Math.floor(entries.length / 2)]?.code ?? 0;
    return `${mn}°C → ${mx}°C   ${wmo(code)}   💧${rain}%`;
  };

  const today = new Date().toLocaleDateString('en-CA', { weekday:'long', month:'short', day:'numeric', year:'numeric' });
  const lines = [`Weather for ${data.location} — ${today}`, ''];
  for (const [label, entries] of Object.entries(buckets)) {
    lines.push(`${label.padEnd(24)} ${fmt(entries)}`);
  }
  return lines.join('\n');
}
