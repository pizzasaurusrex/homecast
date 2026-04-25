// homecast dashboard — vanilla JS, no framework.
// Talks to the JSON API at /api/* and renders three panels:
//   status (bridge state + uptime), devices (enable/disable), recent logs.
(function () {
  'use strict';

  // Poll cadences. /api/devices triggers a 3s mDNS browse today, so it runs
  // less often than status/logs (PLAN.md notes a TTL cache will fix this).
  const STATUS_POLL_MS = 5000;
  const LOGS_POLL_MS = 5000;
  const DEVICES_POLL_MS = 30000;
  const LOG_TAIL = 200;
  const TOAST_MS = 3500;

  const $ = (id) => document.getElementById(id);

  let toastTimer = null;

  async function apiCall(path, options) {
    let res;
    try {
      res = await fetch(path, options);
    } catch (e) {
      throw new Error('network error: ' + e.message);
    }
    let body = null;
    try { body = await res.json(); } catch (_) { /* non-JSON */ }
    if (!res.ok || !body || body.ok === false) {
      const msg = (body && body.error) || res.statusText || `HTTP ${res.status}`;
      throw new Error(msg);
    }
    return body.data;
  }

  function formatUptime(secs) {
    if (typeof secs !== 'number' || secs <= 0) return '—';
    const s = secs % 60;
    const m = Math.floor(secs / 60) % 60;
    const h = Math.floor(secs / 3600);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  function showToast(msg, isError) {
    const el = $('toast');
    el.textContent = msg;
    el.classList.toggle('error', !!isError);
    el.classList.remove('hidden');
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), TOAST_MS);
  }

  function setBridgeState(state) {
    const el = $('bridge-state');
    el.textContent = state || 'unknown';
    const safe = String(state || 'unknown').replace(/[^a-z0-9-]/gi, '');
    el.className = 'value state-' + (safe || 'unknown');
  }

  async function refreshStatus() {
    try {
      const data = await apiCall('/api/status');
      setBridgeState(data.bridgeState);
      $('uptime').textContent = formatUptime(data.uptimeSeconds);
    } catch (e) {
      setBridgeState('unknown');
      $('uptime').textContent = '—';
    }
  }

  function renderDevices(devices) {
    const list = $('devices');
    list.textContent = '';
    if (!devices || devices.length === 0) {
      $('devices-empty').classList.remove('hidden');
      return;
    }
    $('devices-empty').classList.add('hidden');
    for (const d of devices) {
      list.appendChild(renderDeviceRow(d));
    }
  }

  function renderDeviceRow(d) {
    const li = document.createElement('li');
    li.className = 'device';
    li.dataset.deviceId = d.id;

    const label = document.createElement('label');
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.checked = !!d.enabled;
    checkbox.addEventListener('change', () => toggleDevice(d, checkbox));
    label.appendChild(checkbox);

    const nameWrap = document.createElement('span');
    const name = document.createElement('span');
    name.className = 'device-name';
    name.textContent = d.name || '(unnamed)';
    nameWrap.appendChild(name);
    if (Array.isArray(d.addrs) && d.addrs.length > 0) {
      const addrs = document.createElement('span');
      addrs.className = 'device-addrs';
      addrs.textContent = d.addrs.join(', ');
      nameWrap.appendChild(addrs);
    }
    label.appendChild(nameWrap);
    li.appendChild(label);

    if (!d.discovered) {
      const badges = document.createElement('span');
      badges.className = 'badges';
      const badge = document.createElement('span');
      badge.className = 'badge badge-warn';
      badge.textContent = 'offline';
      badge.title = 'Saved but not seen on the LAN right now';
      badges.appendChild(badge);
      li.appendChild(badges);
    }
    return li;
  }

  async function refreshDevices() {
    try {
      const devices = await apiCall('/api/devices');
      renderDevices(devices);
    } catch (e) {
      showToast('Failed to list devices: ' + e.message, true);
    }
  }

  async function toggleDevice(device, checkbox) {
    const action = checkbox.checked ? 'enable' : 'disable';
    checkbox.disabled = true;
    try {
      await apiCall(`/api/devices/${encodeURIComponent(device.id)}/${action}`, {
        method: 'POST',
      });
      showToast(`${device.name || device.id} ${action}d`);
    } catch (e) {
      checkbox.checked = !checkbox.checked;
      showToast(`Could not ${action} ${device.name || device.id}: ${e.message}`, true);
    } finally {
      checkbox.disabled = false;
    }
  }

  async function refreshLogs() {
    try {
      const data = await apiCall(`/api/logs?tail=${LOG_TAIL}`);
      const lines = (data && data.lines) || [];
      const el = $('logs');
      if (lines.length === 0) {
        el.textContent = '(no log output yet)';
      } else {
        el.textContent = lines.join('\n');
        el.scrollTop = el.scrollHeight;
      }
      $('logs-meta').textContent = `${lines.length} line${lines.length === 1 ? '' : 's'}`;
    } catch (e) {
      $('logs').textContent = 'Failed to load logs: ' + e.message;
      $('logs-meta').textContent = '';
    }
  }

  async function restartBridge() {
    const btn = $('restart-btn');
    btn.disabled = true;
    const original = btn.textContent;
    btn.textContent = 'Restarting…';
    try {
      await apiCall('/api/bridge/restart', { method: 'POST' });
      showToast('Bridge restart requested');
      await refreshStatus();
    } catch (e) {
      showToast('Restart failed: ' + e.message, true);
    } finally {
      btn.disabled = false;
      btn.textContent = original;
    }
  }

  function start() {
    $('restart-btn').addEventListener('click', restartBridge);
    $('refresh-devices-btn').addEventListener('click', refreshDevices);

    refreshStatus();
    refreshDevices();
    refreshLogs();
    setInterval(refreshStatus, STATUS_POLL_MS);
    setInterval(refreshLogs, LOGS_POLL_MS);
    setInterval(refreshDevices, DEVICES_POLL_MS);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
