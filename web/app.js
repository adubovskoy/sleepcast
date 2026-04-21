(() => {
  const $ = (id) => document.getElementById(id);
  const form = $('url-form');
  const urlInput = $('url-input');
  const playBtn = $('play-btn');
  const statusEl = $('status');
  const playerSection = $('player-section');
  const playerTitle = $('player-title');
  const audio = $('audio');
  const playPause = $('play-pause');
  const scrubber = $('scrubber');
  const curTime = $('cur-time');
  const durTime = $('dur-time');
  const sleepSelect = $('sleep-select');
  const sleepCountdown = $('sleep-countdown');
  const toast = $('toast');
  const historySection = $('history-section');
  const historyList = $('history-list');
  const clearHistoryBtn = $('clear-history');

  const HISTORY_KEY = 'sleepcast.history';
  const HISTORY_MAX = 10;

  let currentVideoId = null;
  let sleepTimer = null;
  let sleepDeadline = 0;
  let sleepTick = null;
  let lastPositionSaveAt = 0;

  const sleep = (ms) => new Promise(r => setTimeout(r, ms));

  function showToast(msg, ms = 2500) {
    toast.textContent = msg;
    toast.hidden = false;
    clearTimeout(showToast._t);
    showToast._t = setTimeout(() => { toast.hidden = true; }, ms);
  }

  function fmtTime(s) {
    if (!isFinite(s) || s < 0) s = 0;
    const m = Math.floor(s / 60);
    const r = Math.floor(s % 60).toString().padStart(2, '0');
    return `${m}:${r}`;
  }

  // ---- History (localStorage) ----
  function loadHistory() {
    try {
      const raw = localStorage.getItem(HISTORY_KEY);
      const arr = raw ? JSON.parse(raw) : [];
      return Array.isArray(arr) ? arr : [];
    } catch { return []; }
  }
  function saveHistory(arr) {
    try { localStorage.setItem(HISTORY_KEY, JSON.stringify(arr)); } catch {}
  }
  function upsertHistory(entry) {
    const h = loadHistory();
    const i = h.findIndex(e => e.videoId === entry.videoId);
    const prev = i >= 0 ? h[i] : {};
    const merged = { ...prev, ...entry, updatedAt: Date.now() };
    if (i >= 0) h.splice(i, 1);
    h.unshift(merged);
    if (h.length > HISTORY_MAX) h.length = HISTORY_MAX;
    saveHistory(h);
    renderHistory();
    return merged;
  }
  function updateHistoryPosition(videoId, position) {
    const h = loadHistory();
    const i = h.findIndex(e => e.videoId === videoId);
    if (i < 0) return;
    h[i].position = position;
    h[i].updatedAt = Date.now();
    saveHistory(h);
    // Update rendered position without rebuilding the list.
    const row = historyList.querySelector(`[data-video-id="${videoId}"] .h-pos`);
    if (row) row.textContent = fmtTime(position);
  }
  function removeHistoryEntry(videoId) {
    saveHistory(loadHistory().filter(e => e.videoId !== videoId));
    renderHistory();
  }

  function renderHistory() {
    const h = loadHistory();
    historyList.innerHTML = '';
    if (!h.length) {
      historySection.hidden = true;
      return;
    }
    historySection.hidden = false;
    for (const entry of h) {
      const li = document.createElement('li');
      li.className = 'history-item' + (entry.videoId === currentVideoId ? ' active' : '');
      li.dataset.videoId = entry.videoId;

      const title = document.createElement('div');
      title.className = 'h-title';
      title.textContent = entry.title || entry.videoId;

      const pos = document.createElement('span');
      pos.className = 'h-pos';
      pos.textContent = fmtTime(entry.position || 0);

      const del = document.createElement('button');
      del.type = 'button';
      del.className = 'h-del';
      del.setAttribute('aria-label', 'Remove from history');
      del.textContent = '×';
      del.addEventListener('click', (e) => {
        e.stopPropagation();
        removeHistoryEntry(entry.videoId);
      });

      li.addEventListener('click', () => resumeFromHistory(entry));
      li.append(title, pos, del);
      historyList.append(li);
    }
  }

  function savePositionIfPlaying(force = false) {
    if (!currentVideoId) return;
    const now = Date.now();
    if (!force && now - lastPositionSaveAt < 3000) return;
    lastPositionSaveAt = now;
    const t = Math.floor(audio.currentTime || 0);
    updateHistoryPosition(currentVideoId, t);
  }

  // ---- Playback ----
  async function playVideo(url, startOverride) {
    playBtn.disabled = true;
    statusEl.textContent = 'resolving…';

    let first;
    try {
      const res = await fetch('/api/play', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url }),
      });
      if (!res.ok) {
        const err = await safeJson(res);
        statusEl.textContent = 'error: ' + (err.error || res.status);
        playBtn.disabled = false;
        return;
      }
      first = await res.json();
    } catch {
      statusEl.textContent = 'network error';
      playBtn.disabled = false;
      return;
    }

    const videoId = first.videoId;
    const startAt = (startOverride != null) ? startOverride : (first.startSeconds || 0);
    currentVideoId = videoId;
    playerTitle.textContent = first.title || 'Loading…';

    if (first.title) upsertHistory({ videoId, title: first.title });

    if (first.state === 'ready') {
      startPlayback({ videoId, title: first.title, startSeconds: startAt });
      playBtn.disabled = false;
      return;
    }
    if (first.state === 'error') {
      statusEl.textContent = 'error: ' + (first.error || 'unknown');
      playBtn.disabled = false;
      return;
    }

    statusEl.textContent = 'downloading…';
    const deadline = Date.now() + 120_000;
    while (Date.now() < deadline) {
      await sleep(1000);
      let st;
      try {
        st = await fetch(`/api/status?videoId=${encodeURIComponent(videoId)}`).then(r => r.json());
      } catch { continue; }
      if (st.state === 'ready') {
        if (st.title) upsertHistory({ videoId, title: st.title });
        startPlayback({ videoId, title: st.title, startSeconds: startAt });
        playBtn.disabled = false;
        return;
      }
      if (st.state === 'error') {
        statusEl.textContent = 'error: ' + (st.error || 'unknown');
        playBtn.disabled = false;
        return;
      }
    }
    statusEl.textContent = 'timeout';
    playBtn.disabled = false;
  }

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    const url = urlInput.value.trim();
    if (!url) return;
    playVideo(url);
  });

  async function resumeFromHistory(entry) {
    // Persist current position before switching.
    savePositionIfPlaying(true);
    audio.pause();
    urlInput.value = `https://youtu.be/${entry.videoId}`;
    await playVideo(urlInput.value, entry.position || 0);
  }

  async function safeJson(res) {
    try { return await res.json(); } catch { return {}; }
  }

  function startPlayback({ videoId, title, startSeconds }) {
    statusEl.textContent = '';
    playerSection.hidden = false;
    playerTitle.textContent = title || videoId;
    audio.src = `/media/${videoId}.m4a`;
    const offset = Number(startSeconds) || 0;
    if (offset > 0) {
      const applyOffset = () => {
        try { audio.currentTime = offset; } catch {}
      };
      if (isFinite(audio.duration) && audio.readyState >= 1) {
        applyOffset();
      } else {
        audio.addEventListener('loadedmetadata', applyOffset, { once: true });
      }
    }
    audio.play().catch(() => {});
    setMediaSession(title || videoId);
    renderHistory();
  }

  function setMediaSession(title) {
    if (!('mediaSession' in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({ title, artist: 'Sleepcast' });
    navigator.mediaSession.setActionHandler('play', () => audio.play());
    navigator.mediaSession.setActionHandler('pause', () => audio.pause());
    navigator.mediaSession.setActionHandler('seekbackward', (d) => {
      audio.currentTime = Math.max(0, audio.currentTime - (d.seekOffset || 15));
    });
    navigator.mediaSession.setActionHandler('seekforward', (d) => {
      audio.currentTime = Math.min(audio.duration || 0, audio.currentTime + (d.seekOffset || 15));
    });
  }

  playPause.addEventListener('click', () => {
    if (audio.paused) audio.play(); else audio.pause();
  });
  audio.addEventListener('play', () => {
    playPause.textContent = 'Pause';
    playPause.setAttribute('aria-label', 'Pause');
  });
  audio.addEventListener('pause', () => {
    playPause.textContent = 'Play';
    playPause.setAttribute('aria-label', 'Play');
    savePositionIfPlaying(true);
  });

  document.querySelectorAll('[data-seek]').forEach(btn => {
    btn.addEventListener('click', () => {
      const d = parseInt(btn.dataset.seek, 10);
      const dur = audio.duration || 0;
      audio.currentTime = Math.min(dur, Math.max(0, audio.currentTime + d));
      savePositionIfPlaying(true);
    });
  });

  audio.addEventListener('timeupdate', () => {
    if (!scrubber.dragging) curTime.textContent = fmtTime(audio.currentTime);
    if (!isFinite(audio.duration)) return;
    scrubber.max = Math.floor(audio.duration);
    if (!scrubber.dragging) scrubber.value = Math.floor(audio.currentTime);
    savePositionIfPlaying();
  });
  audio.addEventListener('loadedmetadata', () => {
    durTime.textContent = fmtTime(audio.duration);
    scrubber.max = Math.floor(audio.duration || 0);
  });
  scrubber.addEventListener('pointerdown', () => { scrubber.dragging = true; });
  scrubber.addEventListener('pointerup', () => {
    scrubber.dragging = false;
    savePositionIfPlaying(true);
  });
  scrubber.addEventListener('input', () => {
    scrubber.dragging = true;
    const v = parseFloat(scrubber.value);
    curTime.textContent = fmtTime(v);
    audio.currentTime = v;
  });
  scrubber.addEventListener('change', () => {
    scrubber.dragging = false;
    savePositionIfPlaying(true);
  });

  audio.addEventListener('ended', async () => {
    if (!currentVideoId) return;
    updateHistoryPosition(currentVideoId, 0);
    try {
      await fetch('/api/finished', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ videoId: currentVideoId }),
      });
      showToast('Audio cleaned up on server.');
    } catch {}
  });

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') savePositionIfPlaying(true);
  });
  window.addEventListener('pagehide', () => savePositionIfPlaying(true));

  sleepSelect.addEventListener('change', () => {
    if (sleepTimer) { clearTimeout(sleepTimer); sleepTimer = null; }
    if (sleepTick) { clearInterval(sleepTick); sleepTick = null; }
    sleepCountdown.textContent = '';
    const secs = parseInt(sleepSelect.value, 10);
    if (!secs) return;
    sleepDeadline = Date.now() + secs * 1000;
    sleepTimer = setTimeout(() => {
      audio.pause();
      sleepCountdown.textContent = 'sleep timer fired';
      sleepSelect.value = '0';
      clearInterval(sleepTick);
    }, secs * 1000);
    const render = () => {
      const left = Math.max(0, Math.floor((sleepDeadline - Date.now()) / 1000));
      sleepCountdown.textContent = `stops in ${fmtTime(left)}`;
    };
    render();
    sleepTick = setInterval(render, 1000);
  });

  clearHistoryBtn.addEventListener('click', () => {
    saveHistory([]);
    renderHistory();
  });

  renderHistory();

  const initialHistory = loadHistory();
  if (initialHistory.length === 1) {
    resumeFromHistory(initialHistory[0]);
  }
})();
