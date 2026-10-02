const { spawn } = require("node:child_process");
const path = require("node:path");
const { setTimeout: delay } = require("node:timers/promises");
const root = path.resolve(__dirname, "../..");
async function main() {
  let host;
  if (!process.env.PANELTREE_BROWSER_URL) {
    host = spawn(
      "go",
      [
        "test",
        "./internal/web",
        "-run",
        "^TestBrowserHost$",
        "-v",
        "-timeout=10m",
      ],
      {
        cwd: root,
        env: { ...process.env, PANELTREE_WEB_TEST: "1" },
        stdio: "inherit",
        detached: process.platform !== "win32",
      },
    );
  }
  let launchError;
  host?.on("error", (error) => {
    launchError = error;
  });
  try {
    const base = process.env.PANELTREE_BROWSER_URL || "http://127.0.0.1:8910";
    const deadline = Date.now() + 90000;
    for (;;) {
      if (launchError) throw launchError;
      if (host && host.exitCode !== null)
        throw new Error("Browser fixture exited");
      try {
        const r = await fetch(base + "/api/session");
        if (r.ok) break;
      } catch {}
      if (Date.now() > deadline)
        throw new Error("Browser fixture did not become ready");
      await delay(250);
    }
    const child = spawn(
      process.execPath,
      ["--test", path.join(__dirname, "workspace.cjs")],
      {
        cwd: path.join(root, "web"),
        env: { ...process.env, PANELTREE_BROWSER_URL: base },
        stdio: "inherit",
      },
    );
    const code = await new Promise((resolve, reject) => {
      child.on("error", reject);
      child.on("exit", resolve);
    });
    process.exitCode = code ?? 1;
  } finally {
    if (host?.pid) {
      if (process.platform === "win32") {
        spawn("taskkill", ["/pid", String(host.pid), "/t", "/f"], {
          stdio: "ignore",
        });
      } else {
        try {
          process.kill(-host.pid, "SIGTERM");
        } catch (error) {
          if (error.code !== "ESRCH") throw error;
        }
      }
    }
  }
}
main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
