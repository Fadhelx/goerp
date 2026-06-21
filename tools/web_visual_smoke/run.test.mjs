import assert from "node:assert/strict";
import test from "node:test";
import {
  appURL,
  parseArgs,
  redactedURL,
  scenarioNames,
  selectedScenarios
} from "./run.mjs";

test("parseArgs supports inline and positional values", () => {
  const config = parseArgs([
    "--base-url=http://127.0.0.1:8073/web?debug=1",
    "--out",
    "reports/tmp-visual",
    "--scenario",
    "launcher-desktop",
    "--scenario=search-menu-desktop",
    "--timeout-ms=2000",
    "--headed"
  ]);

  assert.equal(config.baseURL, "http://127.0.0.1:8073/web?debug=1");
  assert.equal(config.outDir, "reports/tmp-visual");
  assert.deepEqual(config.scenarioFilter, ["launcher-desktop", "search-menu-desktop"]);
  assert.equal(config.timeoutMs, 2000);
  assert.equal(config.headed, true);
});

test("selectedScenarios rejects unknown names", () => {
  assert.throws(
    () => selectedScenarios({ scenarioFilter: ["missing"] }, [{ name: "launcher-desktop" }]),
    /unknown scenario/
  );
});

test("URL helpers normalize app path and redact secrets", () => {
  assert.equal(appURL("http://127.0.0.1:8073/web?debug=1", "/web"), "http://127.0.0.1:8073/web");
  assert.equal(
    redactedURL("https://user:pass@example.test/web?session_id=abc&debug=1"),
    "https://redacted:redacted@example.test/web?session_id=redacted&debug=1"
  );
});

test("scenario inventory covers requested web theme surfaces", () => {
  assert.deepEqual(scenarioNames(), [
    "launcher-desktop",
    "settings-desktop",
    "default-webclient-takeover",
    "default-webclient-action-desktop",
    "technical-list-desktop",
    "hash-route-desktop",
    "technical-form-desktop",
    "search-menu-desktop",
    "launcher-mobile",
    "technical-list-mobile",
    "technical-form-mobile"
  ]);
});
