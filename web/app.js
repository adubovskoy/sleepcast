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

  let currentVideoId = null;
  let sleepTimer = null;
  let sleepDeadline = 0;
  let sleepTick = null;

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

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const url = urlInput.value.trim();
    if (!url) return;
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
    } catch (e) {
      statusEl.textContent = 'network error';
      playBtn.disabled = false;
      return;
    }

    currentVideoId = first.videoId;
    playerTitle.textContent = first.title || 'Loading…';
    const startAt = first.startSeconds || 0;

    if (first.state === 'ready') {
      startPlayback({ ...first, startSeconds: startAt });
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
        st = await fetch(`/api/status?videoId=${encodeURIComponent(currentVideoId)}`).then(r => r.json());
      } catch { continue; }
      if (st.state === 'ready') {
        startPlayback({ ...st, startSeconds: startAt });
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
  });

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
  });

  document.querySelectorAll('[data-seek]').forEach(btn => {
    btn.addEventListener('click', () => {
      const d = parseInt(btn.dataset.seek, 10);
      const dur = audio.duration || 0;
      audio.currentTime = Math.min(dur, Math.max(0, audio.currentTime + d));
    });
  });

  audio.addEventListener('timeupdate', () => {
    curTime.textContent = fmtTime(audio.currentTime);
    if (!isFinite(audio.duration)) return;
    scrubber.max = Math.floor(audio.duration);
    if (!scrubber.dragging) scrubber.value = Math.floor(audio.currentTime);
  });
  audio.addEventListener('loadedmetadata', () => {
    durTime.textContent = fmtTime(audio.duration);
    scrubber.max = Math.floor(audio.duration || 0);
  });
  scrubber.addEventListener('pointerdown', () => { scrubber.dragging = true; });
  scrubber.addEventListener('pointerup', () => { scrubber.dragging = false; });
  scrubber.addEventListener('input', () => { audio.currentTime = parseFloat(scrubber.value); });

  audio.addEventListener('ended', async () => {
    if (!currentVideoId) return;
    try {
      await fetch('/api/finished', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ videoId: currentVideoId }),
      });
      showToast('Audio cleaned up on server.');
    } catch {}
  });

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
})();
