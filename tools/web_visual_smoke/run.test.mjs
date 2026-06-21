import assert from "node:assert/strict";
import test from "node:test";
import {
  auditSettingsLabelSnapshot,
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
    "default-systray-dropdowns-desktop",
    "default-webclient-action-desktop",
    "default-technical-search-desktop",
    "default-technical-form-desktop",
    "default-search-menu-desktop",
    "default-search-filter-click-desktop",
    "default-view-switch-desktop",
    "default-hash-route-desktop",
    "default-webclient-mobile",
    "default-mobile-server-actions-flow",
    "technical-list-desktop",
    "hash-route-desktop",
    "technical-form-desktop",
    "search-menu-desktop",
    "launcher-mobile",
    "technical-list-mobile",
    "technical-form-mobile",
    "normal-user-launcher-desktop",
    "default-apps-install-desktop",
    "default-apps-lifecycle-cancel-desktop"
  ]);
});

test("settings label audit accepts human labels without treating field ids as visible text", () => {
  const audit = auditSettingsLabelSnapshot({
    text: "Workflow Expenses Time Off",
    appLabels: ["Workflow", "Activate Workflow on"],
    settings: [
      { id: "module_oi_workflow_expense", labels: ["Expenses"], text: "Expenses" },
      { id: "module_oi_workflow_hr_holidays", labels: ["Time Off"], text: "Time Off" }
    ]
  });

  assert.equal(audit.ok, true);
  assert.equal(audit.visible_setting_count, 2);
  assert.equal(audit.visible_label_count, 2);
  assert.equal(audit.raw_technical_label_count, 0);
  assert.equal(audit.empty_setting_label_count, 0);
});

test("settings label audit rejects raw technical module labels", () => {
  const audit = auditSettingsLabelSnapshot({
    text: "Workflow module_oi_workflow_expense",
    appLabels: ["Workflow"],
    settings: [
      { id: "workflow-expense", labels: ["module_oi_workflow_expense"], text: "module_oi_workflow_expense" }
    ]
  });

  assert.equal(audit.ok, false);
  assert.equal(audit.raw_technical_label_count, 1);
  assert.match(audit.issues.join("\n"), /raw technical module labels: module_oi_workflow_expense/);
});

test("settings label audit rejects empty visible setting labels", () => {
  const audit = auditSettingsLabelSnapshot({
    text: "Workflow",
    appLabels: ["Workflow"],
    settings: [
      { id: "workflow-empty", labels: [], text: "" }
    ]
  });

  assert.equal(audit.ok, false);
  assert.equal(audit.empty_setting_label_count, 1);
  assert.match(audit.issues.join("\n"), /empty visible settings labels: workflow-empty/);
});
