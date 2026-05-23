import type { ExtensionAPI, ExtensionCommandContext, ExecOptions } from "@earendil-works/pi-coding-agent";
import { Container, Text } from "@earendil-works/pi-tui";
import { basename } from "node:path";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";

interface PiWebState {
  pid: number;
  port: string;
  host: string;
  tailscale: boolean;
  startedAt: string;
}

async function detectHostPort(pi: ExtensionAPI): Promise<{ host: string; port: string; tailscale: boolean } | null> {
  // 1. Try pidfile
  try {
    const path = `${homedir()}/.pi/agent/pi-web-state.json`;
    const raw = readFileSync(path, "utf-8");
    const state: PiWebState = JSON.parse(raw);

    // Validate PID is still alive
    try {
      process.kill(state.pid, 0);
    } catch {
      // stale pidfile, fall through
    }

    return { host: state.host, port: state.port, tailscale: state.tailscale };
  } catch {
    // fall through
  }

  // 2. Process fallback (macOS / Linux)
  if (process.platform !== "win32") {
    try {
      const result = await pi.exec("pgrep", ["-a", "pi-web"]);
      const line = result.stdout.trim().split("\n")[0];
      if (line) {
        const parts = line.split(/\s+/);
        const args = parts.slice(1);
        let port = "31415";
        let host = "127.0.0.1";
        for (let i = 0; i < args.length; i++) {
          if ((args[i] === "-p" || args[i] === "--port") && args[i + 1]) {
            port = args[i + 1];
            i++;
          }
          if ((args[i] === "--host") && args[i + 1]) {
            host = args[i + 1];
            i++;
          }
        }
        return { host, port, tailscale: isTailscaleHost(host) };
      }
    } catch {
      // fall through
    }
  }

  // 3. Default fallback
  return { host: "127.0.0.1", port: "31415", tailscale: false };
}

function isTailscaleHost(host: string): boolean {
  const ip = host.split(":")[0];
  const parts = ip.split(".").map(Number);
  if (parts.length === 4) {
    return parts[0] === 100 && parts[1] >= 64 && parts[1] <= 127;
  }
  return ip.toLowerCase().startsWith("fd7a:115c:a1e0");
}

async function healthCheck(host: string, port: string): Promise<boolean> {
  try {
    const res = await fetch(`http://${host}:${port}`, { signal: AbortSignal.timeout(1000) });
    // 401/403 means pi-web is running with auth enabled.
    return res.ok || res.status === 401 || res.status === 403;
  } catch {
    return false;
  }
}

function readPiWebToken(): string | null {
  try {
    const raw = readFileSync(`${homedir()}/.config/pi-web/env`, "utf-8");
    const match = raw.match(/^PI_WEB_TOKEN=(.*)$/m);
    return match?.[1]?.trim() || null;
  } catch {
    return null;
  }
}

function withToken(url: string): string {
  const token = readPiWebToken();
  if (!token) return url;
  const separator = url.includes("?") ? "&" : "?";
  return `${url}${separator}token=${encodeURIComponent(token)}`;
}

async function ensureQrCode(pi: ExtensionAPI, ctx: ExtensionCommandContext): Promise<boolean> {
  // Already available?
  try {
    await import("qrcode");
    return true;
  } catch {
    // Not available, try to install
  }

  // Find the extension directory with a package.json that depends on qrcode
  const candidates = [
    `${homedir()}/.pi/agent/extensions/pi-web/`,
    `${homedir()}/.pi/agent/extensions/`,
    `${ctx.sessionManager.getCwd()}/.pi/extensions/`,
  ];

  let extDir: string | null = null;
  for (const dir of candidates) {
    try {
      const pkg = JSON.parse(readFileSync(`${dir}package.json`, "utf-8"));
      if (pkg.dependencies?.qrcode || pkg.devDependencies?.qrcode) {
        extDir = dir;
        break;
      }
    } catch {
      // continue
    }
  }

  if (!extDir) {
    return false;
  }

  ctx.ui.notify("Installing qrcode dependency...", "info");
  try {
    await pi.exec("npm", ["install"], { cwd: extDir } as ExecOptions);
    // Verify it works now
    await import("qrcode");
    return true;
  } catch {
    return false;
  }
}

function openBrowser(pi: ExtensionAPI, url: string): Promise<void> {
  let cmd: string;
  let args: string[];
  switch (process.platform) {
    case "darwin":
      cmd = "open";
      args = [url];
      break;
    case "win32":
      cmd = "cmd";
      args = ["/c", "start", url];
      break;
    default:
      cmd = "xdg-open";
      args = [url];
      break;
  }
  return pi.exec(cmd, args).then(() => {});
}

export default function (pi: ExtensionAPI) {
  // ── /web ──────────────────────────────────────────────────────────
  pi.registerCommand("web", {
    description: "Open current session in pi-web browser",
    handler: async (_args, ctx: ExtensionCommandContext) => {
      const sessionFile = ctx.sessionManager.getSessionFile();
      if (!sessionFile) {
        ctx.ui.notify("Cannot view an in-memory session.", "error");
        return;
      }

      const detected = await detectHostPort(pi);
      if (!detected) {
        ctx.ui.notify("Could not detect pi-web server. Start it with: pi-web -o", "error");
        return;
      }

      const { host, port } = detected;
      if (!(await healthCheck(host, port))) {
        ctx.ui.notify(`pi-web not responding on ${host}:${port}. Start it with: pi-web -o`, "error");
        return;
      }

      const sessionId = basename(sessionFile);
      const url = withToken(`http://${host}:${port}/session?id=${encodeURIComponent(sessionId)}`);

      try {
        await openBrowser(pi, url);
        ctx.ui.notify("Opened session in browser", "success");
      } catch {
        ctx.ui.notify(`Failed to open browser. Visit ${url} manually.`, "warning");
      }
    },
  });

  // ── /mobile ───────────────────────────────────────────────────────
  pi.registerCommand("mobile", {
    description: "Show QR code for mobile Tailscale access",
    handler: async (_args, ctx: ExtensionCommandContext) => {
      const sessionFile = ctx.sessionManager.getSessionFile();
      if (!sessionFile) {
        ctx.ui.notify("Cannot view an in-memory session.", "error");
        return;
      }

      const detected = await detectHostPort(pi);
      if (!detected) {
        ctx.ui.notify("Could not detect pi-web server. Start it with: pi-web -o", "error");
        return;
      }

      const { host, port, tailscale } = detected;
      if (!(await healthCheck(host, port))) {
        ctx.ui.notify(`pi-web not responding on ${host}:${port}. Start it with: pi-web -o`, "error");
        return;
      }

      if (!tailscale) {
        ctx.ui.notify(
          "pi-web is not running on a Tailscale IP. " +
            "Install Tailscale, then start with: PI_WEB_TOKEN=... pi-web --host $(tailscale ip -4)",
          "error"
        );
        return;
      }

      const sessionId = basename(sessionFile);
      const url = withToken(`http://${host}:${port}/session?id=${encodeURIComponent(sessionId)}`);

      // Ensure qrcode is available (auto-install on first use)
      const hasQr = await ensureQrCode(pi, ctx);

      if (hasQr && ctx.hasUI) {
        const QRCode = await import("qrcode");
        const qrText = await QRCode.toString(url, { type: "utf8", margin: 2 });

        ctx.ui.setWidget(
          "pi-web-mobile-qr",
          (_tui, _theme) => {
            const container = new Container();
            container.addChild(
              new Text(
                `Mobile access via Tailscale\n\nMake sure your phone is connected to Tailscale, then scan this QR code.\n\n${qrText}\n\n${url}`,
                1,
                1
              )
            );
            return container;
          },
          { placement: "belowEditor" }
        );
        ctx.ui.notify("QR code shown below the editor. Make sure your phone is connected to Tailscale.", "info");
      } else {
        ctx.ui.notify(
          `QR code unavailable. Make sure your phone is connected to Tailscale, then open this URL: ${url}`,
          "warning"
        );
      }
    },
  });

  // ── /refresh ──────────────────────────────────────────────────────
  pi.registerCommand("refresh", {
    description: "Sync mobile-written messages back into this session",
    handler: async (_args, ctx: ExtensionCommandContext) => {
      const sessionFile = ctx.sessionManager.getSessionFile();
      if (!sessionFile) {
        ctx.ui.notify("Cannot refresh an in-memory session.", "error");
        return;
      }

      let fileEntries: unknown[] = [];
      try {
        const raw = readFileSync(sessionFile, "utf-8");
        fileEntries = raw
          .split("\n")
          .map((line) => line.trim())
          .filter((line) => line.length > 0)
          .map((line) => JSON.parse(line));
      } catch (err) {
        ctx.ui.notify(`Failed to read session file: ${err}`, "error");
        return;
      }

      const currentCount = ctx.sessionManager.getEntries().length;
      // fileEntries includes header, so subtract 1 for message entries
      const fileCount = Math.max(0, fileEntries.length - 1);

      if (fileCount > currentCount) {
        const delta = fileCount - currentCount;
        ctx.ui.notify(`Mobile added ${delta} new message(s). Reloading session...`, "info");
        await ctx.switchSession(sessionFile);
      } else {
        ctx.ui.notify("Session is up to date.", "info");
      }
    },
  });
}
