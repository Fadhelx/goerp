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
      const searchCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app_search_input", 1, "TS app search");
      const searchState = await evaluate(page, `(() => {
        const node = document.querySelector(".o_web_client .o_home_menu .o_home_menu_search");
        const input = document.querySelector(".o_web_client .o_home_menu .o_app_search_input");
        if (!node || !input) return { ok: false, reason: "missing launcher search" };
        const style = getComputedStyle(node);
        const hidden = node.dataset.searchActive === "false" && style.opacity === "0" && Number.parseFloat(style.maxHeight) === 0;
        const rect = input.getBoundingClientRect();
        const visible = rect.width >= 300 && rect.height >= 30 && style.display !== "none" && style.visibility !== "hidden";
        return { ok: hidden || visible, hidden, visible, width: rect.width, height: rect.height, search_active: node.dataset.searchActive || "" };
      })()`);
      if (!searchState.ok) throw new Error(`TS app search is not usable: ${JSON.stringify(searchState)}`);
      const launcherStyle = await assertEnterpriseLauncherSnapshot(page);
      const actionCount = await waitForCount(page, ".o_web_client .o_action_manager", 1, "TS action manager");
      const hasShellCue = await evaluate(page, `document.body.textContent.includes("Gorp") || document.body.textContent.includes("GoERP")`);
      if (hasShellCue) throw new Error("TS takeover exposes non-Odoo shell cue");
      return { nav_count: navCount, app_count: appCount, search_count: searchCount, search_state: searchState, ...launcherStyle, action_count: actionCount };
    }
  },
  {
    name: "default-systray-dropdowns-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?debug=1&smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "systray TS webclient ready");
      const systrayCount = await waitForCount(page, ".o_web_client .o_menu_systray", 1, "TS systray");
      const selectors = [
        ".o_web_client .o_mail_systray_item",
        ".o_web_client .o_activity_menu",
        ".o_web_client .o_switch_company_menu",
        ".o_web_client .o_debug_manager",
        ".o_web_client .o_user_menu"
      ];
      const actionStatusBefore = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus || ""`);
      for (const selector of selectors) {
        await waitForCount(page, selector, 1, `systray item ${selector}`);
        await clickSelector(page, selector);
        await waitFor(page, `(() => {
          const button = document.querySelector(${JSON.stringify(selector)});
          const menu = button?.nextElementSibling;
          return button?.getAttribute("aria-expanded") === "true" && menu?.classList.contains("show") && menu.querySelectorAll("[role='menuitem']").length >= 1;
        })()`, `systray dropdown opens ${selector}`);
        await evaluate(page, `document.dispatchEvent(new KeyboardEvent("keydown", {key: "Escape", bubbles: true}))`);
        await waitFor(page, `(() => !document.querySelector(".o_web_client .o_menu_systray .dropdown-menu.show"))()`, `systray dropdown escape closes ${selector}`);
      }
      await clickSelector(page, ".o_web_client .o_user_menu");
      await waitFor(page, `document.querySelector(".o_web_client .o_user_menu")?.getAttribute("aria-expanded") === "true"`, "user menu opened before outside click");
      await evaluate(page, `document.body.click()`);
      await waitFor(page, `(() => !document.querySelector(".o_web_client .o_menu_systray .dropdown-menu.show"))()`, "systray outside click closes");
      const actionStatusAfter = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus || ""`);
      if (actionStatusAfter !== actionStatusBefore) throw new Error(`systray changed action status: ${actionStatusBefore} -> ${actionStatusAfter}`);
      await clickSelector(page, ".o_web_client .o_user_menu");
      const openMenuItems = await waitForCount(page, ".o_web_client .o_user_menu + .dropdown-menu.show [role='menuitem']", 1, "user menu final open items");
      return { systray_count: systrayCount, item_count: selectors.length, open_menu_items: openMenuItems, action_status: actionStatusAfter };
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
      const settingsCount = await waitForCount(page, ".o_web_client .o_action_manager .o_settings_container", 1, "TS settings renderer");
      const settingsLabelAudit = await assertSettingsLabelSnapshot(page, ".o_web_client .o_action_manager .o_settings_container", "default TS Settings labels");
      const saveDisabled = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager [data-settings-action='save']")?.disabled === true`);
      const discardDisabled = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager [data-settings-action='discard']")?.disabled === true`);
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=res.config.settings") && hash.includes("menu_id=") ? hash : "";
      })()`, "TS action route hash");
      return { title, hash, window_count: windowCount, control_panel_count: controlPanelCount, settings_count: settingsCount, ...settingsLabelAudit, save_disabled: saveDisabled, discard_disabled: discardDisabled };
    }
  },
  {
    name: "default-technical-search-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      const opened = await openDefaultServerActionsList(page, config, desktopViewport());
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=ir.actions.server") && hash.includes("view_type=list") && hash.includes("menu_id=") ? hash : "";
      })()`, "TS technical action hash");
      const themeAudit = await assertEnterprisePolishSnapshot(page);
      const labelState = await evaluate(page, `(() => {
        const headers = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view th[data-name] .o_list_header_button")]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        const state = document.querySelector(".o_web_client .o_action_manager .gorp-list-view td[data-field='state']")?.textContent?.trim() || "";
        return { headers, state };
      })()`);
      for (const label of ["Name", "Type", "Model", "Active"]) {
        if (!labelState.headers.includes(label)) throw new Error(`TS technical list missing header ${label}: ${JSON.stringify(labelState)}`);
      }
      if (labelState.state === "code") throw new Error(`TS technical list shows raw state value: ${JSON.stringify(labelState)}`);
      return { title, hash, ...opened, ...themeAudit, label_state: labelState };
    }
  },
  {
    name: "default-technical-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=7&model=ir.actions.server&view_type=form&id=25&menu_id=8`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS technical form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "TS Server Actions form");
      const fieldCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-form-field", 1, "TS Server Actions form fields");
      const serverActionBandCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-band[data-state]", 1, "TS Server Actions header band");
      const serverActionNotebookCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-notebook .gorp-form-notebook-tab", 2, "TS Server Actions Code Help notebook");
      const codeViewerCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-notebook .gorp-code-viewer[data-field='code']", 1, "TS Server Actions code viewer");
      const selectionPillCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-selection-pills[data-field='state'] .gorp-selection-pill", 1, "TS Server Actions state selection pills");
      const relationLinkCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-many2one-link[data-field='model_id'][data-relation='ir.model']", 1, "TS Server Actions many2one relation link");
      const relationState = await evaluate(page, `(() => {
        const link = document.querySelector(".o_web_client .o_action_manager .gorp-form-view .gorp-many2one-link[data-field='model_id'][data-relation='ir.model']");
        return {
          text: link?.textContent?.trim() || "",
          res_id: link?.dataset?.resId || "",
          href: link?.getAttribute("href") || ""
        };
      })()`);
      if (!relationState.text || !relationState.res_id || !relationState.href.includes("model=ir.model")) {
        throw new Error(`TS Server Actions relation link invalid: ${JSON.stringify(relationState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const stateRadioCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-selection-radio-group[data-field='state'] input[type='radio']", 1, "TS Server Actions state radio editor");
      const codeEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-notebook .gorp-code-editor[data-field='code']", 1, "TS Server Actions code editor");
      const relationEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'][data-relation='ir.model']", 1, "TS Server Actions many2one editor");
      await setInput(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] input", "mail");
      const relationOptionCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option", 1, "TS Server Actions many2one options");
      const editorState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const input = editor?.querySelector("input");
        const options = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option")];
        return {
          relation: editor?.dataset?.relation || "",
          input_value: input?.value || "",
          option_labels: options.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (editorState.relation !== "ir.model" || !editorState.option_labels.length) throw new Error(`TS Server Actions relation editor invalid: ${JSON.stringify(editorState)}`);
      await clickFirst(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option");
      const selectedState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const save = document.querySelector(".o_web_client .o_action_manager [data-form-action='save']");
        return {
          res_id: editor?.dataset?.resId || "",
          save_hidden: save?.hidden === true,
          save_disabled: save?.disabled === true
        };
      })()`);
      if (!selectedState.res_id || selectedState.save_hidden) throw new Error(`TS Server Actions relation selection invalid: ${JSON.stringify(selectedState)}`);
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='discard']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-many2one-link[data-field='model_id'][data-relation='ir.model']", 1, "TS Server Actions readonly relation after discard");
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=form") && hash.includes("id=") ? hash : "";
      })()`, "TS technical form hash");
      return { title, hash, form_count: formCount, field_count: fieldCount, server_action_band_count: serverActionBandCount, server_action_notebook_count: serverActionNotebookCount, code_viewer_count: codeViewerCount, selection_pill_count: selectionPillCount, state_radio_count: stateRadioCount, code_editor_count: codeEditorCount, relation_link_count: relationLinkCount, relation_state: relationState, relation_editor_count: relationEditorCount, relation_option_count: relationOptionCount, relation_editor_state: editorState, relation_selected_state: selectedState };
    }
  },
  {
    name: "default-groups-form-notebook-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Groups notebook TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu .o_app_search_input", "Groups");
      const actionCardCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", 1, "TS Groups action card");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Groups", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS Groups list action ready");
      const listCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list']", 1, "TS Groups list");
      const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view tbody tr.o_data_row", 1, "TS Groups rows");
      await clickFirst(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view tbody tr.o_data_row");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS Groups form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-form-view", 1, "TS Groups form");
      const notebookCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-notebook.o_notebook", 1, "TS Groups form notebook");
      const tabCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-notebook-tab[role='tab']", 1, "TS Groups form notebook tabs");
      const x2ManyCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-tags[data-field='inherited_by_ids']", 1, "TS Groups x2many tag widget");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-form-notebook.o_notebook");
        const tabs = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-notebook-tab[role='tab']")];
        const pages = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-notebook-page[role='tabpanel']")];
        const inheritedFields = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-field[data-field='inherited_by_ids']")];
        const x2many = document.querySelector(".o_web_client .o_action_manager .gorp-x2many-tags[data-field='inherited_by_ids']");
        const tags = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-x2many-tags[data-field='inherited_by_ids'] .gorp-x2many-tag")];
        return {
          notebook_id: root?.dataset?.notebook || "",
          tab_labels: tabs.map((node) => node.textContent.trim()),
          selected_tabs: tabs.map((node) => node.getAttribute("aria-selected")),
          visible_pages: pages.filter((node) => !node.hidden).length,
          inherited_field_count: inheritedFields.length,
          x2many_count: x2many?.dataset?.count || "",
          x2many_relation: x2many?.dataset?.relation || "",
          x2many_tag_labels: tags.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (!state.tab_labels.includes("Inherited By")) throw new Error(`Groups notebook tab missing: ${JSON.stringify(state)}`);
      if (state.visible_pages !== 1) throw new Error(`Groups notebook visible page mismatch: ${JSON.stringify(state)}`);
      if (state.inherited_field_count !== 1) throw new Error(`Groups notebook field duplication: ${JSON.stringify(state)}`);
      if (state.x2many_relation !== "res.groups") throw new Error(`Groups x2many relation mismatch: ${JSON.stringify(state)}`);
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const x2ManyEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-editor[data-field='inherited_by_ids'][data-relation='res.groups']", 1, "TS Groups editable x2many tag widget");
      const editorState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-x2many-editor[data-field='inherited_by_ids']");
        const input = editor?.querySelector("input");
        const tags = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-x2many-editor[data-field='inherited_by_ids'] .gorp-x2many-editor-tag")];
        return {
          relation: editor?.dataset?.relation || "",
          count: editor?.dataset?.count || "",
          input_role: input?.getAttribute("role") || "",
          input_autocomplete: input?.getAttribute("aria-autocomplete") || "",
          tag_labels: tags.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (editorState.relation !== "res.groups" || editorState.input_role !== "combobox") throw new Error(`Groups editable x2many invalid: ${JSON.stringify(editorState)}`);
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='discard']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-tags[data-field='inherited_by_ids']", 1, "TS Groups readonly x2many after discard");
      return { action_card_count: actionCardCount, list_count: listCount, row_count: rowCount, form_count: formCount, notebook_count: notebookCount, tab_count: tabCount, x2many_widget_count: x2ManyCount, x2many_editor_count: x2ManyEditorCount, x2many_editor_state: editorState, ...state };
    }
  },
  {
    name: "default-delegation-one2many-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&delegation_one2many_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "delegation one2many setup document ready");
      await createDelegationAdminSession(page, config);
      const fixture = await createDelegationOne2ManySmokeRecord(page, config);
      const menuParam = fixture.menuID ? `&menu_id=${fixture.menuID}` : "";
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=delegation&view_type=form&id=${fixture.delegationID}${menuParam}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Delegation one2many TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Delegation one2many form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='delegation'][data-view='form'] .gorp-form-view", 1, "TS Delegation form");
      const readonlyCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-tags[data-field='lines']", 1, "TS Delegation readonly one2many tags");
      const readonlyState = await evaluate(page, `(() => {
        const tags = document.querySelector(".o_web_client .o_action_manager .gorp-x2many-tags[data-field='lines']");
        const labels = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-x2many-tags[data-field='lines'] .gorp-x2many-tag")]
          .map((node) => (node.textContent || "").trim())
          .filter(Boolean);
        return {
          relation: tags?.dataset?.relation || "",
          field_type: tags?.dataset?.fieldType || "",
          labels
        };
      })()`);
      if (readonlyState.relation !== "delegation.line" || readonlyState.field_type !== "one2many" || readonlyState.labels.length < 1) {
        throw new Error(`Delegation readonly one2many invalid: ${JSON.stringify(readonlyState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const editorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines'][data-relation='delegation.line']", 1, "TS Delegation editable one2many");
      const editorState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines']");
        const headers = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines'] th")]
          .map((node) => (node.textContent || "").trim())
          .filter(Boolean);
        const rows = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines'] tbody tr.o_data_row")];
        const relationInputs = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines'] [data-field='group_id']")];
        return {
          relation: editor?.dataset?.relation || "",
          field_type: editor?.dataset?.fieldType || "",
          headers,
          row_count: rows.length,
          relation_input_count: relationInputs.length
        };
      })()`);
      if (editorState.relation !== "delegation.line" || editorState.field_type !== "one2many" || editorState.row_count < 1 || editorState.relation_input_count < 1) {
        throw new Error(`Delegation one2many editor invalid: ${JSON.stringify(editorState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='discard']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-tags[data-field='lines']", 1, "TS Delegation readonly one2many after discard");
      return { form_count: formCount, readonly_count: readonlyCount, editor_count: editorCount, fixture, readonly_state: readonlyState, editor_state: editorState };
    }
  },
  {
    name: "default-search-menu-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, desktopViewport());
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const filterItems = await waitForCount(page, ".o_web_client .o_action_manager .o_filter_menu .o_menu_item", 1, "TS filter items");
      const groupItems = await waitForCount(page, ".o_web_client .o_action_manager .o_group_by_menu .o_menu_item", 1, "TS group by items");
      const favoriteItems = await waitForCount(page, ".o_web_client .o_action_manager .o_favorite_menu .o_menu_item", 1, "TS favorite items");
      return { filter_items: filterItems, group_by_items: groupItems, favorite_items: favoriteItems };
    }
  },
  {
    name: "default-search-filter-click-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, desktopViewport());
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      await clickSelector(page, ".o_web_client .o_action_manager .o_filter_menu .o_menu_item[data-menu-item-id='filter-code']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS filtered action ready");
      const facetCount = await waitForCount(page, ".o_web_client .o_action_manager .o_searchview_facet[data-facet-id='filter-code']", 1, "TS filter facet");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const selectedCount = await waitForCount(page, ".o_web_client .o_action_manager .o_filter_menu .o_menu_item.selected[data-menu-item-id='filter-code']", 1, "TS selected filter item");
      const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row", 1, "TS filtered rows");
      return { facet_count: facetCount, selected_filter_count: selectedCount, row_count: rowCount };
    }
  },
  {
    name: "default-view-switch-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, desktopViewport());
      await clickSelector(page, ".o_web_client .o_action_manager .o_switch_view.o_form");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS form switch ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "TS switched form");
      const formHash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=form") ? hash : "";
      })()`, "TS switched form hash");
      await clickSelector(page, ".o_web_client .o_action_manager .o_switch_view.o_list");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS list switch ready");
      const listCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='list'] .gorp-list-view tbody tr.o_data_row", 1, "TS switched list");
      const listHash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=list") ? hash : "";
      })()`, "TS switched list hash");
      return { form_count: formCount, list_count: listCount, form_hash: formHash, list_hash: listHash };
    }
  },
  {
    name: "default-hash-route-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, desktopViewport());
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=ir.actions.server") && hash.includes("view_type=list") && hash.includes("menu_id=") ? hash : "";
      })()`, "TS action route hash");
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}${hash}`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "default TS hash document ready");
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "default TS hash webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "default TS hash action ready");
      const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='list'] .gorp-list-view tbody tr.o_data_row", 1, "default TS restored rows");
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      return { hash, title, row_count: rowCount };
    }
  },
  {
    name: "default-webclient-mobile",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await setViewport(page, mobileViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "mobile TS webclient ready");
      const appCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "mobile TS app tiles");
      const mobileToggleCount = await waitForCount(page, ".o_web_client .o-mobile-menu-toggle", 1, "mobile TS menu toggle");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`mobile TS horizontal overflow: ${overflow}px`);
      await clickSelector(page, ".o_web_client .o-mobile-menu-toggle");
      await waitFor(page, `document.body.classList.contains("o-mobile-menu-open") && document.querySelector(".o_web_client .o-mobile-menu-toggle")?.getAttribute("aria-expanded") === "true"`, "mobile TS menu opened");
      await clickText(page, ".o_web_client .o_navbar_sections .o_nav_entry", "Settings");
      await waitFor(page, `!document.body.classList.contains("o-mobile-menu-open") && document.querySelector(".o_web_client .o-mobile-menu-toggle")?.getAttribute("aria-expanded") === "false"`, "mobile TS menu closed");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "mobile TS Settings action ready");
      return { app_count: appCount, mobile_toggle_count: mobileToggleCount, horizontal_overflow_px: overflow };
    }
  },
  {
    name: "default-mobile-server-actions-flow",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await openDefaultServerActionsList(page, config, mobileViewport());
      const cardCount = await waitForCount(page, ".o_web_client .o_action_manager .o_mobile_list_cards .o_mobile_record_card", 1, "default TS mobile Server Actions cards");
      await clickFirst(page, ".o_web_client .o_action_manager .o_mobile_list_cards .o_mobile_record_open");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "default TS mobile form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "default TS mobile Server Actions form");
      const breadcrumbCount = await waitForCount(page, ".o_web_client .o_action_manager .o_control_panel_breadcrumbs", 1, "default TS mobile breadcrumbs");
      const sheetCount = await waitForCount(page, ".o_web_client .o_action_manager .o_form_sheet", 1, "default TS mobile form sheet");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=form") && hash.includes("id=") ? hash : "";
      })()`, "default TS mobile form hash");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`default TS mobile action horizontal overflow: ${overflow}px`);
      return { card_count: cardCount, form_count: formCount, breadcrumb_count: breadcrumbCount, sheet_count: sheetCount, hash, horizontal_overflow_px: overflow };
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
  },
  {
    name: "normal-user-launcher-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&normal_user_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "normal-user setup document ready");
      const normalUser = await createNormalUserSession(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&normal_user=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "normal-user TS webclient ready");
      const appLabels = await waitFor(page, `(() => {
        const labels = [...document.querySelectorAll(".o_web_client .o_home_menu .o_app_name")]
          .map((node) => (node.textContent || "").trim())
          .filter(Boolean);
        return labels.length ? labels : null;
      })()`, "normal-user app labels");
      assertIncludes(appLabels, "Approvals", "normal-user launcher");
      for (const hidden of ["Apps", "Delegation", "Settings", "Technical"]) {
        assertExcludes(appLabels, hidden, "normal-user launcher");
      }
      const menus = await webRequestJSON(page, config, "/web/webclient/load_menus", null, "GET");
      const menuLabels = flattenMenuNames(menus);
      for (const hidden of ["Apps", "Delegation", "Settings", "Technical"]) {
        assertExcludes(menuLabels, hidden, "normal-user menus");
      }
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Approvals", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "normal-user Approvals action ready");
      const windowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action", 1, "normal-user Approvals action");
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const model = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager .gorp-window-action")?.dataset.model || ""`);
      return { uid: normalUser.uid, app_count: appLabels.length, menu_count: menuLabels.length, title, model, window_count: windowCount };
    }
  },
  {
    name: "default-apps-install-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&apps_install_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "apps install setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const targetRows = await webCallKW(page, config, "ir.module.module", "search_read", {
        args: [[["name", "=", "ai"]]],
        kwargs: { fields: ["id", "name", "state"], limit: 1 }
      });
      const targetID = Number(targetRows?.[0]?.id || 0);
      if (!targetID) throw new Error("ai module row not found for Apps install smoke");
      await webCallKW(page, config, "ir.module.module", "write", { args: [[targetID], { state: "uninstalled" }] });
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&apps_install=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Apps install TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu .o_app_search_input", "install");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Apps", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Apps catalog action ready");
      const catalogCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog", 1, "TS Apps catalog");
      await setInput(page, ".o_web_client .gorp-apps-catalog .o_searchview_input", "ai");
      const beforeState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "uninstalled" ? "uninstalled" : ""`, "AI module uninstalled state");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_install_button");
      const afterInstallState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "installed" ? "installed" : ""`, "AI module installed state");
      await waitForCount(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_upgrade_button", 1, "AI module upgrade button");
      await waitForCount(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_uninstall_button", 1, "AI module uninstall button");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_upgrade_button");
      const afterUpgradeState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "installed" ? "installed" : ""`, "AI module upgraded state");
      await waitFor(page, `(() => {
        const button = document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_uninstall_button");
        return button && !button.disabled && button.textContent.trim() === "Uninstall" ? "ready" : "";
      })()`, "AI module uninstall button ready after upgrade");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_uninstall_button");
      const afterUninstallState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "uninstalled" ? "uninstalled" : ""`, "AI module uninstalled after uninstall action");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_install_button");
      const restoredState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "installed" ? "installed" : ""`, "AI module restored installed state");
      return { module: "ai", catalog_count: catalogCount, before_state: beforeState, after_install_state: afterInstallState, after_upgrade_state: afterUpgradeState, after_uninstall_state: afterUninstallState, restored_state: restoredState };
    }
  },
  {
    name: "default-apps-catalog-detail-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultAppsCatalogForModule(page, config, "ai", "apps_catalog_detail");
      const sidebarCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog-sidebar", 1, "Apps catalog category sidebar");
      const filterCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog [data-catalog-filter]", 4, "Apps catalog filters");
      const categoryCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog-sidebar [data-category]", 1, "Apps catalog categories");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog [data-catalog-filter='installed']");
      const activeFilter = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog")?.dataset.activeFilter === "installed" ? "installed" : ""`, "Apps catalog installed filter active");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog [data-catalog-filter='all']");
      const cardCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai']", 1, "AI module card for detail");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_info_button");
      const detailModule = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-detail")?.dataset.moduleName === "ai" ? "ai" : ""`, "AI module detail panel");
      const detailText = await textContent(page, ".o_web_client .gorp-apps-catalog-detail");
      if (!detailText.includes("Technical Name") || !detailText.includes("Dependencies")) {
        throw new Error(`Apps detail panel missing metadata: ${detailText}`);
      }
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-detail .o_module_info_close");
      const detailClosed = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-detail")?.hidden === true ? "closed" : ""`, "Apps detail panel closes");
      return { module: "ai", sidebar_count: sidebarCount, filter_count: filterCount, category_count: categoryCount, active_filter: activeFilter, card_count: cardCount, detail_module: detailModule, detail_closed: detailClosed };
    }
  },
  {
    name: "default-apps-lifecycle-cancel-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&apps_cancel_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "apps cancel setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const targetRows = await webCallKW(page, config, "ir.module.module", "search_read", {
        args: [[["name", "=", "ai"]]],
        kwargs: { fields: ["id", "name", "state"], limit: 1 }
      });
      const targetID = Number(targetRows?.[0]?.id || 0);
      if (!targetID) throw new Error("ai module row not found for Apps cancel smoke");

      await webCallKW(page, config, "ir.module.module", "write", { args: [[targetID], { state: "uninstalled" }] });
      await webCallKW(page, config, "ir.module.module", "button_install", { args: [[targetID]] });
      await openDefaultAppsCatalogForModule(page, config, "ai", "apps_cancel_install");
      const toInstallState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "to install" ? "to install" : ""`, "AI module to install state");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_cancel_button[data-module-action='button_cancel_install']");
      const afterCancelInstallState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "uninstalled" ? "uninstalled" : ""`, "AI module canceled install state");

      await webCallKW(page, config, "ir.module.module", "write", { args: [[targetID], { state: "installed" }] });
      await webCallKW(page, config, "ir.module.module", "button_upgrade", { args: [[targetID]] });
      await openDefaultAppsCatalogForModule(page, config, "ai", "apps_cancel_upgrade");
      const toUpgradeState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "to upgrade" ? "to upgrade" : ""`, "AI module to upgrade state");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_cancel_button[data-module-action='button_cancel_upgrade']");
      const afterCancelUpgradeState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "installed" ? "installed" : ""`, "AI module canceled upgrade state");

      await webCallKW(page, config, "ir.module.module", "write", { args: [[targetID], { state: "installed" }] });
      await webCallKW(page, config, "ir.module.module", "button_uninstall", { args: [[targetID]] });
      await openDefaultAppsCatalogForModule(page, config, "ai", "apps_cancel_uninstall");
      const toRemoveState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "to remove" ? "to remove" : ""`, "AI module to remove state");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_cancel_button[data-module-action='button_cancel_uninstall']");
      const afterCancelUninstallState = await waitFor(page, `document.querySelector(".o_web_client .gorp-apps-catalog-card[data-module-name='ai'] .o_module_state")?.textContent?.trim() === "installed" ? "installed" : ""`, "AI module canceled uninstall state");
      return { module: "ai", to_install_state: toInstallState, after_cancel_install_state: afterCancelInstallState, to_upgrade_state: toUpgradeState, after_cancel_upgrade_state: afterCancelUpgradeState, to_remove_state: toRemoveState, after_cancel_uninstall_state: afterCancelUninstallState };
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

export function auditSettingsLabelSnapshot(snapshot) {
  const settings = Array.isArray(snapshot?.settings) ? snapshot.settings : [];
  const labelTexts = settings.flatMap((setting) => Array.isArray(setting.labels) ? setting.labels : [])
    .map(normalizeText)
    .filter(Boolean);
  const allText = normalizeText([
    ...(Array.isArray(snapshot?.appLabels) ? snapshot.appLabels : []),
    ...settings.flatMap((setting) => Array.isArray(setting.labels) ? setting.labels : [])
  ].join(" "));
  const rawTechnicalLabels = [...new Set(allText.match(/\bmodule_[a-z0-9_]*\b/gi) || [])].sort();
  const emptySettings = settings
    .filter((setting) => !(Array.isArray(setting.labels) && setting.labels.some((label) => normalizeText(label))))
    .map((setting, index) => normalizeText(setting?.id) || `setting-${index + 1}`);
  const issues = [];
  if (!settings.length) issues.push("no visible settings boxes");
  if (rawTechnicalLabels.length) issues.push(`raw technical module labels: ${rawTechnicalLabels.join(", ")}`);
  if (emptySettings.length) issues.push(`empty visible settings labels: ${emptySettings.join(", ")}`);
  return {
    ok: issues.length === 0,
    issues,
    visible_setting_count: settings.length,
    visible_label_count: labelTexts.length,
    raw_technical_label_count: rawTechnicalLabels.length,
    empty_setting_label_count: emptySettings.length
  };
}

function normalizeText(value) {
  return String(value ?? "").replace(/\s+/g, " ").trim();
}

async function assertEnterprisePolishSnapshot(page) {
  const snapshot = await evaluate(page, `(() => {
    const styleValue = (selector, property) => {
      const node = document.querySelector(selector);
      return node ? getComputedStyle(node).getPropertyValue(property) : "";
    };
    const pixelValue = (selector, property) => {
      const value = styleValue(selector, property);
      const parsed = Number.parseFloat(value);
      return Number.isFinite(parsed) ? parsed : 0;
    };
    return {
      control_panel_bg: styleValue(".o_web_client .o_action_manager .o_control_panel", "background-color"),
      control_panel_shadow: styleValue(".o_web_client .o_action_manager .o_control_panel", "box-shadow"),
      control_panel_min_height_px: pixelValue(".o_web_client .o_action_manager .o_control_panel", "min-height"),
      search_width_px: pixelValue(".o_web_client .o_action_manager .o_cp_searchview", "width"),
      search_radius_px: pixelValue(".o_web_client .o_action_manager .o_searchview", "border-top-left-radius"),
      list_header_bg: styleValue(".o_web_client .o_action_manager .gorp-list-view th", "background-color")
    };
  })()`);
  const issues = [];
  const acceptedControlPanelBG = new Set(["rgb(255, 255, 255)", "rgb(41, 44, 58)"]);
  const acceptedListHeaderBG = new Set(["rgb(246, 247, 248)", "rgb(28, 31, 42)"]);
  if (!acceptedControlPanelBG.has(snapshot.control_panel_bg)) issues.push(`control panel bg ${snapshot.control_panel_bg}`);
  if (snapshot.control_panel_bg === "rgb(255, 255, 255)" && (!snapshot.control_panel_shadow || snapshot.control_panel_shadow === "none")) issues.push("control panel shadow missing");
  if (snapshot.control_panel_min_height_px < 60 || snapshot.control_panel_min_height_px > 66) issues.push(`control panel min-height ${snapshot.control_panel_min_height_px}`);
  if (snapshot.search_width_px < 400 || snapshot.search_width_px > 450) issues.push(`search width ${snapshot.search_width_px}`);
  if (snapshot.search_radius_px !== 4) issues.push(`search radius ${snapshot.search_radius_px}`);
  if (!acceptedListHeaderBG.has(snapshot.list_header_bg)) issues.push(`list header bg ${snapshot.list_header_bg}`);
  if (issues.length) throw new Error(`enterprise polish style audit failed: ${issues.join("; ")}`);
  return snapshot;
}

async function assertEnterpriseLauncherSnapshot(page) {
  const snapshot = await evaluate(page, `(() => {
    const card = document.querySelector(".o_web_client .o_home_menu .o_app");
    const icon = card?.querySelector(".o_app_icon");
    const cardStyle = card ? getComputedStyle(card) : null;
    const iconStyle = icon ? getComputedStyle(icon) : null;
    const rect = card?.getBoundingClientRect();
    const iconRect = icon?.getBoundingClientRect();
    return {
      app_card_width_px: rect ? Math.round(rect.width) : 0,
      app_card_height_px: rect ? Math.round(rect.height) : 0,
      app_card_bg: cardStyle?.backgroundColor || "",
      app_card_border_color: cardStyle?.borderTopColor || "",
      app_card_border_radius_px: cardStyle ? Number.parseFloat(cardStyle.borderTopLeftRadius) || 0 : 0,
      app_icon_width_px: iconRect ? Math.round(iconRect.width) : 0,
      app_icon_height_px: iconRect ? Math.round(iconRect.height) : 0,
      app_icon_radius_px: iconStyle ? Number.parseFloat(iconStyle.borderTopLeftRadius) || 0 : 0
    };
  })()`);
  const issues = [];
  const transparent = new Set(["rgba(0, 0, 0, 0)", "transparent"]);
  if (snapshot.app_card_width_px < 80 || snapshot.app_card_width_px > 100) issues.push(`app card width ${snapshot.app_card_width_px}`);
  if (snapshot.app_card_height_px < 86 || snapshot.app_card_height_px > 108) issues.push(`app card height ${snapshot.app_card_height_px}`);
  if (!transparent.has(snapshot.app_card_bg)) issues.push(`app card background ${snapshot.app_card_bg}`);
  if (!transparent.has(snapshot.app_card_border_color)) issues.push(`app card border ${snapshot.app_card_border_color}`);
  if (snapshot.app_icon_width_px < 50 || snapshot.app_icon_width_px > 70) issues.push(`app icon width ${snapshot.app_icon_width_px}`);
  if (snapshot.app_icon_height_px < 50 || snapshot.app_icon_height_px > 70) issues.push(`app icon height ${snapshot.app_icon_height_px}`);
  if (snapshot.app_icon_radius_px < 10 || snapshot.app_icon_radius_px > 16) issues.push(`app icon radius ${snapshot.app_icon_radius_px}`);
  if (issues.length) throw new Error(`enterprise launcher style audit failed: ${issues.join("; ")}`);
  return snapshot;
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

async function openDefaultServerActionsList(page, config, viewport) {
  await setViewport(page, viewport);
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
  await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "TS webclient ready");
  await setInput(page, ".o_web_client .o_app_search_input", "Server Actions");
  const actionCardCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", 1, "TS technical search actions");
  await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Server Actions", ".o_app_name");
  await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS technical action ready");
  const windowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='list']", 1, "TS Server Actions list");
  const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row", 1, "TS Server Actions rows");
  return { action_card_count: actionCardCount, window_count: windowCount, row_count: rowCount };
}

async function openDefaultAppsCatalogForModule(page, config, moduleName, marker) {
  await setViewport(page, desktopViewport());
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&${marker}=1`) });
  await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Apps catalog TS webclient ready");
  await setInput(page, ".o_web_client .o_home_menu .o_app_search_input", "install");
  await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Apps", ".o_app_name");
  await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Apps catalog action ready");
  await waitForCount(page, ".o_web_client .gorp-apps-catalog", 1, "TS Apps catalog");
  await setInput(page, ".o_web_client .gorp-apps-catalog .o_searchview_input", moduleName);
  await waitForCount(page, `.o_web_client .gorp-apps-catalog-card[data-module-name='${moduleName}']`, 1, `${moduleName} module card`);
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

async function createNormalUserSession(page, config) {
  await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
  const groupRows = await webCallKW(page, config, "ir.model.data", "search_read", {
    args: [[["module", "=", "base"], ["name", "=", "group_user"]]],
    kwargs: { fields: ["res_id"], limit: 1 }
  });
  const groupID = Number(groupRows?.[0]?.res_id || 0);
  if (!groupID) throw new Error("base.group_user not found for normal-user smoke");
  const session = await webRequestJSON(page, config, "/web/session/info", null, "GET");
  const companyID = Number(session?.company_id || 0);
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const login = `visual-normal-${suffix}@example.test`;
  const password = `visual-normal-${suffix}`;
  const values = {
    login,
    password,
    email: login,
    name: "Visual Normal User",
    active: true,
    groups_id: [groupID],
    group_ids: [groupID],
    all_group_ids: [groupID]
  };
  if (companyID > 0) {
    values.company_id = companyID;
    values.company_ids = [companyID];
  }
  const created = await webCallKW(page, config, "res.users", "create", { values });
  const uid = Number(created?.id || created || 0);
  if (!uid) throw new Error("normal-user smoke user was not created");
  const authenticated = await webRequestJSON(page, config, "/web/session/authenticate", { login, password });
  const authenticatedUID = Number(authenticated?.uid || 0);
  if (authenticatedUID !== uid) throw new Error(`normal-user authenticate uid mismatch: ${authenticatedUID} !== ${uid}`);
  return { uid };
}

async function createDelegationAdminSession(page, config) {
  await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
  const xmlIDs = [
    "base.group_user",
    "base.group_erp_manager",
    "base.group_system",
    "oi_delegation.group_delegation_user",
    "oi_delegation.group_delegation_manager",
    "oi_delegation.group_delegation_admin",
    "oi_delegation.delegation_employee",
    "oi_delegation.delegation_manager",
    "oi_delegation.delegation_admin"
  ];
  const ids = await externalResIDs(page, config, xmlIDs);
  const groupIDs = xmlIDs.map((xmlID) => ids[xmlID]).filter((id) => Number(id) > 0);
  if (groupIDs.length !== xmlIDs.length) {
    const missing = xmlIDs.filter((xmlID) => !ids[xmlID]);
    throw new Error(`delegation smoke admin groups not found: ${missing.join(", ")}`);
  }
  const session = await webRequestJSON(page, config, "/web/session/info", null, "GET");
  const companyID = Number(session?.company_id || 0);
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const login = `visual-delegation-admin-${suffix}@example.test`;
  const password = `visual-delegation-admin-${suffix}`;
  const values = {
    login,
    password,
    email: login,
    name: "Visual Delegation Admin",
    active: true,
    share: false,
    groups_id: groupIDs,
    group_ids: groupIDs,
    all_group_ids: groupIDs
  };
  if (companyID > 0) {
    values.company_id = companyID;
    values.company_ids = [companyID];
  }
  const created = await webCallKW(page, config, "res.users", "create", { values });
  const uid = Number(created?.id || created || 0);
  if (!uid) throw new Error("delegation smoke admin user was not created");
  const authenticated = await webRequestJSON(page, config, "/web/session/authenticate", { login, password });
  const authenticatedUID = Number(authenticated?.uid || 0);
  if (authenticatedUID !== uid) throw new Error(`delegation smoke admin authenticate uid mismatch: ${authenticatedUID} !== ${uid}`);
  return { uid, groupIDs };
}

async function createDelegationOne2ManySmokeRecord(page, config) {
  const ids = await externalResIDs(page, config, [
    "oi_delegation.act_delegation",
    "oi_delegation.menu_delegation"
  ]);
  const actionID = Number(ids["oi_delegation.act_delegation"] || 0);
  if (!actionID) throw new Error("oi_delegation.act_delegation not found for one2many smoke");
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const groupName = `Visual One2many ${suffix}`;
  const groupCreated = await webCallKW(page, config, "res.groups", "create", {
    values: {
      name: groupName,
      full_name: `Role / ${groupName}`,
      name_delegation: groupName,
      allow_delegation: true
    }
  });
  const groupID = Number(groupCreated?.id || groupCreated || 0);
  if (!groupID) throw new Error("delegation one2many smoke group was not created");
  const rows = await webCallKW(page, config, "delegation", "web_save", {
    args: [[], {
      name: `Visual Delegation ${suffix}`,
      date_from: "2099-01-01",
      date_to: "2099-12-31",
      state: "draft",
      lines: [[0, false, { group_id: groupID }]]
    }],
    kwargs: {
      specification: {
        name: {},
        employee_id: {},
        lines: { fields: { group_id: {}, employee_id: {}, state: {} } }
      }
    }
  });
  const row = Array.isArray(rows) ? rows[0] : null;
  const delegationID = Number(row?.id || 0);
  if (!delegationID) throw new Error(`delegation one2many smoke record was not created: ${JSON.stringify(rows)}`);
  return {
    actionID,
    menuID: Number(ids["oi_delegation.menu_delegation"] || 0),
    delegationID,
    groupID,
    groupName
  };
}

async function externalResIDs(page, config, xmlIDs) {
  const grouped = new Map();
  for (const xmlID of xmlIDs) {
    const [module, name] = String(xmlID).split(".");
    if (!module || !name) throw new Error(`invalid external id: ${xmlID}`);
    const names = grouped.get(module) || [];
    names.push(name);
    grouped.set(module, names);
  }
  const ids = {};
  for (const [module, names] of grouped.entries()) {
    const rows = await webCallKW(page, config, "ir.model.data", "search_read", {
      args: [[["module", "=", module], ["name", "in", names]]],
      kwargs: { fields: ["module", "name", "res_id"], limit: names.length }
    });
    for (const row of rows || []) {
      ids[`${row.module}.${row.name}`] = Number(row.res_id || 0);
    }
  }
  return ids;
}

async function webCallKW(page, config, model, method, payload = {}) {
  try {
    return await webRequestJSON(page, config, "/web/dataset/call_kw", Object.assign({ model, method }, payload));
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    throw new Error(`${model}.${method}: ${message}`);
  }
}

async function webRequestJSON(page, config, path, payload = null, method = "POST") {
  const url = appURL(config.baseURL, path);
  const body = payload == null ? "" : JSON.stringify(payload);
  return evaluate(page, `(async () => {
    const options = {
      method: ${JSON.stringify(method)},
      credentials: "include",
      headers: {"Content-Type": "application/json"}
    };
    if (${JSON.stringify(body)} !== "") options.body = ${JSON.stringify(body)};
    const response = await fetch(${JSON.stringify(url)}, options);
    const text = await response.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch (_error) { data = text; }
    const rpcError = data && typeof data === "object" && data.error;
    if (!response.ok || rpcError) {
      const message = rpcError?.message || rpcError?.data?.message || (typeof data === "string" ? data : JSON.stringify(data));
      throw new Error("request failed " + response.status + " " + ${JSON.stringify(path)} + ": " + message);
    }
    return data && typeof data === "object" && Object.prototype.hasOwnProperty.call(data, "result") ? data.result : data;
  })()`);
}

function flattenMenuNames(menus) {
  const labels = [];
  const children = menus?.children && typeof menus.children === "object" ? menus.children : {};
  const collect = (node) => {
    if (!node || typeof node !== "object") return;
    const name = normalizeText(node.name);
    if (name) labels.push(name);
    const ids = Array.isArray(node.children) ? node.children : [];
    for (const id of ids) collect(children[String(id)] || children[id]);
  };
  for (const id of Object.keys(children)) collect(children[id]);
  return [...new Set(labels)];
}

function assertIncludes(values, wanted, label) {
  if (!values.includes(wanted)) throw new Error(`${label}: expected ${wanted} in ${JSON.stringify(values)}`);
}

function assertExcludes(values, unwanted, label) {
  if (values.includes(unwanted)) throw new Error(`${label}: expected ${unwanted} to be hidden in ${JSON.stringify(values)}`);
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

async function assertSettingsLabelSnapshot(page, selector, label) {
  const snapshot = await evaluate(page, `(() => {
    const root = document.querySelector(${JSON.stringify(selector)});
    if (!root) throw new Error("settings container not found: ${escapeForJS(selector)}");
    const clean = (value) => String(value || "").replace(/\\s+/g, " ").trim();
    const isVisible = (node) => !node.closest("[hidden], [aria-hidden='true']");
    const labelSelectors = ".o_form_label, .o_setting_field_label, .form-check-label";
    const settings = [...root.querySelectorAll(".o_setting_box")]
      .filter(isVisible)
      .map((box, index) => ({
        id: box.dataset.settingId || "setting-" + (index + 1),
        labels: [...new Set([...box.querySelectorAll(labelSelectors)].filter(isVisible).map((node) => clean(node.textContent)).filter(Boolean))],
        text: clean(box.textContent)
      }));
    const appLabels = [...root.querySelectorAll(".o_settings_tab, .o_settings_app_title, .o_settings_block_title")]
      .filter(isVisible)
      .map((node) => clean(node.textContent))
      .filter(Boolean);
    return { settings, appLabels, text: [...appLabels, ...settings.map((setting) => setting.text)].join(" ") };
  })()`);
  const audit = auditSettingsLabelSnapshot(snapshot);
  if (!audit.ok) throw new Error(`${label}: ${audit.issues.join("; ")}`);
  return {
    visible_setting_count: audit.visible_setting_count,
    visible_label_count: audit.visible_label_count,
    raw_technical_label_count: audit.raw_technical_label_count,
    empty_setting_label_count: audit.empty_setting_label_count
  };
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
