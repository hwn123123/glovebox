'use strict';

// The upstream NetProxy terminal normally runs inside KernelSU's privileged
// WebView and calls ksu.exec(). Glovebox serves it to authenticated LAN clients,
// so provide the same callback contract through a narrow HTTP bridge. The
// server only accepts netproxyctl commands; the terminal's "! shell" extension
// remains deliberately unavailable over the network.
(function installGloveboxBridge() {
  async function invoke(command, callbackName) {
    let errno = 1;
    let stdout = '';
    let stderr = '';
    try {
      const response = await fetch('/api/proxies/terminal', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-Glovebox': '1' },
        body: JSON.stringify({ command }),
      });
      const body = await response.json().catch(() => ({}));
      errno = Number.isInteger(body.errno) ? body.errno : (response.ok ? 0 : 1);
      stdout = typeof body.stdout === 'string' ? body.stdout : '';
      stderr = typeof body.stderr === 'string' ? body.stderr : '';
      if (!response.ok && !stderr) {
        stderr = body.error || (response.status === 401
          ? 'The Glovebox session expired. Return to the main console and log in again.'
          : `Glovebox command bridge returned HTTP ${response.status}`);
      }
    } catch (error) {
      errno = 1;
      stderr = error instanceof Error ? error.message : String(error);
    }
    const callback = window[callbackName];
    if (typeof callback === 'function') callback(errno, stdout, stderr);
  }

  window.ksu = {
    exec(command, _options, callbackName) {
      void invoke(String(command || ''), String(callbackName || ''));
    },
  };
  window.KSU = window.ksu;
}());
