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
  const stage = document.getElementById("stage");

  let pc = null;
  let dc = null;
  let connecting = false;

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
      ev.preventDefault();
      sendEvent({ t: type, k: ev.code, r: ev.repeat });
    };

    video.addEventListener("pointermove", onMove);
    video.addEventListener("pointerdown", onDown);
    video.addEventListener("pointerup", onUp);
    video.addEventListener("wheel", onWheel, { passive: false });
    video.addEventListener("keydown", onKey("kd"));
    video.addEventListener("keyup", onKey("ku"));
    video.addEventListener("contextmenu", (ev) => ev.preventDefault());
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

    pc.ontrack = (ev) => {
      video.srcObject = ev.streams[0] || new MediaStream([ev.track]);
      video.classList.add("live");
      placeholder.classList.add("hidden");
      focusHint.hidden = false;
      video.focus();
    };
    pc.onconnectionstatechange = () => {
      const state = pc?.connectionState;
      if (state === "connected") {
        setStatus("connected", "connected");
        placeholderHint.textContent = "Connected";
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
