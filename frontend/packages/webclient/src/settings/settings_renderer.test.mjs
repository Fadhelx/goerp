import assert from "node:assert/strict";
import {
  createSettingsRendererState,
  parseSettingsArch,
  renderSettingsView
} from "../../../../dist/packages/webclient/src/settings/settings_renderer.js";

class TestEvent {
  constructor(type, options = {}) {
    this.type = type;
    this.detail = options.detail;
    this.bubbles = options.bubbles === true;
    this.defaultPrevented = false;
    this.target = null;
    this.currentTarget = null;
  }

  preventDefault() {
    this.defaultPrevented = true;
  }
}

globalThis.CustomEvent = TestEvent;
globalThis.document = {
  createTextNode(text) {
    return { tag: "#text", textContent: text, children: [] };
  },
  createElement(tag) {
    return {
      tag,
      tagName: tag.toUpperCase(),
      className: "",
      dataset: {},
      attributes: {},
      textContent: "",
      value: "",
      type: "",
      checked: false,
      disabled: false,
      hidden: false,
      required: false,
      step: "",
      selected: false,
      children: [],
      listeners: {},
      append(...nodes) {
        this.children.push(...nodes);
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
      },
      addEventListener(type, listener) {
        this.listeners[type] = [...(this.listeners[type] ?? []), listener];
      },
      dispatchEvent(event) {
        event.target ??= this;
        event.currentTarget = this;
        for (const listener of this.listeners[event.type] ?? []) listener.call(this, event);
        return !event.defaultPrevented;
      }
    };
  }
};

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

function hasClass(node, name) {
  return String(node.className).split(/\s+/).includes(name);
}

function allText(node) {
  return [node.textContent, ...(node.children ?? []).map(allText)].filter(Boolean).join(" ");
}

const arch = `
  <form>
    <app name="general_settings" string="General Settings">
      <block title="Companies" name="companies">
        <setting id="company" string="Company" help="Main company">
          <field name="company_id"/>
        </setting>
        <setting string="Multi Currency">
          <field name="group_multi_currency"/>
        </setting>
        <setting string="Hidden" invisible="not group_multi_currency">
          <field name="hidden_option"/>
        </setting>
        <setting string="Provider">
          <field name="default_provider"/>
        </setting>
      </block>
    </app>
    <app name="workflow" data-string="Workflow">
      <field name="is_enterprise" invisible="1"/>
      <block title="Activate Workflow on" name="workflow">
        <setting>
          <field name="module_oi_workflow_expense"/>
        </setting>
        <setting invisible="not module_oi_workflow_hr_holidays">
          <field name="module_oi_workflow_hr_holidays_manager"/>
          <div class="text-muted">Add manager approval status</div>
        </setting>
      </block>
    </app>
  </form>
`;

const fields = {
  company_id: { type: "many2one", string: "Company" },
  group_multi_currency: { type: "boolean", string: "Multi Currency", help: "Use several currencies" },
  hidden_option: { type: "boolean", string: "Hidden Option" },
  default_provider: {
    type: "selection",
    string: "Default Provider",
    selection: [["openai", "OpenAI"], ["local", "Local"]]
  },
  is_enterprise: { type: "boolean", string: "Enterprise" },
  module_oi_workflow_expense: { type: "boolean", string: "Expenses" },
  module_oi_workflow_hr_holidays: { type: "boolean", string: "Time Off" },
  module_oi_workflow_hr_holidays_manager: { type: "boolean", string: "Time Off Manager" }
};

const values = {
  company_id: [3, "Main Company"],
  group_multi_currency: false,
  hidden_option: true,
  default_provider: "local",
  is_enterprise: true,
  module_oi_workflow_expense: true,
  module_oi_workflow_hr_holidays: false,
  module_oi_workflow_hr_holidays_manager: true
};

const parsed = parseSettingsArch(arch, fields, values);
assert.deepEqual(parsed.map((app) => [app.id, app.label]), [
  ["general_settings", "General Settings"],
  ["workflow", "Workflow"]
]);
assert.deepEqual(parsed[0].blocks.map((block) => [block.id, block.title]), [["companies", "Companies"]]);
assert.deepEqual(parsed[0].blocks[0].settings.map((setting) => setting.label), ["Company", "Multi Currency", "Provider"]);
assert.deepEqual(parsed[1].blocks[0].settings.map((setting) => setting.label), ["Expenses"]);
assert.equal(parsed[1].blocks[0].settings[0].fields[0].name, "module_oi_workflow_expense");

const state = createSettingsRendererState({ arch, fields, values, activeApp: "workflow" });
assert.equal(state.activeAppId, "workflow");
assert.equal(state.apps.length, 2);

const events = [];
const root = renderSettingsView({ arch, fields, values, activeApp: "workflow" }, {
  onAppSelect: (app) => events.push(["app", app.id]),
  onFieldChange: (name, value) => events.push(["field", name, value])
});

assert.ok(hasClass(root, "o_settings_container"));
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_search_panel")).length, 1);
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_search")).length, 1);
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_tab")).length, 2);
assert.equal(findAll(root, (node) => hasClass(node, "app_settings_block")).length, 2);
assert.equal(findAll(root, (node) => node.dataset?.appId === "general_settings" && hasClass(node, "app_settings_block"))[0].hidden, true);
assert.equal(findAll(root, (node) => node.dataset?.appId === "workflow" && hasClass(node, "app_settings_block"))[0].hidden, false);
const settingsSearch = findAll(root, (node) => hasClass(node, "o_settings_search"))[0];
settingsSearch.value = "expenses";
settingsSearch.dispatchEvent(new TestEvent("input"));
assert.equal(findAll(root, (node) => node.dataset?.settingId === "module_oi_workflow_expense")[0].hidden, false);
assert.equal(findAll(root, (node) => node.dataset?.settingId === "company")[0].hidden, true);
settingsSearch.value = "does not exist";
settingsSearch.dispatchEvent(new TestEvent("input"));
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_no_result"))[0].hidden, false);
settingsSearch.value = "";
settingsSearch.dispatchEvent(new TestEvent("input"));
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_no_result"))[0].hidden, true);
assert.equal(findAll(root, (node) => hasClass(node, "o_settings_block_title"))[1].textContent, "Activate Workflow on");
assert.equal(findAll(root, (node) => node.dataset?.settingId === "hidden_option").length, 0);
assert.equal(findAll(root, (node) => node.dataset?.field === "is_enterprise").length, 0);

const expenseInput = findAll(root, (node) => node.dataset?.field === "module_oi_workflow_expense" && node.type === "checkbox")[0];
assert.equal(expenseInput.checked, true);
expenseInput.checked = false;
expenseInput.dispatchEvent(new TestEvent("change"));

const providerSelect = findAll(root, (node) => node.dataset?.field === "default_provider" && node.tag === "select")[0];
assert.equal(providerSelect.value, "local");
assert.deepEqual(providerSelect.children.map((node) => node.textContent), ["OpenAI", "Local"]);
providerSelect.value = "openai";
providerSelect.dispatchEvent(new TestEvent("change"));

findAll(root, (node) => node.dataset?.appId === "general_settings" && hasClass(node, "o_settings_tab"))[0].dispatchEvent(new TestEvent("click"));

assert.deepEqual(events, [
  ["field", "module_oi_workflow_expense", false],
  ["field", "default_provider", "openai"],
  ["app", "general_settings"]
]);

const fallback = parseSettingsArch(
  `<form><sheet><group><field name="company_id"/><field name="group_multi_currency"/></group></sheet></form>`,
  fields,
  values
);
assert.equal(fallback.length, 1);
assert.equal(fallback[0].id, "general-settings");
assert.equal(fallback[0].blocks[0].settings.length, 2);

const generatedLabelsArch = `
  <form>
    <app name="workflow" string="Workflow">
      <block title="Activate Workflow on">
        <setting>
          <field name="module_oi_workflow_expense" string="module_oi_workflow_expense"/>
        </setting>
        <setting string="module_oi_workflow_hr_holidays_manager">
          <field name="module_oi_workflow_hr_holidays_manager"/>
        </setting>
        <setting>
          <field name="group_multi_currency"/>
        </setting>
      </block>
    </app>
  </form>
`;
const generatedLabels = parseSettingsArch(generatedLabelsArch, {}, {
  module_oi_workflow_expense: false,
  module_oi_workflow_hr_holidays_manager: false,
  group_multi_currency: false
});
assert.deepEqual(generatedLabels[0].blocks[0].settings.map((setting) => setting.label), [
  "Expenses",
  "Time Off Manager",
  "Multi Currency"
]);
const generatedLabelsRoot = renderSettingsView({
  arch: generatedLabelsArch,
  fields: {},
  values: {
    module_oi_workflow_expense: false,
    module_oi_workflow_hr_holidays_manager: false,
    group_multi_currency: false
  },
  activeApp: "workflow"
});
const generatedLabelsText = allText(generatedLabelsRoot);
assert.match(generatedLabelsText, /Expenses/);
assert.match(generatedLabelsText, /Time Off Manager/);
assert.doesNotMatch(generatedLabelsText, /module_oi_workflow|group_multi_currency/);

const metadataRawLabels = parseSettingsArch(generatedLabelsArch, {
  module_oi_workflow_expense: { type: "boolean", string: "module_oi_workflow_expense" },
  module_oi_workflow_hr_holidays_manager: { type: "boolean", string: "module_oi_workflow_hr_holidays_manager" },
  group_multi_currency: { type: "boolean", string: "group_multi_currency" }
}, {
  module_oi_workflow_expense: false,
  module_oi_workflow_hr_holidays_manager: false,
  group_multi_currency: false
});
assert.deepEqual(metadataRawLabels[0].blocks[0].settings.map((setting) => setting.label), [
  "Expenses",
  "Time Off Manager",
  "Multi Currency"
]);
