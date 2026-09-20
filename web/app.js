(() => {
  const video = document.getElementById("screen");
  const statusEl = document.getElementById("status");
  const banner = document.getElementById("banner");
  const placeholder = document.getElementById("placeholder");
  const placeholderHint = document.getElementById("placeholder-hint");
  const focusHint = document.getElementById("focus-hint");
  const btnConnect = document.getElementById("btn-connect");
  const btnDisconnect = document.getElementById("btn-disconnect");
  const btnFs = document.getElementById("btn-fs");
  const btnClip = document.getElementById("btn-clip");
  const stage = document.getElementById("stage");

  const MAX_CLIP_BYTES = 1024 * 1024;
  const LOCK_HINT = "If the screen is locked, click once then type your Mac password and press Enter (picture stays black until unlocked).";

  let pc = null;
  let dc = null;
  let connecting = false;
  let lastSentClip = "";
  let lastAppliedClip = "";
  let pendingLocalClip = "";
  let clipReadDenied = false;
  let clipTimer = null;
  let lastRemotePasteAt = 0;

  const clipSink = document.createElement("textarea");
  clipSink.setAttribute("aria-hidden", "true");
  clipSink.tabIndex = -1;
  clipSink.style.cssText = "position:fixed;left:-9999px;top:0;opacity:0;height:1px;width:1px";
  document.body.appendChild(clipSink);

  const setStatus = (state, label) => {
    statusEl.dataset.state = state;
    statusEl.className = `pill ${state}`;
    statusEl.textContent = label || state;
  };

  const showBanner = (text) => {
    if (!text) {
      banner.hidden = true;
      banner.textContent = "";
      return;
    }
    banner.hidden = false;
    banner.textContent = text;
  };

  const iceComplete = (peer) =>
    new Promise((resolve) => {
      if (peer.iceGatheringState === "complete") {
        resolve();
        return;
      }
      const onChange = () => {
        if (peer.iceGatheringState === "complete") {
          peer.removeEventListener("icegatheringstatechange", onChange);
          resolve();
        }
      };
      peer.addEventListener("icegatheringstatechange", onChange);
      setTimeout(resolve, 4000);
    });

  const sendEvent = (payload) => {
    if (!dc || dc.readyState !== "open") return;
    dc.send(JSON.stringify(payload));
  };

  const utf8Len = (s) => new TextEncoder().encode(s).length;

  const hideClipApply = () => {
    pendingLocalClip = "";
    if (btnClip) btnClip.hidden = true;
  };

  const showClipApply = (text) => {
    pendingLocalClip = text;
    if (btnClip) btnClip.hidden = false;
  };

  const sessionLive = () => !!(dc && dc.readyState === "open");

  const isHudControl = (el) =>
    el === btnConnect || el === btnDisconnect || el === btnFs || el === btnClip;

  const enableRemoteKeyboard = () => {
    focusHint.textContent = LOCK_HINT;
    focusHint.hidden = false;
    video.tabIndex = 0;
    video.focus({ preventScroll: true });
  };

  // Black frames are still a live remote surface: accept keys without waiting
  // for a "looking live" picture (lock-screen capture is black by design).
  const revealRemoteSurface = () => {
    placeholder.classList.add("hidden");
    video.classList.add("live");
    enableRemoteKeyboard();
  };

  const sendClipboard = (text) => {
    if (!sessionLive() || typeof text !== "string" || text === "") return;
    if (utf8Len(text) > MAX_CLIP_BYTES) {
      console.warn("clipboard text exceeds 1MB; dropping");
      return;
    }
    if (text === lastSentClip) return;
    lastSentClip = text;
    sendEvent({ t: "cb", d: text });
  };

  const execCopy = (text) => {
    clipSink.value = text;
    clipSink.focus({ preventScroll: true });
    clipSink.select();
    let ok = false;
    try {
      ok = document.execCommand("copy");
    } catch (_) {
      ok = false;
    }
    video.focus();
    return ok;
  };

  const writeLocalClipboard = async (text) => {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      try {
        await navigator.clipboard.writeText(text);
        hideClipApply();
        return true;
      } catch (_) {
        /* need a user gesture */
      }
    }
    if (execCopy(text)) {
      hideClipApply();
      return true;
    }
    showClipApply(text);
    return false;
  };

  const applyRemoteClipboard = (text) => {
    if (typeof text !== "string" || text === "") return;
    if (utf8Len(text) > MAX_CLIP_BYTES) {
      console.warn("remote clipboard exceeds 1MB; dropping");
      return;
    }
    lastAppliedClip = text;
    lastSentClip = text; // do not echo this text back to the Mac
    writeLocalClipboard(text);
  };

  const applyPendingClip = () => {
    if (!pendingLocalClip) return;
    const text = pendingLocalClip;
    writeLocalClipboard(text);
  };

  const readAndSendLocal = async () => {
    if (!sessionLive() || clipReadDenied) return;
    if (!navigator.clipboard || !navigator.clipboard.readText) return;
    try {
      const text = await navigator.clipboard.readText();
      if (text && text !== lastAppliedClip) sendClipboard(text);
    } catch (err) {
      if (err && (err.name === "NotAllowedError" || err.name === "SecurityError")) {
        clipReadDenied = true;
      }
    }
  };

  const injectMacCommand = (keyCode) => {
    // Linux Ctrl+C/V must become Cmd+C/V on the Mac. Release Control first so
    // the host does not see Control+Command.
    sendEvent({ t: "ku", k: "ControlLeft" });
    sendEvent({ t: "ku", k: "ControlRight" });
    sendEvent({ t: "kd", k: "MetaLeft" });
    sendEvent({ t: "kd", k: keyCode });
    sendEvent({ t: "ku", k: keyCode });
    sendEvent({ t: "ku", k: "MetaLeft" });
  };

  const isClipKey = (ev) =>
    (ev.ctrlKey || ev.metaKey) &&
    !ev.altKey &&
    !ev.shiftKey &&
    (ev.code === "KeyC" || ev.code === "KeyX" || ev.code === "KeyV");

  const pasteToRemote = (text) => {
    sendClipboard(text);
    const now = Date.now();
    if (now - lastRemotePasteAt < 250) return;
    lastRemotePasteAt = now;
    injectMacCommand("KeyV");
  };

  const startClipPoll = () => {
    stopClipPoll();
    clipTimer = setInterval(() => {
      if (document.hasFocus()) readAndSendLocal();
    }, 2000);
  };

  const stopClipPoll = () => {
    if (clipTimer) {
      clearInterval(clipTimer);
      clipTimer = null;
    }
  };

  const handleIncoming = (raw) => {
    let msg;
    try {
      msg = JSON.parse(raw);
    } catch (_) {
      return;
    }
    if (msg && msg.t === "cb" && typeof msg.d === "string") {
      applyRemoteClipboard(msg.d);
    }
  };

  const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);

  // object-fit:contain letterboxes the decoded picture *inside* the <video>
  // box. Normalize against videoWidth x videoHeight, not the element rect.
  const containNorm = (px, py, boxW, boxH, srcW, srcH) => {
    if (boxW <= 0 || boxH <= 0) return { x: 0, y: 0 };
    if (srcW <= 0 || srcH <= 0) {
      return { x: clamp01(px / boxW), y: clamp01(py / boxH) };
    }
    const scale = Math.min(boxW / srcW, boxH / srcH);
    const w = srcW * scale;
    const h = srcH * scale;
    const left = (boxW - w) / 2;
    const top = (boxH - h) / 2;
    return {
      x: clamp01((px - left) / w),
      y: clamp01((py - top) / h),
    };
  };

  const point = (ev) => {
    const rect = video.getBoundingClientRect();
    return containNorm(
      ev.clientX - rect.left,
      ev.clientY - rect.top,
      rect.width,
      rect.height,
      video.videoWidth,
      video.videoHeight,
    );
  };

  const attachInput = () => {
    const onMove = (ev) => {
      const { x, y } = point(ev);
      sendEvent({ t: "mm", x, y });
    };
    const onDown = (ev) => {
      ev.preventDefault();
      video.focus();
      video.setPointerCapture?.(ev.pointerId);
      const { x, y } = point(ev);
      sendEvent({ t: "md", b: ev.button, x, y });
    };
    const onUp = (ev) => {
      ev.preventDefault();
      const { x, y } = point(ev);
      sendEvent({ t: "mu", b: ev.button, x, y });
    };
    const onWheel = (ev) => {
      ev.preventDefault();
      const { x, y } = point(ev);
      sendEvent({ t: "wh", x, y, dx: ev.deltaX, dy: ev.deltaY });
    };
    const onKey = (type) => (ev) => {
      if (!sessionLive()) return;
      if (isHudControl(ev.target) || ev.target === clipSink) return;
      if (isClipKey(ev)) {
        if (ev.code === "KeyV") {
          // Do not preventDefault: the paste event is the reliable HTTP path.
          if (type === "kd" && !ev.repeat) readAndSendLocal();
          return;
        }
        ev.preventDefault();
        if (type === "kd" && !ev.repeat) {
          // Copy/cut on the Mac; pasteboard watcher pushes text back here.
          injectMacCommand(ev.code);
        }
        return;
      }
      ev.preventDefault();
      sendEvent({ t: type, k: ev.code, r: ev.repeat });
    };

    video.addEventListener("pointermove", onMove);
    video.addEventListener("pointerdown", (ev) => {
      if (pendingLocalClip) applyPendingClip();
      onDown(ev);
    });
    video.addEventListener("pointerup", onUp);
    video.addEventListener("wheel", onWheel, { passive: false });
    // Page-level keys so a black / unfocused video still forwards typing.
    document.addEventListener("keydown", onKey("kd"));
    document.addEventListener("keyup", onKey("ku"));
    video.addEventListener("contextmenu", (ev) => ev.preventDefault());
    stage.addEventListener("pointerdown", (ev) => {
      if (!sessionLive() || isHudControl(ev.target)) return;
      video.focus({ preventScroll: true });
    });
  };

  const teardown = (status, label) => {
    if (dc) {
      try { dc.close(); } catch (_) { /* ignore */ }
      dc = null;
    }
    if (pc) {
      pc.ontrack = null;
      pc.onconnectionstatechange = null;
      try { pc.close(); } catch (_) { /* ignore */ }
      pc = null;
    }
    video.srcObject = null;
    video.classList.remove("live");
    placeholder.classList.remove("hidden");
    connecting = false;
    btnConnect.hidden = false;
    btnDisconnect.hidden = true;
    focusHint.hidden = true;
    stopClipPoll();
    hideClipApply();
    lastSentClip = "";
    lastAppliedClip = "";
    clipReadDenied = false;
    setStatus(status || "idle", label || "idle");
  };

  const connect = async () => {
    if (connecting || pc) return;
    connecting = true;
    showBanner("");
    setStatus("connecting", "connecting");
    placeholderHint.textContent = "Negotiating WebRTC…";
    btnConnect.hidden = true;
    btnDisconnect.hidden = false;

    const iceServers = [{ urls: "stun:stun.l.google.com:19302" }];
    pc = new RTCPeerConnection({ iceServers });
    pc.addTransceiver("video", { direction: "recvonly" });
    dc = pc.createDataChannel("input", { ordered: true });
    dc.onopen = () => {
      sendEvent({ t: "cb-req" });
      startClipPoll();
      readAndSendLocal();
      revealRemoteSurface();
    };
    dc.onmessage = (ev) => handleIncoming(ev.data);
    dc.onclose = stopClipPoll;

    pc.ontrack = (ev) => {
      video.srcObject = ev.streams[0] || new MediaStream([ev.track]);
      revealRemoteSurface();
    };
    pc.onconnectionstatechange = () => {
      const state = pc?.connectionState;
      if (state === "connected") {
        setStatus("connected", "connected");
        placeholderHint.textContent = "Connected";
        revealRemoteSurface();
      } else if (state === "failed") {
        showBanner("WebRTC connection failed. Check that you can reach the Mac on UDP and that Screen Recording is allowed.");
        teardown("error", "failed");
      } else if (state === "disconnected") {
        setStatus("connecting", "reconnecting");
      } else if (state === "closed") {
        teardown("idle", "idle");
      }
    };

    try {
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      await iceComplete(pc);

      const res = await fetch("/api/signal", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sdp: pc.localDescription.sdp,
          type: pc.localDescription.type,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (res.status === 409) {
        showBanner("Another client is already connected. Only one remote session is allowed.");
        teardown("busy", "busy");
        return;
      }
      if (!res.ok) {
        showBanner(body.error || `Signaling failed (${res.status})`);
        teardown("error", "error");
        return;
      }
      await pc.setRemoteDescription({ type: body.type, sdp: body.sdp });
      connecting = false;
    } catch (err) {
      showBanner(err.message || String(err));
      teardown("error", "error");
    }
  };

  video.tabIndex = 0;
  attachInput();

  document.addEventListener("paste", (ev) => {
    if (!sessionLive()) return;
    const text = ev.clipboardData ? ev.clipboardData.getData("text/plain") : "";
    if (!text) return;
    ev.preventDefault();
    pasteToRemote(text);
  });
  document.addEventListener("copy", () => setTimeout(readAndSendLocal, 0));
  document.addEventListener("cut", () => setTimeout(readAndSendLocal, 0));
  window.addEventListener("focus", readAndSendLocal);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") readAndSendLocal();
  });
  btnClip.addEventListener("click", applyPendingClip);

  btnConnect.addEventListener("click", connect);
  btnDisconnect.addEventListener("click", () => {
    showBanner("");
    teardown("idle", "idle");
  });
  btnFs.addEventListener("click", async () => {
    if (!document.fullscreenElement) {
      await stage.requestFullscreen?.();
    } else {
      await document.exitFullscreen?.();
    }
  });

  connect();
})();
