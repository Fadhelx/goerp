#!/usr/bin/env node
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { createServer } from "node:net";
import { existsSync } from "node:fs";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const DEFAULT_TIMEOUT_MS = 15000;
const DEFAULT_BASE_URL = "http://127.0.0.1:8069";
const DEFAULT_OUT_DIR = "reports/web_visual_smoke";
let navigationCounter = 0;

export const scenarios = [
  {
    name: "launcher-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openWeb(page, config, desktopViewport());
      const appCount = await waitForCount(page, "#appGrid .o_app", 2, "launcher app tiles");
      const systrayCount = await waitForCount(page, ".o_menu_systray [role='menuitem']", 3, "systray entries");
      return { app_count: appCount, systray_count: systrayCount };
    }
  },
  {
    name: "settings-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openWeb(page, config, desktopViewport());
      await clickText(page, "#appGrid .o_app", "Settings");
      await waitFor(page, `document.body.dataset.view === "settings"`, "settings view");
      const blockCount = await waitForCount(page, "#settingsBlocks .app_settings_block", 1, "settings blocks");
      const boxCount = await waitForCount(page, "#settingsBlocks .o_setting_box", 1, "settings boxes");
      return { settings_blocks: blockCount, setting_boxes: boxCount };
    }
  },
  {
    name: "default-webclient-takeover",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "default TS takeover document ready");
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "TS webclient ready");
      const navCount = await waitForCount(page, ".o_web_client .o_main_navbar", 1, "TS navbar");
      const appCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "TS app tiles");
      const actionCount = await waitForCount(page, ".o_web_client .o_action_manager", 1, "TS action manager");
      const hasShellCue = await evaluate(page, `document.body.textContent.includes("Gorp") || document.body.textContent.includes("GoERP")`);
      if (hasShellCue) throw new Error("TS takeover exposes non-Odoo shell cue");
      return { nav_count: navCount, app_count: appCount, action_count: actionCount };
    }
  },
  {
    name: "default-webclient-action-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "TS webclient ready");
      await clickText(page, ".o_web_client .o_home_menu .o_app", "Settings");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS action ready");
      const windowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action", 1, "TS window action");
      const controlPanelCount = await waitForCount(page, ".o_web_client .o_action_manager .o_control_panel", 1, "TS action control panel");
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=res.config.settings") && hash.includes("menu_id=") ? hash : "";
      })()`, "TS action route hash");
      return { title, hash, window_count: windowCount, control_panel_count: controlPanelCount };
    }
  },
  {
    name: "technical-list-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openServerActionsList(page, config, desktopViewport());
      const rowCount = await waitForCount(page, "#rows .o_list_table tbody tr", 1, "technical list rows");
      const title = await textContent(page, "#recordsView .o_breadcrumb.active");
      return { title, row_count: rowCount };
    }
  },
  {
    name: "hash-route-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openServerActionsList(page, config, desktopViewport());
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=ir.actions.server") && hash.includes("view_type=list") && hash.includes("menu_id=") ? hash : "";
      })()`, "action route hash");
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?legacy_webclient=1${hash}`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "document ready after hash reload");
      await waitFor(page, `Boolean(document.querySelector(".o_web_client .o_action_manager"))`, "web client shell after hash reload");
      await waitFor(page, `document.body.dataset.view === "records"`, "records view after hash reload");
      const rowCount = await waitForCount(page, "#rows .o_list_table tbody tr", 1, "restored technical list rows");
      const title = await textContent(page, "#recordsView .o_breadcrumb.active");
      return { hash, title, row_count: rowCount };
    }
  },
  {
    name: "technical-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openServerActionsList(page, config, desktopViewport());
      await clickFirst(page, "#rows .o_list_table tbody tr");
      await waitFor(page, `!document.querySelector("#recordPanel")?.hidden`, "technical form panel");
      const fieldCount = await waitForCount(page, "#recordForm input[data-field]", 1, "technical form fields");
      const title = await waitFor(page, `(() => {
        const title = document.querySelector("#recordTitle")?.textContent?.trim() || "";
        return title && title !== "Loading" ? title : "";
      })()`, "technical form title");
      const layout = await assertFormHeaderLayout(page, "desktop technical form");
      return { title, field_count: fieldCount, ...layout };
    }
  },
  {
    name: "search-menu-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openServerActionsList(page, config, desktopViewport());
      await clickSelector(page, "#recordSearchDropdown");
      await waitFor(page, `document.querySelector("#recordSearchMenu")?.hidden === false`, "search menu open");
      const filterItems = await waitForCount(page, "#recordFilterMenu .o_menu_item", 1, "filter items");
      const groupItems = await waitForCount(page, "#recordGroupByMenu .o_menu_item", 1, "group by items");
      const favoriteItems = await waitForCount(page, "#recordFavoriteMenu .o_menu_item", 2, "favorite items");
      return { filter_items: filterItems, group_by_items: groupItems, favorite_items: favoriteItems };
    }
  },
  {
    name: "launcher-mobile",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await openWeb(page, config, mobileViewport());
      const appCount = await waitForCount(page, "#appGrid .o_app", 2, "mobile launcher app tiles");
      const hasMenuToggle = await waitFor(page, `Boolean(document.querySelector(".o-mobile-menu-toggle"))`, "mobile menu toggle");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`mobile horizontal overflow: ${overflow}px`);
      return { app_count: appCount, menu_toggle: hasMenuToggle, horizontal_overflow_px: overflow };
    }
  },
  {
    name: "technical-list-mobile",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await openServerActionsList(page, config, mobileViewport());
      const cardCount = await waitForCount(page, ".o_mobile_list_cards .o_mobile_record_card", 1, "mobile technical cards");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`mobile technical horizontal overflow: ${overflow}px`);
      return { card_count: cardCount, horizontal_overflow_px: overflow };
    }
  },
  {
    name: "technical-form-mobile",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await openServerActionsList(page, config, mobileViewport());
      await clickFirst(page, ".o_mobile_list_cards .o_mobile_record_card button");
      await waitFor(page, `!document.querySelector("#recordPanel")?.hidden`, "mobile technical form panel");
      const fieldCount = await waitForCount(page, "#recordForm input[data-field]", 1, "mobile technical form fields");
      const title = await waitFor(page, `(() => {
        const title = document.querySelector("#recordTitle")?.textContent?.trim() || "";
        return title && title !== "Loading" ? title : "";
      })()`, "mobile technical form title");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`mobile form horizontal overflow: ${overflow}px`);
      const layout = await assertFormHeaderLayout(page, "mobile technical form");
      return { title, field_count: fieldCount, horizontal_overflow_px: overflow, ...layout };
    }
  }
];

export function parseArgs(argv) {
  const config = {
    baseURL: DEFAULT_BASE_URL,
    outDir: DEFAULT_OUT_DIR,
    baselineDir: "",
    updateBaseline: false,
    chromePath: process.env.CHROME_BIN || "",
    timeoutMs: DEFAULT_TIMEOUT_MS,
    headed: false,
    keepBrowser: false,
    list: false,
    help: false,
    scenarioFilter: []
  };

  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    const equalIndex = arg.indexOf("=");
    const flag = equalIndex >= 0 ? arg.slice(0, equalIndex) : arg;
    const inlineValue = equalIndex >= 0 ? arg.slice(equalIndex + 1) : undefined;
    const value = () => inlineValue ?? argv[++index];
    switch (flag) {
      case "--base-url":
        config.baseURL = value();
        break;
      case "--out":
        config.outDir = value();
        break;
      case "--baseline-dir":
        config.baselineDir = value();
        break;
      case "--chrome":
        config.chromePath = value();
        break;
      case "--timeout-ms":
        config.timeoutMs = Number(value());
        break;
      case "--scenario":
        config.scenarioFilter.push(value());
        break;
      case "--update-baseline":
        config.updateBaseline = true;
        break;
      case "--headed":
        config.headed = true;
        break;
      case "--keep-browser":
        config.keepBrowser = true;
        break;
      case "--list":
        config.list = true;
        break;
      case "--help":
      case "-h":
        config.help = true;
        break;
      default:
        throw new Error(`unknown argument: ${arg}`);
    }
  }

  if (!Number.isFinite(config.timeoutMs) || config.timeoutMs < 1000) {
    throw new Error("--timeout-ms must be a number >= 1000");
  }
  return config;
}

export function selectedScenarios(config, list = scenarios) {
  if (!config.scenarioFilter.length) return list;
  const wanted = new Set(config.scenarioFilter);
  const selected = list.filter((scenario) => wanted.has(scenario.name));
  const found = new Set(selected.map((scenario) => scenario.name));
  const missing = [...wanted].filter((name) => !found.has(name));
  if (missing.length) throw new Error(`unknown scenario(s): ${missing.join(", ")}`);
  return selected;
}

export function redactedURL(raw) {
  const url = new URL(raw);
  if (url.username) url.username = "redacted";
  if (url.password) url.password = "redacted";
  for (const key of [...url.searchParams.keys()]) {
    if (/token|password|secret|session|key/i.test(key)) url.searchParams.set(key, "redacted");
  }
  return url.toString();
}

export function appURL(baseURL, path) {
  const base = new URL(baseURL);
  return new URL(path, `${base.protocol}//${base.host}`).toString();
}

export function scenarioNames(list = scenarios) {
  return list.map((scenario) => scenario.name);
}

async function main() {
  const config = parseArgs(process.argv.slice(2));
  if (config.help) {
    process.stdout.write(helpText());
    return;
  }
  if (config.list) {
    process.stdout.write(`${scenarioNames().join("\n")}\n`);
    return;
  }

  const selected = selectedScenarios(config);
  const outDir = resolve(config.outDir);
  await mkdir(outDir, { recursive: true });

  const chrome = await launchChrome(config);
  const page = new CDPPage(chrome.wsURL, config.timeoutMs);
  const startedAt = Date.now();
  const results = [];
  let failures = [];

  try {
    await page.connect();
    await page.send("Page.enable");
    await page.send("Runtime.enable");

    for (const scenario of selected) {
      const scenarioStartedAt = Date.now();
      try {
        const assertions = await scenario.run(page, config);
        const screenshotName = `${scenario.name}.png`;
        const screenshotPath = join(outDir, screenshotName);
        const png = Buffer.from(await captureScreenshot(page), "base64");
        await writeFile(screenshotPath, png);
        const sha256 = hashBuffer(png);
        results.push({
          name: scenario.name,
          status: "passed",
          viewport: scenario.viewport,
          screenshot: screenshotName,
          sha256,
          assertions,
          duration_ms: Date.now() - scenarioStartedAt
        });
        process.stdout.write(`pass ${scenario.name} ${screenshotName}\n`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        results.push({
          name: scenario.name,
          status: "failed",
          viewport: scenario.viewport,
          error: message,
          duration_ms: Date.now() - scenarioStartedAt
        });
        failures.push(`${scenario.name}: ${message}`);
        process.stderr.write(`fail ${scenario.name}: ${message}\n`);
      }
    }

    const manifest = {
      generated_at: new Date().toISOString(),
      base_url: redactedURL(config.baseURL),
      chrome: basename(chrome.path),
      scenarios: results,
      duration_ms: Date.now() - startedAt
    };
    await writeFile(join(outDir, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);

    const baselineFailures = await handleBaseline(config, outDir, manifest);
    failures = failures.concat(baselineFailures);
  } finally {
    await page.close();
    await chrome.close();
  }

  if (failures.length) {
    throw new Error(`visual smoke failed\n${failures.map((failure) => `- ${failure}`).join("\n")}`);
  }
}

function helpText() {
  return `Usage: node tools/web_visual_smoke/run.mjs [options]

Options:
  --base-url URL          GoERP origin. Default: ${DEFAULT_BASE_URL}
  --out DIR              Screenshot/report output. Default: ${DEFAULT_OUT_DIR}
  --baseline-dir DIR     Compare screenshot hashes against DIR/manifest.json.
  --update-baseline      Replace baseline-dir contents with this run.
  --chrome PATH          Chrome/Chromium binary. Defaults to CHROME_BIN or auto-detect.
  --scenario NAME        Run one scenario. Repeatable.
  --timeout-ms N         Per-wait timeout. Default: ${DEFAULT_TIMEOUT_MS}
  --headed               Run Chrome with a visible window.
  --keep-browser         Do not delete the temporary Chrome profile.
  --list                 Print scenario names.
  --help                 Print this help.
`;
}

async function handleBaseline(config, outDir, manifest) {
  if (!config.baselineDir) return [];
  const baselineDir = resolve(config.baselineDir);
  if (config.updateBaseline) {
    await mkdir(baselineDir, { recursive: true });
    for (const scenario of manifest.scenarios) {
      if (scenario.status !== "passed" || !scenario.screenshot) continue;
      const screenshot = await readFile(join(outDir, scenario.screenshot));
      await writeFile(join(baselineDir, scenario.screenshot), screenshot);
    }
    await writeFile(join(baselineDir, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
    return [];
  }

  let baseline;
  try {
    baseline = JSON.parse(await readFile(join(baselineDir, "manifest.json"), "utf8"));
  } catch (error) {
    return [`baseline manifest missing or unreadable at ${join(baselineDir, "manifest.json")}`];
  }

  const expected = new Map((baseline.scenarios || []).map((scenario) => [scenario.name, scenario]));
  const failures = [];
  for (const actual of manifest.scenarios) {
    if (actual.status !== "passed") continue;
    const previous = expected.get(actual.name);
    if (!previous) {
      failures.push(`${actual.name}: missing baseline entry`);
      continue;
    }
    if (previous.sha256 !== actual.sha256) {
      failures.push(`${actual.name}: screenshot hash changed`);
    }
  }
  return failures;
}

async function openServerActionsList(page, config, viewport) {
  await openWeb(page, config, viewport);
  await setInput(page, "#appSearch", "Server Actions");
  await waitFor(page, `(() => {
    return [...document.querySelectorAll("#appGrid .o_app_name")].some((node) => node.textContent.trim() === "Server Actions");
  })()`, "Server Actions launcher result");
  await clickExactText(page, "#appGrid .o_app", "Server Actions", ".o_app_name");
  await waitFor(page, `document.body.dataset.view === "records"`, "records view");
  await waitForCount(page, "#rows .o_list_renderer", 1, "technical list renderer");
}

async function openWeb(page, config, viewport) {
  await setViewport(page, viewport);
  navigationCounter += 1;
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?legacy_webclient=1&smoke=${navigationCounter}`) });
  await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "document ready");
  await waitFor(page, `Boolean(document.querySelector(".o_web_client .o_action_manager"))`, "web client shell");
  await maybeLogin(page);
  await waitForCount(page, "#appGrid .o_app", 2, "app launcher tiles");
}

async function maybeLogin(page) {
  const loginVisible = await evaluate(page, `(() => {
    const panel = document.querySelector("#loginPanel");
    if (!panel) return false;
    const style = getComputedStyle(panel);
    return style.display !== "none" && style.visibility !== "hidden" && panel.getClientRects().length > 0;
  })()`);
  const hasApps = await evaluate(page, `document.querySelectorAll("#appGrid .o_app").length > 0`);
  if (!loginVisible || hasApps) return;
  await clickSelector(page, "#loginButton");
}

async function setViewport(page, viewport) {
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: viewport.width,
    height: viewport.height,
    deviceScaleFactor: 1,
    mobile: viewport.mobile
  });
}

function desktopViewport() {
  return { width: 1366, height: 900, mobile: false };
}

function mobileViewport() {
  return { width: 390, height: 844, mobile: true };
}

async function captureScreenshot(page) {
  const response = await page.send("Page.captureScreenshot", { format: "png", fromSurface: true });
  return response.data;
}

async function textContent(page, selector) {
  return evaluate(page, `document.querySelector(${JSON.stringify(selector)})?.textContent?.trim() || ""`);
}

async function clickSelector(page, selector) {
  return evaluate(page, `(() => {
    const node = document.querySelector(${JSON.stringify(selector)});
    if (!node) throw new Error("selector not found: ${escapeForJS(selector)}");
    node.click();
    return true;
  })()`);
}

async function clickFirst(page, selector) {
  return evaluate(page, `(() => {
    const node = document.querySelector(${JSON.stringify(selector)});
    if (!node) throw new Error("selector not found: ${escapeForJS(selector)}");
    node.click();
    return true;
  })()`);
}

async function clickText(page, selector, text) {
  return evaluate(page, `(() => {
    const text = ${JSON.stringify(text)};
    const node = [...document.querySelectorAll(${JSON.stringify(selector)})]
      .find((candidate) => (candidate.textContent || "").trim().includes(text));
    if (!node) throw new Error("text not found: " + text);
    node.click();
    return (node.textContent || "").trim();
  })()`);
}

async function clickExactText(page, selector, text, textSelector = "") {
  return evaluate(page, `(() => {
    const text = ${JSON.stringify(text)};
    const textSelector = ${JSON.stringify(textSelector)};
    const node = [...document.querySelectorAll(${JSON.stringify(selector)})].find((candidate) => {
      const target = textSelector ? candidate.querySelector(textSelector) : candidate;
      return ((target && target.textContent) || "").trim() === text;
    });
    if (!node) throw new Error("exact text not found: " + text);
    node.click();
    return true;
  })()`);
}

async function setInput(page, selector, value) {
  return evaluate(page, `(() => {
    const input = document.querySelector(${JSON.stringify(selector)});
    if (!input) throw new Error("input not found: ${escapeForJS(selector)}");
    input.focus();
    input.value = ${JSON.stringify(value)};
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keyup", { bubbles: true, key: "Enter" }));
    return input.value;
  })()`);
}

async function waitForCount(page, selector, minimum, label) {
  return waitFor(page, `(() => {
    const count = document.querySelectorAll(${JSON.stringify(selector)}).length;
    return count >= ${Number(minimum)} ? count : 0;
  })()`, label);
}

async function assertFormHeaderLayout(page, label) {
  return evaluate(page, `(() => {
    const selectors = {
      buttons: "#recordPanel .o_control_panel_main_buttons",
      breadcrumbs: "#recordPanel .o_control_panel_breadcrumbs",
      navigation: "#recordPanel .o_control_panel_navigation",
      controlPanel: "#recordPanel .o-control-panel",
      formSheet: "#recordPanel .o_form_sheet"
    };
    const rects = {};
    for (const [name, selector] of Object.entries(selectors)) {
      const node = document.querySelector(selector);
      if (!node) throw new Error("missing form layout selector " + selector);
      const rect = node.getBoundingClientRect();
      if (!rect.width || !rect.height) throw new Error("empty form layout selector " + selector);
      rects[name] = { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height };
    }
    const intersects = (a, b) => a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1;
    const failures = [];
    if (intersects(rects.buttons, rects.breadcrumbs)) failures.push("buttons overlap breadcrumbs");
    if (intersects(rects.breadcrumbs, rects.navigation)) failures.push("breadcrumbs overlap navigation");
    if (rects.controlPanel.bottom > rects.formSheet.top + 1) failures.push("control panel overlaps form sheet");
    if (failures.length) throw new Error(${JSON.stringify(label)} + ": " + failures.join("; ") + " " + JSON.stringify(rects));
    return {
      form_header_buttons_width: Math.round(rects.buttons.width),
      form_header_breadcrumbs_width: Math.round(rects.breadcrumbs.width),
      form_header_navigation_width: Math.round(rects.navigation.width),
      form_header_gap_px: Math.round(rects.formSheet.top - rects.controlPanel.bottom)
    };
  })()`);
}

async function waitFor(page, expression, label) {
  const startedAt = Date.now();
  let lastError = "";
  while (Date.now() - startedAt < page.timeoutMs) {
    try {
      const value = await evaluate(page, expression);
      if (value) return value;
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await delay(150);
  }
  throw new Error(`timed out waiting for ${label}${lastError ? `: ${lastError}` : ""}`);
}

async function evaluate(page, expression) {
  const response = await page.send("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
    userGesture: true
  });
  if (response.exceptionDetails) {
    const detail = response.exceptionDetails.exception?.description || response.exceptionDetails.text || "Runtime.evaluate failed";
    throw new Error(detail);
  }
  return response.result?.value;
}

async function launchChrome(config) {
  const chromePath = config.chromePath || findChrome();
  if (!chromePath) {
    throw new Error("Chrome/Chromium not found. Set CHROME_BIN or pass --chrome.");
  }
  const port = await freePort();
  const profileDir = await mkdtemp(join(tmpdir(), "gorp-web-visual-smoke-"));
  const args = [
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${profileDir}`,
    "--no-first-run",
    "--no-default-browser-check",
    "--disable-background-networking",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    "--window-size=1366,900"
  ];
  if (!config.headed) args.push("--headless=new");
  args.push("about:blank");

  const child = spawn(chromePath, args, { stdio: ["ignore", "ignore", "pipe"] });
  let stderr = "";
  let spawnError = "";
  child.on("error", (error) => {
    spawnError = error.message;
  });
  child.stderr.on("data", (chunk) => {
    stderr += chunk.toString();
  });

  const wsURL = await waitForChrome(port, config.timeoutMs, () => child.exitCode !== null || Boolean(spawnError), () => spawnError || stderr);
  return {
    path: chromePath,
    wsURL,
    async close() {
      if (child.exitCode === null) {
        child.kill("SIGTERM");
        await Promise.race([
          new Promise((resolveExit) => child.once("exit", resolveExit)),
          delay(1000)
        ]);
        if (child.exitCode === null) child.kill("SIGKILL");
      }
      if (!config.keepBrowser) await removeProfileDir(profileDir);
    }
  };
}

async function removeProfileDir(profileDir) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    try {
      await rm(profileDir, { recursive: true, force: true });
      return;
    } catch (error) {
      if (attempt === 4) throw error;
      await delay(100 * (attempt + 1));
    }
  }
}

function findChrome() {
  const candidates = [
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/usr/bin/google-chrome",
    "/usr/bin/google-chrome-stable",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser"
  ];
  return candidates.find((candidate) => existsSync(candidate));
}

async function waitForChrome(port, timeoutMs, exited, stderr) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    if (exited()) throw new Error(`Chrome exited before DevTools was ready: ${stderr()}`);
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`);
      if (response.ok) {
        const targets = await response.json();
        const page = targets.find((target) => target.type === "page" && target.webSocketDebuggerUrl);
        if (page) return page.webSocketDebuggerUrl;
      }
    } catch (_error) {
    }
    await delay(100);
  }
  throw new Error("timed out waiting for Chrome DevTools");
}

class CDPPage {
  constructor(wsURL, timeoutMs) {
    this.wsURL = wsURL;
    this.timeoutMs = timeoutMs;
    this.nextID = 1;
    this.pending = new Map();
    this.ws = null;
  }

  async connect() {
    this.ws = new WebSocket(this.wsURL);
    await new Promise((resolveOpen, rejectOpen) => {
      const timer = setTimeout(() => rejectOpen(new Error("timed out opening CDP websocket")), this.timeoutMs);
      this.ws.addEventListener("open", () => {
        clearTimeout(timer);
        resolveOpen();
      }, { once: true });
      this.ws.addEventListener("error", () => {
        clearTimeout(timer);
        rejectOpen(new Error("failed opening CDP websocket"));
      }, { once: true });
    });
    this.ws.addEventListener("message", (event) => this.handleMessage(event.data));
  }

  send(method, params = {}) {
    const id = this.nextID++;
    const payload = JSON.stringify({ id, method, params });
    return new Promise((resolveSend, rejectSend) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        rejectSend(new Error(`CDP timeout: ${method}`));
      }, this.timeoutMs);
      this.pending.set(id, {
        resolve: (value) => {
          clearTimeout(timer);
          resolveSend(value);
        },
        reject: (error) => {
          clearTimeout(timer);
          rejectSend(error);
        }
      });
      this.ws.send(payload);
    });
  }

  handleMessage(data) {
    const message = JSON.parse(data);
    if (!message.id) return;
    const pending = this.pending.get(message.id);
    if (!pending) return;
    this.pending.delete(message.id);
    if (message.error) {
      pending.reject(new Error(`${message.error.message || "CDP error"}${message.error.data ? `: ${message.error.data}` : ""}`));
    } else {
      pending.resolve(message.result || {});
    }
  }

  async close() {
    if (!this.ws) return;
    if (this.ws.readyState === WebSocket.OPEN) this.ws.close();
    this.ws = null;
  }
}

function hashBuffer(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function escapeForJS(value) {
  return String(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}

async function freePort() {
  const server = createServer();
  await new Promise((resolveListen, rejectListen) => {
    server.listen(0, "127.0.0.1", resolveListen);
    server.on("error", rejectListen);
  });
  const address = server.address();
  const port = typeof address === "object" && address ? address.port : 0;
  await new Promise((resolveClose) => server.close(resolveClose));
  return port;
}

function isMainModule() {
  return process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url);
}

if (isMainModule()) {
  main().catch((error) => {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exit(1);
  });
}
