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
const ODOO_TECHNICAL_DROPDOWN_LABELS = [
  "Email",
  "Outgoing Mail Servers",
  "Actions",
  "Actions",
  "Reports",
  "Window Actions",
  "Client Actions",
  "Server Actions",
  "Embedded Actions",
  "Configuration Wizards",
  "User-defined Defaults",
  "IAP",
  "IAP Accounts",
  "User Interface",
  "Menu Items",
  "Views",
  "Customized Views",
  "User-defined Filters",
  "Tours",
  "Database Structure",
  "Decimal Accuracy",
  "Assets",
  "Models",
  "Fields",
  "Fields Selection",
  "Model Constraints",
  "ManyToMany Relations",
  "Attachments",
  "Logging",
  "Profiling",
  "Automation",
  "Scheduled Actions",
  "Scheduled Actions Triggers",
  "Reporting",
  "Paper Format",
  "Reports",
  "Sequences & Identifiers",
  "External Identifiers",
  "Sequences",
  "Parameters",
  "System Parameters",
  "Security",
  "Record Rules",
  "Access Rights",
  "User Devices"
];
let navigationCounter = 0;

export const scenarios = [
  {
    name: "launcher-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "launcher TS webclient ready");
      const appCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "launcher app tiles");
      const systrayCount = await waitForCount(page, ".o_menu_systray [role='menuitem']", 3, "systray entries");
      const launcherChrome = await assertEnterpriseLauncherSnapshot(page);
      return { app_count: appCount, systray_count: systrayCount, launcher_chrome: launcherChrome };
    }
  },
  {
    name: "settings-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&settings_desktop=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "settings TS webclient ready");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Settings", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "settings action ready");
      const blockCount = await waitForCount(page, ".o_web_client .o_action_manager .app_settings_block", 1, "settings blocks");
      const boxCount = await waitForCount(page, ".o_web_client .o_action_manager .o_setting_box", 1, "settings boxes");
      const settingsState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client");
        const action = root?.querySelector(".o_action_manager");
        const buttons = [...(action?.querySelectorAll(".o_setting_action") || [])];
        const labels = buttons.map((button) => button.textContent.trim()).filter(Boolean);
        const targetModels = [...(action?.querySelectorAll("[data-settings-target-model]") || [])].map((node) => [node.textContent.trim(), node.dataset.settingsTargetModel]);
        return {
          brand: root?.querySelector(".o_menu_brand")?.textContent?.trim() || "",
          nav_sections: [...(root?.querySelectorAll(".o_navbar_sections .o_nav_entry") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          labels,
          generic_open_count: labels.filter((label) => label === "Open" || label.startsWith("Open ")).length,
          grid_count: action?.querySelectorAll(".o_setting_grid").length || 0,
          block_titles: [...(action?.querySelectorAll(".app_settings_block h2, .app_settings_block .o_setting_block_title") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          target_models: targetModels,
          has_manage_users: labels.includes("Manage Users"),
          has_manage_groups: labels.includes("Manage Groups"),
          has_server_actions: labels.includes("Server Actions"),
          has_scheduled_actions: labels.includes("Scheduled Actions"),
          has_automation: labels.includes("Automation Rules"),
          has_views: labels.includes("Views"),
          has_models: labels.includes("Models"),
          has_fields: labels.includes("Fields"),
          has_access_rights: labels.includes("Access Rights"),
          has_record_rules: labels.includes("Record Rules"),
          has_mail: labels.includes("Outgoing Mail Servers") && labels.includes("Incoming Mail Servers") && labels.includes("Email Templates"),
          has_apps: labels.includes("Apps"),
          has_ai: labels.includes("AI Apps")
        };
      })()`);
      const expectedNav = ["General Settings", "Users & Companies", "Translations", "Technical"];
      const expectedModels = {
        "Manage Users": "res.users",
        "Manage Groups": "res.groups",
        "Server Actions": "ir.actions.server",
        "Scheduled Actions": "ir.cron",
        "Automation Rules": "base.automation",
        "Views": "ir.ui.view",
        "Models": "ir.model",
        "Fields": "ir.model.fields",
        "Access Rights": "ir.model.access",
        "Record Rules": "ir.rule",
        "Outgoing Mail Servers": "ir.mail_server",
        "Incoming Mail Servers": "fetchmail.server",
        "Email Templates": "mail.template",
        "Apps": "ir.module.module",
        "AI Apps": "ir.module.module"
      };
      const modelMap = Object.fromEntries(settingsState.target_models);
      const missingModels = Object.entries(expectedModels).filter(([label, model]) => modelMap[label] !== model);
      if (
        settingsState.brand !== "Settings" ||
        JSON.stringify(settingsState.nav_sections) !== JSON.stringify(expectedNav) ||
        settingsState.generic_open_count ||
        !settingsState.has_manage_users ||
        !settingsState.has_manage_groups ||
        !settingsState.has_server_actions ||
        !settingsState.has_scheduled_actions ||
        !settingsState.has_automation ||
        !settingsState.has_views ||
        !settingsState.has_models ||
        !settingsState.has_fields ||
        !settingsState.has_access_rights ||
        !settingsState.has_record_rules ||
        !settingsState.has_mail ||
        !settingsState.has_apps ||
        !settingsState.has_ai ||
        settingsState.grid_count < 3 ||
        missingModels.length
      ) {
        throw new Error(`settings layout invalid: ${JSON.stringify({ ...settingsState, missing_models: missingModels })}`);
      }
      return { settings_blocks: blockCount, setting_boxes: boxCount, settings_state: settingsState };
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
      const navCount = await waitForCount(page, ".o_navbar > .o_main_navbar", 1, "TS navbar");
      const appCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "TS app tiles");
      const searchCount = await waitForCount(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", 1, "TS app search");
      const launcherLabels = await evaluate(page, `[...document.querySelectorAll(".o_web_client .o_home_menu .o_app_name")].map((node) => node.textContent.trim())`);
      if (JSON.stringify(launcherLabels) !== JSON.stringify(["Apps", "Settings"])) {
        throw new Error(`launcher labels invalid: ${JSON.stringify(launcherLabels)}`);
      }
      const searchState = await evaluate(page, `(() => {
        const node = document.querySelector(".o_web_client .o_home_menu .o_home_menu_search");
        const input = document.querySelector(".o_web_client .o_home_menu input[aria-label='Search apps and menus']");
        if (!node || !input) return { ok: false, reason: "missing launcher search" };
        const style = getComputedStyle(node);
        const hidden = node.dataset.searchActive === "false" && style.opacity === "0" && Number.parseFloat(style.maxHeight) === 0 && Number.parseFloat(style.marginBottom) <= 1;
        const rect = input.getBoundingClientRect();
        const visible = rect.width >= 300 && rect.height >= 30 && style.display !== "none" && style.visibility !== "hidden";
        return { ok: hidden || visible, hidden, visible, width: rect.width, height: rect.height, margin_bottom_px: Math.round(Number.parseFloat(style.marginBottom) || 0), search_active: node.dataset.searchActive || "" };
      })()`);
      if (!searchState.ok) throw new Error(`TS app search is not usable: ${JSON.stringify(searchState)}`);
      const typedSearchState = await assertLauncherSearchActivation(page);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&launcher_idle=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "launcher idle TS webclient ready");
      await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "launcher idle app tiles");
      const launcherStyle = await assertEnterpriseLauncherSnapshot(page);
      const actionCount = await waitForCount(page, ".o_web_client .o_action_manager", 1, "TS action manager");
      const hasShellCue = await evaluate(page, `document.body.textContent.includes("Gorp") || document.body.textContent.includes("GoERP")`);
      if (hasShellCue) throw new Error("TS takeover exposes non-Odoo shell cue");
      return { nav_count: navCount, app_count: appCount, app_labels: launcherLabels, search_count: searchCount, search_state: searchState, typed_search_state: typedSearchState, ...launcherStyle, action_count: actionCount };
    }
  },
  {
    name: "default-systray-dropdowns-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?debug=1&smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "systray TS webclient ready");
      if (await evaluate(page, `document.querySelector(".o_web_client")?.dataset.view === "apps"`)) {
        await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Settings", ".o_app_name");
        await waitFor(page, `document.querySelector(".o_web_client")?.dataset.view === "action"`, "systray action view active");
      }
      const systrayCount = await waitForCount(page, ".o_web_client .o_menu_systray", 1, "TS systray");
      const selectors = [
        ".o_web_client .o_mail_systray_item",
        ".o_web_client .o_activity_menu",
        ".o_web_client .o_switch_company_menu",
        ".o_web_client .o_debug_manager",
        ".o_web_client .o_user_menu"
      ];
      await waitForCount(page, ".o_web_client .o_web_studio_navbar_item", 1, "Odoo Studio systray control");
      const actionStatusBefore = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus || ""`);
      for (const selector of selectors) {
        await waitForCount(page, selector, 1, `systray item ${selector}`);
        await clickSelector(page, selector);
        await waitFor(page, `(() => {
          const button = document.querySelector(${JSON.stringify(selector)});
          const menu = button?.nextElementSibling;
          return button?.getAttribute("aria-expanded") === "true" && menu?.classList.contains("show") && menu.querySelectorAll("[role='menuitem'], [role='menuitemcheckbox']").length >= 1;
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
    name: "default-user-preferences-dialog-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&prefs_dialog=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "preferences TS webclient ready");
      await clickSelector(page, ".o_web_client .o_user_menu");
      await clickExactText(page, ".o_web_client .o_user_menu + .dropdown-menu [role='menuitem']", "My Preferences");
      const dialogCount = await waitForCount(page, ".o_web_client .gorp-action-dialog[data-model='res.users'][data-dialog-open='true']", 1, "preferences user dialog");
      const state = await evaluate(page, `(() => {
        const dialog = document.querySelector(".o_web_client .gorp-action-dialog[data-model='res.users']");
        const title = dialog?.querySelector(".modal-title")?.textContent?.trim() || "";
        const form = dialog?.querySelector(".gorp-user-preferences-form[data-preferences-dialog='true']");
        const sheet = dialog?.querySelector(".gorp-user-preferences-sheet.o_form_sheet");
        const labels = [...(dialog?.querySelectorAll(".o_form_label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const footer = dialog?.querySelector(".gorp-action-dialog-footer");
        const footerButtons = [...(footer?.querySelectorAll("button") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const rect = dialog?.querySelector(".modal-dialog")?.getBoundingClientRect();
        return {
          title,
          preferences_dialog: dialog?.dataset.preferencesDialog || "",
          has_preferences_form: Boolean(form),
          has_sheet: Boolean(sheet),
          has_reference_labels: ["Language", "Email Signature", "Theme"].every((label) => labels.includes(label)),
          has_preferences_group: Boolean(dialog?.querySelector("[data-preference-group='preferences']")),
          tab_labels: [...(dialog?.querySelectorAll("[data-preferences-tab]") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          admin_tab_count: dialog?.querySelectorAll(".gorp-form-notebook-tab, .gorp-access-smart-button, .o_res_users_access_rights").length || 0,
          footer_save_count: footer?.querySelectorAll("[data-form-action='save']").length || 0,
          footer_discard_count: footer?.querySelectorAll("[data-form-action='discard']").length || 0,
          footer_buttons: footerButtons,
          label_count: labels.length,
          body_modal_open: document.body.classList.contains("modal-open"),
          width: Math.round(rect?.width || 0)
        };
      })()`);
      if (!state.body_modal_open || state.title !== "Change My Preferences" || state.preferences_dialog !== "true" || !state.has_preferences_form || !state.has_sheet || !state.has_reference_labels || !state.has_preferences_group || JSON.stringify(state.tab_labels) !== JSON.stringify(["Preferences", "Calendar", "Security"]) || state.admin_tab_count !== 0 || state.footer_save_count !== 1 || state.footer_discard_count !== 1 || JSON.stringify(state.footer_buttons) !== JSON.stringify(["Update Preferences", "Discard"]) || state.width < 760 || state.width > 1000) {
        throw new Error(`preferences dialog invalid: ${JSON.stringify(state)}`);
      }
      return { dialog_count: dialogCount, state };
    }
  },
  {
    name: "default-navbar-nested-menus-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "nested menu TS webclient ready");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Settings", ".o_app_name");
      await delay(350);
      const settingsClickState = await evaluate(page, `(() => ({
        hash: window.location.hash || "",
        webclient_view: document.querySelector(".o_web_client")?.dataset.view || "",
        action_status: document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus || "",
        active_menu: document.querySelector(".o_web_client > .o_navbar")?.dataset.activeMenuId || "",
        settings_tiles: [...document.querySelectorAll(".o_web_client .o_home_menu .o_app")].filter((node) => node.querySelector(".o_app_name")?.textContent.trim() === "Settings").map((node) => ({ href: node.getAttribute("href") || "", menu_id: node.dataset.menuId || "", action: node.dataset.menuAction || "" }))
      }))()`);
      if (settingsClickState.action_status !== "ready" || !settingsClickState.hash.includes("model=res.config.settings")) {
        throw new Error(`Settings click did not open action: ${JSON.stringify(settingsClickState)}`);
      }
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "nested menu Settings ready");
      await waitForCount(page, ".o_web_client .o_menu_sections .o_nav_dropdown_toggle", 1, "nested navbar dropdown toggles");
      const sections = await evaluate(page, `(() => [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_entry")]
        .map((node) => ({ text: node.textContent.trim(), dropdown: node.classList.contains("o_nav_dropdown_toggle"), menu_id: node.dataset.menuId || "" })))()`);
      if (!sections.some((section) => section.text === "Technical" && section.dropdown)) {
        throw new Error(`Technical navbar section is not a dropdown: ${JSON.stringify(sections)}`);
      }
      await evaluate(page, `(() => {
        const button = [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_dropdown_toggle")]
          .find((node) => node.textContent.trim() === "Technical");
        if (!button) throw new Error("Technical dropdown button not found");
        button.click();
        return true;
      })()`);
      await waitFor(page, `(() => {
        const button = [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_dropdown_toggle")]
          .find((node) => node.textContent.trim() === "Technical");
        const menu = button?.nextElementSibling;
        return button?.getAttribute("aria-expanded") === "true" && menu?.classList.contains("show") ? true : false;
      })()`, "Technical dropdown opens");
      const technicalMenu = await evaluate(page, `(() => {
        const button = [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_dropdown_toggle")]
          .find((node) => node.textContent.trim() === "Technical");
        const menu = button?.nextElementSibling;
        const headers = [...(menu?.querySelectorAll(".o_navbar_dropdown_header") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const items = [...(menu?.querySelectorAll(".o_navbar_dropdown_item") || [])].map((node) => ({
          text: node.textContent.trim(),
          level: node.dataset.menuLevel || "",
          menu_id: node.dataset.menuId || ""
        })).filter((item) => item.text);
        return {
          parent_id: button?.parentElement?.id || "",
          in_legacy_top_menu: Boolean(button && document.querySelector("#topMenu")?.contains(button)),
          webclient_view: document.querySelector(".o_web_client")?.dataset.view || "",
          button_menu_id: button?.dataset.menuId || "",
          button_expanded: button?.getAttribute("aria-expanded") || "",
          menu_class: menu?.className || "",
          menu_hidden: menu?.hidden === true,
          raw_children: [...(menu?.children || [])].map((node) => ({
            tag: node.tagName || "",
            text: node.textContent.trim(),
            class_name: node.className || "",
            menu_id: node.dataset?.menuId || "",
            level: node.dataset?.menuLevel || ""
          })),
          headers,
          items,
          item_labels: items.map((item) => item.text)
        };
      })()`);
      if (technicalMenu.items.length < 8) {
        technicalMenu.payload = await evaluate(page, `(async () => {
          const payload = await fetch("/web/webclient/load_menus").then((response) => response.json());
          const entries = Object.entries(payload).filter(([, value]) => value && typeof value === "object" && value.name === "Technical");
          const nestedEntries = Object.entries(payload.children || {}).filter(([, value]) => value && typeof value === "object" && value.name === "Technical");
          return {
            root_children: payload.root?.children || [],
            menu_roots: payload.menu_roots || [],
            technical_entries: entries.map(([key, value]) => ({ key, id: value.id, children: value.children || [], actionID: value.actionID || false })),
            technical_nested_entries: nestedEntries.map(([key, value]) => ({ key, id: value.id, children: value.children || [], actionID: value.actionID || false }))
          };
        })()`);
      }
      if (technicalMenu.items.length < 8) throw new Error(`Technical dropdown has too few items: ${JSON.stringify(technicalMenu)}`);
      const visibleLabels = technicalMenu.raw_children.map((item) => item.text).filter(Boolean);
      if (JSON.stringify(visibleLabels) !== JSON.stringify(ODOO_TECHNICAL_DROPDOWN_LABELS)) {
        throw new Error(`Technical dropdown reference order mismatch: ${JSON.stringify({ visibleLabels, expected: ODOO_TECHNICAL_DROPDOWN_LABELS })}`);
      }
      for (const label of ["Email", "Actions", "IAP", "User Interface", "Database Structure"]) {
        if (!technicalMenu.headers.includes(label)) throw new Error(`Technical dropdown missing header ${label}: ${JSON.stringify(technicalMenu)}`);
      }
      for (const label of ["Server Actions", "Scheduled Actions", "Scheduled Actions Triggers", "Views", "Menu Items", "Models", "Fields", "Fields Selection", "ManyToMany Relations", "Access Rights", "Record Rules", "Outgoing Mail Servers", "IAP Accounts", "Tours", "Paper Format"]) {
        if (!technicalMenu.item_labels.includes(label)) throw new Error(`Technical dropdown missing item ${label}: ${JSON.stringify(technicalMenu)}`);
      }
      for (const label of ["Users", "Groups", "Companies", "Languages", "Automation Rules", "Apps", "Scheduled Messages", "Email Templates", "Incoming Mail Servers", "Incoming Mail Server"]) {
        if (visibleLabels.includes(label)) throw new Error(`Technical dropdown exposes non-reference label: ${JSON.stringify({ label, visibleLabels })}`);
      }
      await clickExactText(page, ".o_web_client .o_navbar_dropdown_menu.show .o_navbar_dropdown_item", "Server Actions");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "nested menu Server Actions ready");
      const activeTitle = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      if (activeTitle !== "Server Actions") throw new Error(`Technical dropdown did not open Server Actions: ${activeTitle}`);
      return { sections, technical_menu: technicalMenu, active_title: activeTitle };
    }
  },
  {
    name: "default-navbar-technical-dropdown-open-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "open dropdown TS webclient ready");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Settings", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "open dropdown Settings action ready");
      await evaluate(page, `(() => {
        const button = [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_dropdown_toggle")]
          .find((node) => node.textContent.trim() === "Technical");
        if (!button) throw new Error("Technical dropdown button not found");
        button.click();
        return true;
      })()`);
      const state = await waitFor(page, `(() => {
        const button = [...document.querySelectorAll(".o_web_client .o_menu_sections .o_nav_dropdown_toggle")]
          .find((node) => node.textContent.trim() === "Technical");
        const menu = button?.nextElementSibling;
        if (!button || !menu || button.getAttribute("aria-expanded") !== "true" || !menu.classList.contains("show")) return null;
        const rect = menu.getBoundingClientRect();
        const pointX = Math.min(Math.max(rect.left + 24, rect.left + 1), rect.right - 1);
        const pointY = Math.min(Math.max(rect.top + 24, rect.top + 1), rect.bottom - 1);
        const topNode = document.elementFromPoint(pointX, pointY);
        const headers = [...menu.querySelectorAll(".o_navbar_dropdown_header")].map((node) => node.textContent.trim()).filter(Boolean);
        const items = [...menu.querySelectorAll(".o_navbar_dropdown_item")].map((node) => node.textContent.trim()).filter(Boolean);
        const visibleLabels = [...menu.children].map((node) => node.textContent.trim()).filter(Boolean);
        return {
          button_text: button.textContent.trim(),
          expanded: button.getAttribute("aria-expanded"),
          menu_visible: rect.width > 220 && rect.height > 250,
          menu_left: Math.round(rect.left),
          menu_top: Math.round(rect.top),
          hit_test_x: Math.round(pointX),
          hit_test_y: Math.round(pointY),
          dropdown_on_top: Boolean(topNode && menu.contains(topNode)),
          top_element: topNode ? topNode.tagName + "." + String(topNode.className || "").replace(/\\s+/g, ".") : "",
          headers,
          items,
          visibleLabels,
          has_grouped_sections: headers.length >= 5,
          has_admin_items: ${JSON.stringify(["Server Actions", "Scheduled Actions", "Scheduled Actions Triggers", "Views", "Menu Items", "Models", "Fields", "Fields Selection", "ManyToMany Relations", "Access Rights", "Record Rules", "IAP Accounts", "Tours", "Paper Format"])}.every((label) => items.includes(label)),
          reference_order: JSON.stringify(visibleLabels) === ${JSON.stringify(JSON.stringify(ODOO_TECHNICAL_DROPDOWN_LABELS))},
          hidden_custom_first_page: !visibleLabels.some((label) => ${JSON.stringify(["Users", "Groups", "Companies", "Languages", "Automation Rules", "Apps", "Scheduled Messages", "Email Templates", "Incoming Mail Servers", "Incoming Mail Server"])}.includes(label))
        };
      })()`, "Technical dropdown remains open");
      if (!state.menu_visible || !state.dropdown_on_top || !state.has_grouped_sections || !state.has_admin_items || !state.reference_order || !state.hidden_custom_first_page) {
        throw new Error(`Technical dropdown open state invalid: ${JSON.stringify(state)}`);
      }
      return state;
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
      const settingsTargets = await evaluate(page, `(() => {
        const required = [
          "users",
          "groups",
          "languages",
          "company_records",
          "companies",
          "server_actions",
          "scheduled_actions",
          "automation_rules",
          "views",
          "models",
          "fields",
          "access_rights",
          "record_rules",
          "mail_servers",
          "fetchmail_servers",
          "email_templates",
          "apps",
          "ai"
        ];
        const buttons = [...document.querySelectorAll(".o_web_client .o_action_manager [data-settings-target]")];
        const ids = buttons.map((button) => button.dataset.settingsTarget).filter(Boolean);
        const models = Object.fromEntries(buttons.map((button) => [button.dataset.settingsTarget, button.dataset.settingsTargetModel || ""]));
        const labels = Object.fromEntries(buttons.map((button) => [button.dataset.settingsTarget, button.textContent.trim()]));
        const gridCount = document.querySelectorAll(".o_web_client .o_action_manager .o_setting_grid").length;
        const actionBoxCount = document.querySelectorAll(".o_web_client .o_action_manager .o_setting_box[data-has-settings-action='true']").length;
        const inviteCount = document.querySelectorAll(".o_web_client .o_action_manager [data-settings-action='invite-users']").length;
        const appTabs = [...document.querySelectorAll(".o_web_client .o_action_manager .o_settings_tab")].map((node) => node.textContent.trim()).filter(Boolean);
        const blockTitles = [...document.querySelectorAll(".o_web_client .o_action_manager .o_settings_block_title")].map((node) => node.textContent.trim()).filter(Boolean);
        const missing = required.filter((id) => !ids.includes(id));
        return { count: buttons.length, ids, models, labels, gridCount, actionBoxCount, inviteCount, appTabs, blockTitles, missing };
      })()`);
      const settingsChrome = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager");
        const controlPanel = root?.querySelector(".o_control_panel");
        const settings = root?.querySelector(".o_settings_container");
        const cpSearch = controlPanel?.querySelector(".o_cp_searchview");
        const internalSearch = settings?.querySelector(".o_settings_search_panel");
        const switchers = controlPanel?.querySelectorAll(".o_switch_view, .o_view_switcher button, [data-view-switch]").length || 0;
        const controlRect = controlPanel?.getBoundingClientRect();
        const searchRect = cpSearch?.getBoundingClientRect();
        const settingsRect = settings?.getBoundingClientRect();
        return {
          cp_search_count: cpSearch ? 1 : 0,
          internal_search_count: internalSearch ? 1 : 0,
          view_switcher_count: switchers,
          control_height: Math.round(controlRect?.height || 0),
          search_top: Math.round(searchRect?.top || 0),
          settings_top: Math.round(settingsRect?.top || 0)
        };
      })()`);
      if (settingsChrome.cp_search_count !== 1 || settingsChrome.internal_search_count !== 0 || settingsChrome.view_switcher_count !== 0 || settingsChrome.settings_top - settingsChrome.search_top > 80) {
        throw new Error(`TS Settings chrome invalid: ${JSON.stringify(settingsChrome)}`);
      }
      if (settingsTargets.missing.length) {
        throw new Error(`TS Settings navigation targets missing: ${settingsTargets.missing.join(", ")}`);
      }
      const genericOpenLabels = Object.values(settingsTargets.labels).filter((label) => String(label).startsWith("Open "));
      const expectedBlocks = ["Users", "Languages", "Companies", "Contacts", "Technical"];
      const expectedTechnicalTargets = {
        server_actions: ["Server Actions", "ir.actions.server"],
        scheduled_actions: ["Scheduled Actions", "ir.cron"],
        automation_rules: ["Automation Rules", "base.automation"],
        views: ["Views", "ir.ui.view"],
        models: ["Models", "ir.model"],
        fields: ["Fields", "ir.model.fields"],
        access_rights: ["Access Rights", "ir.model.access"],
        record_rules: ["Record Rules", "ir.rule"],
        mail_servers: ["Outgoing Mail Servers", "ir.mail_server"],
        fetchmail_servers: ["Incoming Mail Servers", "fetchmail.server"],
        email_templates: ["Email Templates", "mail.template"],
        apps: ["Apps", "ir.module.module"],
        ai: ["AI Apps", "ir.module.module"]
      };
      const invalidTechnicalTargets = Object.entries(expectedTechnicalTargets)
        .filter(([id, [label, model]]) => settingsTargets.labels[id] !== label || settingsTargets.models[id] !== model)
        .map(([id]) => id);
      if (settingsTargets.gridCount !== 5 || settingsTargets.actionBoxCount !== 18 || settingsTargets.inviteCount !== 1 || genericOpenLabels.length || invalidTechnicalTargets.length || settingsTargets.labels.users !== "Manage Users" || settingsTargets.labels.languages !== "Languages" || JSON.stringify(settingsTargets.appTabs) !== JSON.stringify(["General Settings"]) || JSON.stringify(settingsTargets.blockTitles) !== JSON.stringify(expectedBlocks)) {
        throw new Error(`TS Settings action layout invalid: ${JSON.stringify(settingsTargets)}`);
      }
      const saveDisabled = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager [data-settings-action='save']")?.disabled === true`);
      const discardDisabled = await evaluate(page, `document.querySelector(".o_web_client .o_action_manager [data-settings-action='discard']")?.disabled === true`);
      const topbarState = await evaluate(page, `(() => {
        const navbar = document.querySelector(".o_web_client > .o_navbar > .o_main_navbar");
        const launcher = navbar?.querySelector(".o-launcher-button");
        const launcherDot = navbar?.querySelector(".o-launcher span");
        const style = navbar ? getComputedStyle(navbar) : null;
        const dotStyle = launcherDot ? getComputedStyle(launcherDot) : null;
        return {
          contract: Boolean(navbar),
          height: Math.round(navbar?.getBoundingClientRect().height || 0),
          background: style?.backgroundColor || "",
          launcher_width: Math.round(launcher?.getBoundingClientRect().width || 0),
          launcher_dot: dotStyle?.backgroundColor || "",
          systray_count: document.querySelectorAll(".o_web_client .o_menu_systray [role='menuitem']").length
        };
      })()`);
      const allowedTopbarBackgrounds = new Set(["rgb(38, 42, 54)"]);
      if (!topbarState.contract || topbarState.height < 44 || topbarState.height > 48 || !allowedTopbarBackgrounds.has(topbarState.background) || topbarState.launcher_width < 30 || !["rgb(113, 75, 103)", "rgb(135, 90, 123)"].includes(topbarState.launcher_dot) || topbarState.systray_count < 4) {
        throw new Error(`TS action topbar contract invalid: ${JSON.stringify(topbarState)}`);
      }
      const title = await textContent(page, ".o_web_client .o_action_manager .o_breadcrumb .active");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("action=") && hash.includes("model=res.config.settings") && hash.includes("menu_id=") ? hash : "";
      })()`, "TS action route hash");
      return { title, hash, window_count: windowCount, control_panel_count: controlPanelCount, settings_count: settingsCount, settings_targets: settingsTargets, settings_chrome: settingsChrome, topbar_state: topbarState, ...settingsLabelAudit, save_disabled: saveDisabled, discard_disabled: discardDisabled };
    }
  },
  {
    name: "default-launcher-back-mode-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "launcher back-mode TS webclient ready");
      await clickText(page, ".o_web_client .o_home_menu .o_app", "Settings");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "launcher back-mode action ready");
      await waitForCount(page, ".o_web_client .o_action_manager .o_settings_container", 1, "launcher back-mode Settings action");
      const before = await evaluate(page, `(() => {
        const shell = document.querySelector("main.o_web_client");
        const launcher = document.querySelector(".o_web_client > .o_navbar .o_menu_toggle");
        return {
          view: shell?.dataset?.view || "",
          mode: shell?.dataset?.homeMenuMode || "",
          home_count: document.querySelectorAll(".o_web_client .o_home_menu").length,
          settings_count: document.querySelectorAll(".o_web_client .o_action_manager .o_settings_container").length,
          back_class: launcher?.classList.contains("o_menu_toggle_back") || false
        };
      })()`);
      if (before.view !== "action" || before.mode || before.home_count || before.settings_count !== 1 || before.back_class) {
        throw new Error(`launcher back-mode initial state invalid: ${JSON.stringify(before)}`);
      }
      await clickSelector(page, ".o_web_client > .o_navbar .o_menu_toggle");
      await waitForCount(page, ".o_web_client .o_action_manager .o_home_menu", 1, "launcher back-mode overlay home menu");
      const overlay = await evaluate(page, `(() => {
        const shell = document.querySelector("main.o_web_client");
        const launcher = document.querySelector(".o_web_client > .o_navbar .o_menu_toggle");
        const navbarApps = document.querySelector(".o_web_client > .o_navbar .o_navbar_apps_menu");
        const navbarStyle = navbarApps ? getComputedStyle(navbarApps) : null;
        const navbar = document.querySelector(".o_web_client > .o_navbar > .o_main_navbar");
        const navbarBg = navbar ? getComputedStyle(navbar).backgroundColor : "";
        const title = document.querySelector(".o_web_client > .o_navbar .o_menu_brand")?.textContent?.trim() || "";
        return {
          view: shell?.dataset?.view || "",
          mode: shell?.dataset?.homeMenuMode || "",
          body_home_background: document.body.classList.contains("o_home_menu_background"),
          shell_home_background: shell?.classList.contains("o_home_menu_background") || false,
          back_class: launcher?.classList.contains("o_menu_toggle_back") || false,
          launcher_visible: Boolean(launcher && launcher.getBoundingClientRect().width >= 30 && launcher.getBoundingClientRect().height >= 40),
          navbar_apps_display: navbarStyle?.display || "",
          navbar_background: navbarBg,
          title,
          home_count: document.querySelectorAll(".o_web_client .o_action_manager .o_home_menu").length,
          settings_count: document.querySelectorAll(".o_web_client .o_action_manager .o_settings_container").length
        };
      })()`);
      if (overlay.view !== "apps" || overlay.mode !== "overlay" || !overlay.body_home_background || !overlay.shell_home_background || !overlay.back_class || !overlay.launcher_visible || overlay.navbar_apps_display === "none" || overlay.navbar_background !== "rgba(0, 0, 0, 0)" || overlay.title !== "Settings" || overlay.home_count !== 1 || overlay.settings_count !== 0) {
        throw new Error(`launcher back-mode overlay invalid: ${JSON.stringify(overlay)}`);
      }
      await clickSelector(page, ".o_web_client > .o_navbar .o_menu_toggle_back");
      await waitForCount(page, ".o_web_client .o_action_manager .o_settings_container", 1, "launcher back-mode restored Settings action");
      const restored = await evaluate(page, `(() => {
        const shell = document.querySelector("main.o_web_client");
        const launcher = document.querySelector(".o_web_client > .o_navbar .o_menu_toggle");
        return {
          view: shell?.dataset?.view || "",
          mode: shell?.dataset?.homeMenuMode || "",
          body_home_background: document.body.classList.contains("o_home_menu_background"),
          shell_home_background: shell?.classList.contains("o_home_menu_background") || false,
          back_class: launcher?.classList.contains("o_menu_toggle_back") || false,
          home_count: document.querySelectorAll(".o_web_client .o_action_manager .o_home_menu").length,
          settings_count: document.querySelectorAll(".o_web_client .o_action_manager .o_settings_container").length,
          action_status: document.querySelector(".o_web_client .o_action_manager")?.dataset?.tsActionStatus || ""
        };
      })()`);
      if (restored.view !== "action" || restored.mode || restored.body_home_background || restored.shell_home_background || restored.back_class || restored.home_count !== 0 || restored.settings_count !== 1 || restored.action_status !== "ready") {
        throw new Error(`launcher back-mode restore invalid: ${JSON.stringify(restored)}`);
      }
      await clickSelector(page, ".o_web_client > .o_navbar .o_menu_toggle");
      await waitForCount(page, ".o_web_client .o_action_manager .o_home_menu", 1, "launcher back-mode final overlay home menu");
      const finalOverlay = await evaluate(page, `(() => {
        const shell = document.querySelector("main.o_web_client");
        const launcher = document.querySelector(".o_web_client > .o_navbar .o_menu_toggle");
        return {
          view: shell?.dataset?.view || "",
          mode: shell?.dataset?.homeMenuMode || "",
          back_class: launcher?.classList.contains("o_menu_toggle_back") || false,
          home_count: document.querySelectorAll(".o_web_client .o_action_manager .o_home_menu").length,
          settings_count: document.querySelectorAll(".o_web_client .o_action_manager .o_settings_container").length
        };
      })()`);
      if (finalOverlay.view !== "apps" || finalOverlay.mode !== "overlay" || !finalOverlay.back_class || finalOverlay.home_count !== 1 || finalOverlay.settings_count !== 0) {
        throw new Error(`launcher back-mode final overlay invalid: ${JSON.stringify(finalOverlay)}`);
      }
      return { before, overlay, restored, final_overlay: finalOverlay };
    }
  },
  {
    name: "default-action-dialog-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&dialog_auth=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "dialog auth document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=web.action_base_document_layout_configurator&view_type=form`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "dialog route document ready");
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "dialog route TS webclient ready");
      const dialogCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-action-dialog[data-target='new'][data-dialog-open='true']", 1, "TS target-new action dialog");
      const backdropCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-action-dialog .gorp-action-dialog-backdrop", 1, "TS target-new action dialog backdrop");
      const state = await evaluate(page, `(() => {
        const dialog = document.querySelector(".o_web_client .o_action_manager .gorp-action-dialog");
        const backdrops = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-action-dialog-backdrop")];
        const modal = dialog?.querySelector(".modal.o_dialog_container");
        const body = dialog?.querySelector(".modal-body.o_act_window");
        const footer = dialog?.querySelector(".gorp-action-dialog-footer");
        const title = dialog?.querySelector(".modal-title")?.textContent?.trim() || "";
        const close = dialog?.querySelector(".btn-close");
        const rect = dialog?.querySelector(".modal-dialog")?.getBoundingClientRect();
        const footerRect = footer?.getBoundingClientRect();
        return {
          body_modal_open: document.body.classList.contains("modal-open"),
          dialog_open: dialog?.dataset?.dialogOpen || "",
          model: dialog?.dataset?.model || "",
          backdrop_count: backdrops.length,
          backdrop_in_dialog: backdrops.every((node) => node.parentElement === dialog),
          modal_role: modal?.getAttribute("role") || "",
          close_label: close?.getAttribute("aria-label") || "",
          title,
          body_count: body ? 1 : 0,
          body_control_panel_count: dialog?.querySelectorAll(".modal-body .o_control_panel").length || 0,
          footer_save_count: footer?.querySelectorAll("[data-settings-action='save'], [data-form-action='save']").length || 0,
          footer_discard_count: footer?.querySelectorAll("[data-settings-action='discard'], [data-form-action='discard']").length || 0,
          footer_bottom: Math.round(footerRect?.bottom || 0),
          viewport_height: window.innerHeight,
          width: Math.round(rect?.width || 0),
          height: Math.round(rect?.height || 0)
        };
      })()`);
      if (!state.body_modal_open || state.dialog_open !== "true" || state.backdrop_count !== 1 || !state.backdrop_in_dialog || state.modal_role !== "dialog" || state.close_label !== "Close" || !state.title || state.body_count !== 1 || state.body_control_panel_count !== 0 || state.footer_save_count !== 1 || state.footer_discard_count !== 1 || state.footer_bottom > state.viewport_height || state.width < 360 || state.height < 180) {
        throw new Error(`target-new dialog state invalid: ${JSON.stringify(state)}`);
      }
      return { dialog_count: dialogCount, backdrop_count: backdropCount, state };
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
        const model_values = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view td[data-field='model_name'], .o_web_client .o_action_manager .gorp-list-view td[data-field='model_id']")]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        const usage_values = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view td[data-field='usage']")]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        return { headers, state, model_values, usage_values };
      })()`);
      for (const label of ["Name", "Model", "Type", "Usage"]) {
        if (!labelState.headers.includes(label)) throw new Error(`TS technical list missing header ${label}: ${JSON.stringify(labelState)}`);
      }
      if (labelState.state === "code") throw new Error(`TS technical list shows raw state value: ${JSON.stringify(labelState)}`);
      if (labelState.model_values.some((value) => /^[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)*$/.test(value))) throw new Error(`TS technical list shows raw model value: ${JSON.stringify(labelState)}`);
      if (labelState.usage_values.some((value) => /^ir_/.test(value) || value.includes("_"))) throw new Error(`TS technical list shows raw usage value: ${JSON.stringify(labelState)}`);
      if (!labelState.usage_values.some((value) => value === "Scheduled Action" || value === "Server Action")) throw new Error(`TS technical list missing usage labels: ${JSON.stringify(labelState)}`);
      return { title, hash, ...opened, ...themeAudit, label_state: labelState };
    }
  },
  {
    name: "default-technical-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      const opened = await openDefaultServerActionsList(page, config, desktopViewport(), { preferredName: "Base: Auto-vacuum internal data" });
      const { action_id: actionID, menu_id: menuID, record_id: recordID } = opened.route_state;
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${encodeURIComponent(actionID)}&model=ir.actions.server&view_type=form&id=${encodeURIComponent(recordID)}&menu_id=${encodeURIComponent(menuID)}`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS technical form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "TS Server Actions form");
      const fieldCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-form-field", 1, "TS Server Actions form fields");
      const formIdentityState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const form = root?.querySelector(".gorp-form-view");
        return {
          breadcrumb: root?.querySelector(".o_control_panel_breadcrumbs .breadcrumb-item.active, .o_control_panel_breadcrumbs .active")?.textContent?.trim() || "",
          text: form?.textContent || ""
        };
      })()`);
      if (!formIdentityState.text.includes("Base: Auto-vacuum internal data")) {
        throw new Error(`TS Server Actions form opened wrong record: ${JSON.stringify(formIdentityState)}`);
      }
      const formControlState = await evaluate(page, `(() => ({
        search_inputs: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_searchview_input").length,
        search_toggles: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_searchview_dropdown_toggler").length
      }))()`);
      if (formControlState.search_inputs !== 0 || formControlState.search_toggles !== 0) {
        throw new Error(`TS Server Actions form exposes list search controls: ${JSON.stringify(formControlState)}`);
      }
      const formToolbarState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const mainButtons = root?.querySelector(".o_control_panel_main_buttons");
        const actions = root?.querySelector(".o_control_panel_actions");
        const activeBreadcrumb = root?.querySelector(".o_control_panel_breadcrumbs .breadcrumb-item.active, .o_control_panel_breadcrumbs .active");
        const actionMenus = [...(root?.querySelectorAll(".gorp-form-action-menu") || [])];
        const actionLabels = [...(root?.querySelectorAll(".gorp-form-action-menu .gorp-action-menu-toggle") || [])]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        const rect = (node) => {
          const box = node?.getBoundingClientRect();
          return box ? { left: box.left, right: box.right, top: box.top, bottom: box.bottom, width: box.width, height: box.height } : null;
        };
        const intersects = (left, right) => left && right && left.left < right.right - 1 && left.right > right.left + 1 && left.top < right.bottom - 1 && left.bottom > right.top + 1;
        return {
          action_menu_count: actionMenus.length,
          main_button_action_menu_count: mainButtons?.querySelectorAll(".gorp-form-action-menu").length || 0,
          action_lane_action_menu_count: actions?.querySelectorAll(".gorp-form-action-menu").length || 0,
          duplicate_actions_label_count: actionLabels.filter((label) => label === "Actions").length,
          action_labels: actionLabels,
          main_overlaps_breadcrumb: intersects(rect(mainButtons), rect(activeBreadcrumb)),
          actions_overlaps_breadcrumb: intersects(rect(actions), rect(activeBreadcrumb))
        };
      })()`);
      if (formToolbarState.action_menu_count !== 1 || formToolbarState.main_button_action_menu_count !== 0 || formToolbarState.action_lane_action_menu_count !== 1 || formToolbarState.duplicate_actions_label_count > 1 || formToolbarState.main_overlaps_breadcrumb || formToolbarState.actions_overlaps_breadcrumb) {
        throw new Error(`TS Server Actions form toolbar invalid: ${JSON.stringify(formToolbarState)}`);
      }
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
      const contextualButtonCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-contextual[data-server-action-contextual='true']", 1, "TS Server Actions contextual action button");
      const smartButtonCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-server-action-smart-button[data-server-action-smart-button='scheduled-action']", 1, "TS Server Actions scheduled smart button");
      const serverActionChromeState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const form = root?.querySelector(".gorp-form-view");
        const labels = [...(form?.querySelectorAll(".o_form_label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const contextual = form?.querySelector(".gorp-server-action-contextual[data-server-action-contextual='true']");
        const smart = root?.querySelector(".gorp-server-action-smart-button[data-server-action-smart-button='scheduled-action']");
        return {
          model_label_count: labels.filter((label) => label === "Model").length,
          model_name_field_count: form?.querySelectorAll(".gorp-form-field[data-field='model_name']").length || 0,
          contextual_text: contextual?.textContent?.trim() || "",
          smart_text: smart?.textContent?.trim() || "",
          smart_count: smart?.dataset?.count || ""
        };
      })()`);
      if (serverActionChromeState.model_label_count !== 1 || serverActionChromeState.model_name_field_count !== 0 || serverActionChromeState.contextual_text !== "Create Contextual Action" || serverActionChromeState.smart_text !== "Scheduled Action" || !serverActionChromeState.smart_count) {
        throw new Error(`TS Server Actions form chrome invalid: ${JSON.stringify(serverActionChromeState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const stateRadioCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-selection-radio-group[data-field='state'] input[type='radio']", 1, "TS Server Actions state radio editor");
      const codeEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-server-action-notebook .gorp-code-editor[data-field='code']", 1, "TS Server Actions code editor");
      const relationEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'][data-relation='ir.model']", 1, "TS Server Actions many2one editor");
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-dropdown-toggle");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option", 1, "TS Server Actions many2one dropdown opens empty");
      const relationDropdownState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const toggle = editor?.querySelector(".gorp-many2one-dropdown-toggle");
        const input = editor?.querySelector("input");
        const dropdown = editor?.querySelector(".gorp-many2one-dropdown");
        return {
          toggle_expanded: toggle?.getAttribute("aria-expanded") || "",
          input_expanded: input?.getAttribute("aria-expanded") || "",
          dropdown_open: dropdown?.hidden === false,
          option_count: editor?.querySelectorAll(".gorp-many2one-option").length || 0,
          search_more_count: editor?.querySelectorAll(".gorp-many2one-search-more").length || 0,
          selected_count: editor?.querySelectorAll(".gorp-many2one-option[data-selected='true']").length || 0,
          active_count: editor?.querySelectorAll(".gorp-many2one-option[data-active='true']").length || 0,
          option_labels: [...(editor?.querySelectorAll(".gorp-many2one-option") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          search_more_label: editor?.querySelector(".gorp-many2one-search-more")?.textContent?.trim() || "",
          current_res_id: editor?.dataset?.resId || ""
        };
      })()`);
      if (relationDropdownState.toggle_expanded !== "true" || relationDropdownState.input_expanded !== "true" || !relationDropdownState.dropdown_open || relationDropdownState.option_count < 2 || relationDropdownState.search_more_count < 1 || relationDropdownState.selected_count < 1 || relationDropdownState.active_count < 1 || relationDropdownState.search_more_label !== "Search more..." || !relationDropdownState.current_res_id) {
        throw new Error(`TS Server Actions many2one dropdown invalid: ${JSON.stringify(relationDropdownState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-search-more");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']")?.dataset.searchMoreOpened === "true"`, "TS Server Actions many2one search more opens");
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
      return { title, hash, opened, form_count: formCount, field_count: fieldCount, form_identity_state: formIdentityState, form_control_state: formControlState, form_toolbar_state: formToolbarState, server_action_band_count: serverActionBandCount, server_action_notebook_count: serverActionNotebookCount, code_viewer_count: codeViewerCount, selection_pill_count: selectionPillCount, contextual_button_count: contextualButtonCount, scheduled_smart_button_count: smartButtonCount, server_action_chrome_state: serverActionChromeState, state_radio_count: stateRadioCount, code_editor_count: codeEditorCount, relation_link_count: relationLinkCount, relation_state: relationState, relation_editor_count: relationEditorCount, relation_dropdown_state: relationDropdownState, relation_option_count: relationOptionCount, relation_editor_state: editorState, relation_selected_state: selectedState };
    }
  },
  {
    name: "default-server-action-new-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      const opened = await openDefaultServerActionsList(page, config, desktopViewport());
      const { action_id: actionID, menu_id: menuID } = opened.route_state;
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${encodeURIComponent(actionID)}&model=ir.actions.server&view_type=form&menu_id=${encodeURIComponent(menuID)}`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS Server Actions new form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "TS Server Actions new form");
      const titleInputCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-title-input[data-field='name']", 1, "TS Server Actions new title input");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const form = root?.querySelector(".gorp-form-view");
        const labels = [...(form?.querySelectorAll(".o_form_label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const title = form?.querySelector(".gorp-form-title-input[data-field='name']");
        const save = root?.querySelector("[data-form-action='save']");
        const edit = root?.querySelector("[data-form-action='edit']");
        return {
          title_placeholder: title?.getAttribute("placeholder") || "",
          labels,
          save_hidden: save?.hidden === true,
          edit_hidden: edit?.hidden === true,
          code_notebook_count: form?.querySelectorAll(".gorp-server-action-notebook").length || 0,
          state_field_count: form?.querySelectorAll(".gorp-form-field[data-field='state']").length || 0,
          active_field_count: form?.querySelectorAll(".gorp-form-field[data-field='active']").length || 0,
          model_field_count: form?.querySelectorAll(".gorp-form-field[data-field='model_id']").length || 0,
          groups_field_count: form?.querySelectorAll(".gorp-form-field[data-field='group_ids']").length || 0,
          search_input_count: root?.querySelectorAll(".o_searchview_input").length || 0
        };
      })()`);
      if (state.title_placeholder !== "Set an explicit name" || JSON.stringify(state.labels) !== JSON.stringify(["Model", "Allowed Groups"]) || state.save_hidden || !state.edit_hidden || state.code_notebook_count !== 0 || state.state_field_count !== 0 || state.active_field_count !== 0 || state.model_field_count !== 1 || state.groups_field_count !== 1 || state.search_input_count !== 0) {
        throw new Error(`Server Actions new form parity invalid: ${JSON.stringify(state)}`);
      }
      return { opened, form_count: formCount, title_input_count: titleInputCount, state };
    }
  },
  {
    name: "default-relation-dropdown-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      const opened = await openDefaultServerActionsList(page, config, desktopViewport());
      const { action_id: actionID, menu_id: menuID, record_id: recordID } = opened.route_state;
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${encodeURIComponent(actionID)}&model=ir.actions.server&view_type=form&id=${encodeURIComponent(recordID)}&menu_id=${encodeURIComponent(menuID)}`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS relation dropdown form ready");
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'][data-relation='ir.model']", 1, "relation dropdown editor");
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-dropdown-toggle");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option", 2, "relation dropdown open-state options");
      const initialState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const options = [...(editor?.querySelectorAll(".gorp-many2one-option") || [])];
        return {
          placement: editor?.dataset.dropdownPlacement || "",
          selected_count: options.filter((node) => node.dataset.selected === "true").length,
          active_count: options.filter((node) => node.dataset.active === "true").length,
          labels: options.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (initialState.placement !== "bottom-start" || initialState.selected_count < 1 || initialState.active_count < 1) {
        throw new Error(`relation dropdown selected-open state invalid: ${JSON.stringify(initialState)}`);
      }
      await setInput(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] input", "mail");
      await waitFor(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const labels = [...(editor?.querySelectorAll(".gorp-many2one-option") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        return labels.some((label) => label === "Mail Server");
      })()`, "relation dropdown typed mail result");
      const state = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id']");
        const input = editor?.querySelector("input");
        const dropdown = editor?.querySelector(".gorp-many2one-dropdown");
        const options = [...(editor?.querySelectorAll(".gorp-many2one-option") || [])];
        return {
          open: dropdown?.hidden === false,
          placement: editor?.dataset.dropdownPlacement || "",
          dropdown_placement: dropdown?.dataset.placement || "",
          dropdown_width_source: dropdown?.dataset.widthSource || "",
          input_value: input?.value || "",
          expanded: input?.getAttribute("aria-expanded") || "",
          active_descendant: input?.getAttribute("aria-activedescendant") || "",
          option_count: options.length,
          selected_count: options.filter((node) => node.dataset.selected === "true").length,
          active_count: options.filter((node) => node.dataset.active === "true").length,
          labels: options.map((node) => node.textContent.trim()).filter(Boolean),
          raw_label_count: options.filter((node) => /\\b[a-z_]+\\.[a-z0-9_.]+\\b/.test(node.textContent.trim())).length,
          create_count: editor?.querySelectorAll(".gorp-many2one-create").length || 0,
          create_edit_count: editor?.querySelectorAll(".gorp-many2one-create-edit").length || 0,
          search_more_label: editor?.querySelector(".gorp-many2one-search-more")?.textContent?.trim() || "",
          dropdown_width: Math.round(dropdown?.getBoundingClientRect().width || 0),
          input_width: Math.round(input?.getBoundingClientRect().width || 0)
        };
      })()`);
      if (!state.open || state.placement !== "bottom-start" || state.dropdown_placement !== "bottom-start" || state.dropdown_width_source !== "field" || state.input_value !== "mail" || state.expanded !== "true" || state.option_count !== 1 || state.active_count < 1 || JSON.stringify(state.labels) !== JSON.stringify(["Mail Server"]) || state.raw_label_count !== 0 || state.create_count !== 0 || state.create_edit_count !== 0 || state.search_more_label !== "Search more..." || state.dropdown_width < 155 || state.dropdown_width > state.input_width + 4) {
        throw new Error(`relation dropdown parity state invalid: ${JSON.stringify(state)}`);
      }
      await setInput(page, ".o_web_client .o_action_manager .gorp-x2many-editor[data-field='group_ids'] input", "user");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-editor[data-field='group_ids'] .gorp-x2many-option", 1, "relation x2many dropdown options");
      const x2ManyInitialState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-x2many-editor[data-field='group_ids']");
        const input = editor?.querySelector("input");
        const dropdown = editor?.querySelector(".gorp-x2many-dropdown");
        const buttons = [...(dropdown?.querySelectorAll("button") || [])];
        const options = [...(editor?.querySelectorAll(".gorp-x2many-option") || [])];
        return {
          open: dropdown?.hidden === false,
          placement: editor?.dataset.dropdownPlacement || "",
          dropdown_placement: dropdown?.dataset.placement || "",
          dropdown_width_source: dropdown?.dataset.widthSource || "",
          expanded: input?.getAttribute("aria-expanded") || "",
          active_descendant: input?.getAttribute("aria-activedescendant") || "",
          active_index: dropdown?.dataset.activeIndex || "",
          active_count: buttons.filter((node) => node.dataset.active === "true").length,
          button_count: buttons.length,
          option_count: options.length,
          labels: options.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (!x2ManyInitialState.open || x2ManyInitialState.placement !== "bottom-start" || x2ManyInitialState.dropdown_placement !== "bottom-start" || x2ManyInitialState.dropdown_width_source !== "field" || x2ManyInitialState.expanded !== "true" || !x2ManyInitialState.active_descendant || x2ManyInitialState.active_index !== "0" || x2ManyInitialState.active_count !== 1 || x2ManyInitialState.option_count < 1) {
        throw new Error(`relation x2many dropdown initial state invalid: ${JSON.stringify(x2ManyInitialState)}`);
      }
      const x2ManyKeyboardState = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-x2many-editor[data-field='group_ids']");
        const input = editor?.querySelector("input");
        const dropdown = editor?.querySelector(".gorp-x2many-dropdown");
        input?.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true, cancelable: true }));
        const buttons = [...(dropdown?.querySelectorAll("button") || [])];
        return {
          active_descendant: input?.getAttribute("aria-activedescendant") || "",
          active_index: dropdown?.dataset.activeIndex || "",
          active_count: buttons.filter((node) => node.dataset.active === "true").length,
          button_count: buttons.length
        };
      })()`);
      if (!x2ManyKeyboardState.active_descendant || x2ManyKeyboardState.active_count !== 1 || (x2ManyKeyboardState.button_count > 1 && x2ManyKeyboardState.active_index === "0")) {
        throw new Error(`relation x2many dropdown keyboard state invalid: ${JSON.stringify(x2ManyKeyboardState)}`);
      }
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${encodeURIComponent(actionID)}&model=ir.actions.server&view_type=form&id=${encodeURIComponent(recordID)}&menu_id=${encodeURIComponent(menuID)}`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS relation dropdown final form ready");
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'][data-relation='ir.model']", 1, "relation dropdown final editor");
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-dropdown-toggle");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-many2one-editor[data-field='model_id'] .gorp-many2one-option", 2, "relation dropdown final open-state options");
      const finalState = await evaluate(page, `(() => {
        const action = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const many2one = action?.querySelector(".gorp-many2one-editor[data-field='model_id']");
        const x2many = action?.querySelector(".gorp-x2many-editor[data-field='group_ids']");
        const dropdown = many2one?.querySelector(".gorp-many2one-dropdown");
        const options = [...(many2one?.querySelectorAll(".gorp-many2one-option") || [])];
        return {
          many2one_open: dropdown?.hidden === false,
          x2many_open: x2many?.querySelector(".gorp-x2many-dropdown")?.hidden === false,
          placement: many2one?.dataset.dropdownPlacement || "",
          dropdown_placement: dropdown?.dataset.placement || "",
          selected_count: options.filter((node) => node.dataset.selected === "true").length,
          active_count: options.filter((node) => node.dataset.active === "true").length,
          option_count: options.length,
          labels: options.map((node) => node.textContent.trim()).filter(Boolean)
        };
      })()`);
      if (!finalState.many2one_open || finalState.x2many_open || finalState.placement !== "bottom-start" || finalState.dropdown_placement !== "bottom-start" || finalState.option_count < 2 || finalState.selected_count < 1 || finalState.active_count < 1) {
        throw new Error(`relation dropdown final screenshot state invalid: ${JSON.stringify(finalState)}`);
      }
      return { opened, relation_dropdown_initial_state: initialState, relation_dropdown_state: state, relation_x2many_initial_state: x2ManyInitialState, relation_x2many_keyboard_state: x2ManyKeyboardState, relation_dropdown_final_state: finalState };
    }
  },
  {
    name: "default-relation-dialog-actions-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&relation_dialog_actions=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Relation dialog actions TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        const calls = [];
        const actions = [];
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Relation Dialog Actions",
            res_model: "x.relation.test",
            view_mode: "form",
            views: [[false, "form"]]
          },
          activeView: "form",
          resModel: "x.relation.test",
          viewDescriptions: {
            fields: {
              name: { type: "char", string: "Name" },
              partner_id: { type: "many2one", relation: "res.partner", string: "Partner" }
            },
            relatedModels: {},
            views: {
              form: {
                arch: "<form><sheet><field name='name'/><field name='partner_id' limit='1'/></sheet></form>",
                id: 790
              }
            }
          },
          records: [],
          length: 0,
          offset: 0,
          countLimited: false
        }, {
          values: { id: 26, name: "Relation Test", partner_id: false },
          context: { lang: "en_US" },
          services: {
            orm: {
              call(model, method, args, kwargs) {
                calls.push({ model, method, args, kwargs });
                if (method === "name_create") return Promise.resolve([99, "Created Record"]);
                if (method === "name_search" && kwargs.limit === 1) return Promise.resolve([[1, "Alpha"]]);
                return Promise.resolve([[1, "Alpha"], [2, "Beta"], [3, "Gamma"]]);
              }
            },
            action: {
              doAction(action, options) {
                actions.push({ action, additionalContext: options?.additionalContext || {} });
                return Promise.resolve({});
              }
            }
          }
        });
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "relation-dialog-actions";
        root.querySelector("[data-form-action='edit']")?.click();
        await new Promise((resolve) => setTimeout(resolve, 0));
        const editor = root.querySelector(".gorp-many2one-editor[data-field='partner_id']");
        const input = editor?.querySelector("input[data-field='partner_id']");
        const open = editor?.querySelector(".gorp-many2one-open");
        const initial = {
          editor_count: editor ? 1 : 0,
          open_count: open ? 1 : 0,
          open_hidden: open?.hidden ?? null,
          open_disabled: open?.disabled ?? null,
          open_class: open?.className || "",
          no_open: editor?.dataset.noOpen || ""
        };
        input.value = "omega";
        input.dispatchEvent(new Event("input", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        editor?.querySelector(".gorp-many2one-create-edit")?.click();
        await new Promise((resolve) => setTimeout(resolve, 0));
        input.value = "new acme";
        input.dispatchEvent(new Event("input", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        editor?.querySelector(".gorp-many2one-create")?.click();
        await new Promise((resolve) => setTimeout(resolve, 0));
        const afterCreate = {
          res_id: editor?.dataset.resId || "",
          input_value: input?.value || "",
          open_hidden: open?.hidden ?? null,
          open_disabled: open?.disabled ?? null,
          open_res_id: open?.dataset.resId || "",
          open_label: open?.getAttribute("aria-label") || "",
          open_tooltip: open?.getAttribute("data-tooltip") || ""
        };
        open?.click();
        await new Promise((resolve) => setTimeout(resolve, 0));
        return { initial, after_create: afterCreate, calls, actions };
      })()`);
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='relation-dialog-actions'] .gorp-form-view", 1, "relation dialog form");
      if (renderedState.initial.editor_count !== 1 || renderedState.initial.open_count !== 1 || renderedState.initial.open_hidden !== true || renderedState.initial.open_disabled !== true || !renderedState.initial.open_class.includes("o_external_button") || renderedState.initial.no_open) {
        throw new Error(`relation dialog initial state invalid: ${JSON.stringify(renderedState)}`);
      }
      const createEditAction = renderedState.actions?.[0]?.action;
      if (!createEditAction || createEditAction.name !== "Create Partner" || createEditAction.res_model !== "res.partner" || createEditAction.target !== "new" || createEditAction.context?.default_name !== "omega") {
        throw new Error(`relation create/edit action invalid: ${JSON.stringify(renderedState)}`);
      }
      const nameCreateCall = renderedState.calls?.filter((call) => call.method === "name_create").at(-1);
      if (!nameCreateCall || nameCreateCall.model !== "res.partner" || nameCreateCall.args?.[0] !== "new acme" || nameCreateCall.kwargs?.context?.default_name !== "new acme") {
        throw new Error(`relation quick-create call invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.after_create.res_id !== "99" || renderedState.after_create.input_value !== "Created Record" || renderedState.after_create.open_hidden !== false || renderedState.after_create.open_disabled !== false || renderedState.after_create.open_res_id !== "99" || renderedState.after_create.open_label !== "Open: Partner") {
        throw new Error(`relation open button after create invalid: ${JSON.stringify(renderedState)}`);
      }
      const openAction = renderedState.actions?.[1]?.action;
      if (!openAction || openAction.name !== "Open: Partner" || openAction.res_model !== "res.partner" || openAction.res_id !== 99 || openAction.target !== "new") {
        throw new Error(`relation open action invalid: ${JSON.stringify(renderedState)}`);
      }
      return { form_count: formCount, rendered_state: renderedState };
    }
  },
  {
    name: "default-scheduled-action-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&scheduled_action_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "scheduled action setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const fixture = await ensureScheduledActionSmokeRecord(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=ir.cron&view_type=form&id=${fixture.cronID}&menu_id=${fixture.menuID}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Scheduled Actions TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Scheduled Actions form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.cron'][data-view='form'] .gorp-form-view", 1, "TS Scheduled Actions form");
      const bandCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-scheduled-action-band[data-model='ir.cron'][data-state]", 1, "TS Scheduled Actions header band");
      const runButtonCount = await waitForCount(page, ".o_web_client .o_action_manager [data-scheduled-action-run='true']", 1, "TS Scheduled Actions run manually button");
      const notebookCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-scheduled-action-notebook .gorp-form-notebook-tab", 2, "TS Scheduled Actions Code Help notebook");
      const codeViewerCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-scheduled-action-notebook .gorp-code-viewer[data-field='code']", 1, "TS Scheduled Actions code viewer");
      const executeEveryCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-scheduled-execute-every[data-field='interval_number']", 1, "TS Scheduled Actions Execute Every field");
      const readState = await evaluate(page, `(() => {
        const labels = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-view .o_form_label")]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        return {
          title: document.querySelector(".o_web_client .o_action_manager .oe_title h1")?.textContent?.trim() || "",
          run_button: document.querySelector(".o_web_client .o_action_manager [data-scheduled-action-run='true']")?.textContent?.trim() || "",
          model: document.querySelector(".o_web_client .o_action_manager .gorp-form-field[data-field='model_id'] output")?.textContent?.trim() || "",
          nextcall: document.querySelector(".o_web_client .o_action_manager .gorp-form-field[data-field='nextcall'] output")?.textContent?.trim() || "",
          labels
        };
      })()`);
      if (readState.title !== "Base: Auto-vacuum internal data" || readState.run_button !== "Run Manually" || readState.model !== "Automatic Vacuum" || readState.nextcall !== "Jun 24, 1:02 AM" || !readState.labels.includes("Priority") || !readState.labels.includes("Allowed Groups") || !readState.labels.includes("Scheduler User") || !readState.labels.includes("Execute Every") || readState.labels.includes("Run As") || readState.labels.includes("Interval Unit") || readState.labels.includes("Server Action")) {
        throw new Error(`Scheduled Actions read layout invalid: ${JSON.stringify(readState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const executeEveryEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-scheduled-execute-every[data-field='interval_number'] select[data-field='interval_type']", 1, "TS Scheduled Actions Execute Every unit select");
      const codeEditorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-scheduled-action-notebook .gorp-code-editor[data-field='code']", 1, "TS Scheduled Actions code editor");
      const state = await evaluate(page, `(() => {
        const band = document.querySelector(".o_web_client .o_action_manager .gorp-scheduled-action-band");
        const badge = band?.querySelector(".gorp-server-action-badge")?.textContent?.trim() || "";
        const status = band?.querySelector(".gorp-server-action-state")?.textContent?.trim() || "";
        const interval = document.querySelector(".o_web_client .o_action_manager .gorp-scheduled-execute-every[data-field='interval_number']");
        const number = interval?.querySelector("input[data-field='interval_number']")?.value || "";
        const unit = interval?.querySelector("select[data-field='interval_type']")?.value || "";
        const legacyRadioCount = document.querySelectorAll(".o_web_client .o_action_manager .gorp-selection-radio-group[data-field='interval_type'] input[type='radio']").length;
        return {
          badge,
          status,
          state: band?.dataset?.state || "",
          interval_number: number,
          interval_unit: unit,
          legacy_radio_count: legacyRadioCount
        };
      })()`);
      if (state.badge !== "Scheduled Action" || state.interval_number !== "1" || state.interval_unit !== "days" || state.legacy_radio_count !== 0) {
        throw new Error(`Scheduled Actions chrome invalid: ${JSON.stringify(state)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='discard']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-view .gorp-scheduled-execute-every[data-field='interval_number'].o_readonly_modifier", 1, "TS Scheduled Actions returns to readonly layout");
      return { fixture, form_count: formCount, band_count: bandCount, run_button_count: runButtonCount, notebook_count: notebookCount, code_viewer_count: codeViewerCount, execute_every_count: executeEveryCount, read_state: readState, execute_every_editor_count: executeEveryEditorCount, code_editor_count: codeEditorCount, state };
    }
  },
  {
    name: "default-scheduled-actions-list-parity-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&scheduled_actions_list_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "scheduled actions list setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const ids = await externalResIDs(page, config, ["base.action_ir_cron", "base.menu_ir_cron"]);
      const actionID = Number(ids["base.action_ir_cron"] || 0);
      const menuID = Number(ids["base.menu_ir_cron"] || 0);
      if (!actionID || !menuID) throw new Error(`Scheduled Actions menu/action missing: ${JSON.stringify(ids)}`);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${actionID}&model=ir.cron&view_type=list&menu_id=${menuID}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Scheduled Actions list TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Scheduled Actions list ready");
      const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.cron'][data-view='list'] .gorp-list-view tbody tr.o_data_row", 2, "Scheduled Actions reference rows");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.cron'][data-view='list']");
        const headers = [...(root?.querySelectorAll(".gorp-list-view thead .o_list_header_button") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const rows = [...(root?.querySelectorAll(".gorp-list-view tbody tr.o_data_row") || [])].map((row) => [...row.querySelectorAll("td")].map((cell) => cell.textContent.trim()).filter(Boolean));
        const chips = [...document.querySelectorAll(".o_web_client .o_action_manager .o_searchview_facet .o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean);
        const activeWidgets = [...(root?.querySelectorAll(".gorp-list-view tbody td[data-field='active'] .gorp-readonly-boolean") || [])]
          .map((node) => node.getAttribute("aria-checked") || "");
        const rawActiveText = [...(root?.querySelectorAll(".gorp-list-view tbody td[data-field='active']") || [])]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        const pager = document.querySelector(".o_web_client .o_action_manager .o_cp_pager")?.textContent?.trim() || "";
        return { headers, rows, pager, chips, activeWidgets, rawActiveText };
      })()`);
      const expectedHeaders = ["Priority", "Action Name", "Model", "Next Execution Date", "Interval Number", "Interval Unit", "Active"];
      if (rowCount !== 2 || JSON.stringify(state.headers) !== JSON.stringify(expectedHeaders) || !state.pager.startsWith("1-2 / 2") || !state.chips.includes("All") || state.rows[0]?.[1] !== "Base: Auto-vacuum internal data" || state.rows[0]?.[2] !== "Automatic Vacuum" || state.rows[1]?.[1] !== "Base: Portal Users Deletion" || state.rows[1]?.[2] !== "Users Deletion Request" || JSON.stringify(state.activeWidgets) !== JSON.stringify(["true", "true"]) || state.rawActiveText.length) {
        throw new Error(`Scheduled Actions list parity invalid: ${JSON.stringify(state)}`);
      }
      return { row_count: rowCount, state };
    }
  },
  {
    name: "default-automation-form-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&automation_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "automation setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const fixture = await createAutomationSmokeRecord(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=base.automation&view_type=form&id=${fixture.automationID}&menu_id=${fixture.menuID}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Automation Rules TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Automation Rules form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='base.automation'][data-view='form'] .gorp-form-view", 1, "TS Automation Rules form");
      const bandCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-automation-action-band[data-model='base.automation'][data-trigger]", 1, "TS Automation Rules header band");
      const triggerPillCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-selection-pills[data-field='trigger'] .gorp-selection-pill", 1, "TS Automation trigger pills");
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const triggerRadioCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-selection-radio-group[data-field='trigger'] input[type='radio']", 1, "TS Automation trigger radio");
      const state = await evaluate(page, `(() => {
        const band = document.querySelector(".o_web_client .o_action_manager .gorp-automation-action-band");
        const badge = band?.querySelector(".gorp-server-action-badge")?.textContent?.trim() || "";
        const trigger = band?.querySelector(".gorp-server-action-state")?.textContent?.trim() || "";
        const radio = document.querySelector(".o_web_client .o_action_manager .gorp-selection-radio-group[data-field='trigger']");
        const labels = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-selection-radio-group[data-field='trigger'] .gorp-selection-radio-pill")]
          .map((node) => node.textContent.trim())
          .filter(Boolean);
        return {
          badge,
          trigger,
          trigger_value: band?.dataset?.trigger || "",
          radio_value: radio?.dataset?.value || "",
          trigger_labels: labels
        };
      })()`);
      if (state.badge !== "Automation Rule" || state.trigger !== "On Creation & Update" || !state.trigger_labels.includes("Based on Timed Condition")) {
        throw new Error(`Automation Rules chrome invalid: ${JSON.stringify(state)}`);
      }
      return { fixture, form_count: formCount, band_count: bandCount, trigger_pill_count: triggerPillCount, trigger_radio_count: triggerRadioCount, state };
    }
  },
  {
    name: "default-groups-form-notebook-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Groups notebook TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", "Groups");
      const actionCardCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", 1, "TS Groups action card");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Groups", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS Groups list action ready");
      const listCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list']", 1, "TS Groups list");
      const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view tbody tr.o_data_row", 1, "TS Groups rows");
      const listState = await evaluate(page, `(() => {
        const list = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view");
        return {
          headers: [...(list?.querySelectorAll(".o_list_header_button") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          text: list?.textContent || ""
        };
      })()`);
      if (JSON.stringify(listState.headers) !== JSON.stringify(["Privilege", "Name"])) throw new Error(`Groups list invalid: ${JSON.stringify(listState)}`);
      const roleAdminRow = await evaluate(page, `(() => {
        const rows = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view tbody tr.o_data_row")];
        const row = rows.find((node) => node.textContent.includes("Role / Administrator"));
        if (!row) return "";
        row.dataset.smokeTarget = "role-admin-group";
        return row.textContent.trim();
      })()`);
      if (!roleAdminRow) throw new Error(`Role / Administrator group row missing: ${JSON.stringify(listState)}`);
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list'] .gorp-list-view tbody tr.o_data_row[data-smoke-target='role-admin-group']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS Groups form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-form-view", 1, "TS Groups form");
      const notebookCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-notebook.o_notebook", 1, "TS Groups form notebook");
      const tabCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-form-notebook-tab[role='tab']", 7, "TS Groups form notebook tabs");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-form-notebook.o_notebook");
        const actionManager = document.querySelector(".o_web_client .o_action_manager");
        const form = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-form-view");
        const sheet = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-form-sheet.o_form_sheet");
        const usersGrid = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-groups-users-list[data-field='user_ids']");
        const pager = document.querySelector(".o_web_client .o_control_panel_navigation .o_pager");
	        const tabs = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-notebook-tab[role='tab']")];
	        const pages = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-form-notebook-page[role='tabpanel']")];
	        const smartButtons = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-access-smart-button")].map((node) => node.textContent.trim()).filter(Boolean);
        const labels = [...(form?.querySelectorAll(".o_form_label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const apiKeyDurationField = form?.querySelector("input[data-field='api_key_duration'], output[data-field='api_key_duration'], .gorp-field-value[data-field='api_key_duration']");
        const apiKeyDuration = apiKeyDurationField?.value || apiKeyDurationField?.textContent?.trim() || "";
        const readonlyControls = [...(form?.querySelectorAll("input.gorp-form-control.o_readonly_modifier[data-field='name'], input.gorp-form-control.o_readonly_modifier[data-field='privilege_id'], input.gorp-form-control.o_readonly_modifier[data-field='api_key_duration']") || [])].map((node) => {
          const style = getComputedStyle(node);
          const rect = node.getBoundingClientRect();
          return {
            field: node.dataset.field || "",
            value: node.value || "",
            bg: style.backgroundColor,
            color: style.color,
            width: Math.round(rect.width || 0)
          };
        });
	        return {
          notebook_id: root?.dataset?.notebook || "",
          tab_labels: tabs.map((node) => node.textContent.trim()),
          selected_tabs: tabs.map((node) => node.getAttribute("aria-selected")),
          visible_pages: pages.filter((node) => !node.hidden).length,
          form_text: form?.textContent || "",
          smart_buttons: smartButtons,
          action_width: Math.round(actionManager?.getBoundingClientRect?.().width || 0),
          sheet_width: Math.round(sheet?.getBoundingClientRect?.().width || 0),
          users_grid_count: usersGrid ? 1 : 0,
          users_headers: [...(usersGrid?.querySelectorAll("th") || [])].map((node) => node.textContent.trim()).filter(Boolean),
	          users_rows: [...(usersGrid?.querySelectorAll("tbody tr") || [])].map((row) => [...row.querySelectorAll("td")].map((cell) => cell.textContent.trim()).filter(Boolean)).filter((row) => row.length),
	          users_add_line: usersGrid?.querySelector(".gorp-groups-users-add")?.textContent?.trim() || "",
	          users_blank_rows: usersGrid?.querySelectorAll(".gorp-groups-users-blank-row")?.length || 0,
	          labels,
	          api_key_duration: apiKeyDuration,
	          readonly_controls: readonlyControls,
	          pager_value: pager?.querySelector(".o_pager_value")?.textContent?.trim() || "",
	          pager_limit: pager?.querySelector(".o_pager_limit")?.textContent?.trim() || "",
	          pager_previous_disabled: pager?.querySelector(".o_pager_previous")?.disabled === true,
	          pager_next_disabled: pager?.querySelector(".o_pager_next")?.disabled === true,
	          action_menu_count: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='form'] .gorp-form-action-menu, .o_web_client .o_control_panel .gorp-form-action-menu[data-model='res.groups']").length
	        };
	      })()`);
      for (const tab of ["Users", "Inherited", "Menus", "Views", "Access Rights", "Record Rules", "Notes"]) {
        if (!state.tab_labels.includes(tab)) throw new Error(`Groups notebook tab missing ${tab}: ${JSON.stringify(state)}`);
      }
      if (state.tab_labels.includes("Inherited By")) throw new Error(`Groups notebook still has fallback tab: ${JSON.stringify(state)}`);
      if (state.visible_pages !== 1) throw new Error(`Groups notebook visible page mismatch: ${JSON.stringify(state)}`);
      if (!state.smart_buttons.some((item) => item.includes("Users"))) throw new Error(`Groups smart button missing: ${JSON.stringify(state)}`);
      if (state.sheet_width < Math.min(1180, Math.floor(state.action_width * 0.92))) throw new Error(`Groups sheet width too narrow: ${JSON.stringify(state)}`);
      if (state.users_grid_count !== 1) throw new Error(`Groups Users grid missing: ${JSON.stringify(state)}`);
      if (JSON.stringify(state.users_headers) !== JSON.stringify(["Name", "Login", "Role"])) throw new Error(`Groups Users grid headers invalid: ${JSON.stringify(state)}`);
      if (!state.form_text.includes("Role / Administrator")) throw new Error(`Groups form title mismatch: ${JSON.stringify(state)}`);
	      if (!state.users_rows.some((row) => row[0] === "Administrator" && row[1] === "admin")) throw new Error(`Groups Users grid populated Administrator row missing: ${JSON.stringify(state)}`);
	      if (state.users_add_line !== "Add a line") throw new Error(`Groups Users Add a line missing: ${JSON.stringify(state)}`);
	      if (state.users_blank_rows < 4) throw new Error(`Groups Users blank rows missing: ${JSON.stringify(state)}`);
	      if (!state.labels.includes("Group Name") || state.api_key_duration !== "0.00" || state.action_menu_count < 1 || state.pager_limit !== "13" || !state.pager_value || state.pager_previous_disabled || state.pager_next_disabled) throw new Error(`Groups form chrome invalid: ${JSON.stringify(state)}`);
	      if (state.readonly_controls.length < 3 || state.readonly_controls.some((control) => control.bg !== "rgb(255, 255, 255)" || control.color !== "rgb(31, 41, 51)" || control.width < 160)) throw new Error(`Groups form readonly controls invalid: ${JSON.stringify(state)}`);
	      return { action_card_count: actionCardCount, list_count: listCount, row_count: rowCount, list_state: listState, form_count: formCount, notebook_count: notebookCount, tab_count: tabCount, ...state };
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
    name: "default-mobile-delegation-one2many",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await setViewport(page, mobileViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&delegation_one2many_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "mobile delegation one2many setup document ready");
      await createDelegationAdminSession(page, config);
      const fixture = await createDelegationOne2ManySmokeRecord(page, config);
      const menuParam = fixture.menuID ? `&menu_id=${fixture.menuID}` : "";
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=delegation&view_type=form&id=${fixture.delegationID}${menuParam}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "mobile delegation one2many TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "mobile delegation one2many form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='delegation'][data-view='form'] .gorp-form-view", 1, "mobile TS Delegation form");
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='edit']");
      const editorCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines'][data-relation='delegation.line']", 1, "mobile TS Delegation editable one2many");
      const state = await evaluate(page, `(() => {
        const editor = document.querySelector(".o_web_client .o_action_manager .gorp-one2many-editor[data-field='lines']");
        const table = editor?.querySelector(".gorp-one2many-table");
        const thead = table?.querySelector("thead");
        const rows = [...(editor?.querySelectorAll("tbody tr.o_data_row") || [])];
        const firstRow = rows[0];
        const cells = [...(firstRow?.querySelectorAll("td[data-field]") || [])];
        const actionCell = firstRow?.querySelector("td.gorp-one2many-actions");
        const tableStyle = table ? getComputedStyle(table) : null;
        const theadStyle = thead ? getComputedStyle(thead) : null;
        const firstRowStyle = firstRow ? getComputedStyle(firstRow) : null;
        const editorRect = editor?.getBoundingClientRect();
        const rowRect = firstRow?.getBoundingClientRect();
        return {
          field: editor?.dataset?.field || "",
          relation: editor?.dataset?.relation || "",
          field_type: editor?.dataset?.fieldType || "",
          mobile_widget: editor?.dataset?.mobileWidget || "",
          mobile_layout: editor?.dataset?.mobileLayout || "",
          table_mobile_layout: table?.dataset?.mobileLayout || "",
          table_display: tableStyle?.display || "",
          thead_display: theadStyle?.display || "",
          row_display: firstRowStyle?.display || "",
          row_count: rows.length,
          labels: cells.map((node) => node.dataset.label || "").filter(Boolean),
          action_label: actionCell?.dataset?.label || "",
          input_count: editor?.querySelectorAll(".gorp-one2many-input, .gorp-one2many-readonly").length || 0,
          editor_width_px: editorRect ? Math.round(editorRect.width) : 0,
          row_width_px: rowRect ? Math.round(rowRect.width) : 0,
          viewport_width_px: window.innerWidth,
          overflow_px: document.documentElement.scrollWidth - window.innerWidth
        };
      })()`);
      if (state.field !== "lines" || state.relation !== "delegation.line" || state.field_type !== "one2many") {
        throw new Error(`mobile delegation one2many identity invalid: ${JSON.stringify(state)}`);
      }
      if (state.mobile_widget !== "one2many_list" || state.mobile_layout !== "cards" || state.table_mobile_layout !== "cards") {
        throw new Error(`mobile delegation one2many metadata invalid: ${JSON.stringify(state)}`);
      }
      if (state.thead_display !== "none" || state.table_display !== "block" || state.row_display !== "block") {
        throw new Error(`mobile delegation one2many layout invalid: ${JSON.stringify(state)}`);
      }
      if (state.row_count < 1 || state.input_count < 1 || !state.labels.includes("Group") || state.overflow_px > 1 || state.row_width_px > state.viewport_width_px) {
        throw new Error(`mobile delegation one2many content invalid: ${JSON.stringify(state)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager [data-form-action='discard']");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-x2many-tags[data-field='lines']", 1, "mobile TS Delegation readonly one2many after discard");
      return { form_count: formCount, editor_count: editorCount, fixture, state };
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
    name: "default-date-groupby-menu-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&date_groupby_setup=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "date groupby TS webclient ready");
      const fixture = await createDateGroupBySmokeAction(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=mail.message&view_type=list`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "date groupby action ready");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='mail.message'][data-view='list']", 1, "date groupby mail message list");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const optionState = await waitFor(page, `(() => {
        const parent = document.querySelector(".o_web_client .o_action_manager .o_group_by_menu [data-menu-item-id='group-by-date']");
        const options = [...document.querySelectorAll(".o_web_client .o_action_manager .o_group_by_menu [data-parent-menu-item-id='group-by-date']")];
        const labels = options.map((node) => node.textContent.trim()).filter(Boolean);
        return parent && labels.length === 5 ? { parent: parent.textContent.trim(), labels } : null;
      })()`, "date groupby interval options");
      const expectedLabels = ["Year", "Quarter", "Month", "Week", "Day"];
      if (JSON.stringify(optionState.labels) !== JSON.stringify(expectedLabels)) {
        throw new Error(`date groupby intervals invalid: ${JSON.stringify(optionState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager .o_group_by_menu [data-menu-item-id='group-by-date-year']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "date groupby year action ready");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const selectedState = await waitFor(page, `(() => {
        const facet = document.querySelector(".o_web_client .o_action_manager .o_searchview_facet[data-facet-id='group-by-date-year']");
        const selected = document.querySelector(".o_web_client .o_action_manager .o_group_by_menu [data-menu-item-id='group-by-date-year'].selected");
        return facet && selected ? {
          facet_label: facet.querySelector(".o_searchview_facet_label")?.textContent?.trim() || "",
          facet_values: [...facet.querySelectorAll(".o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean),
          selected_checked: selected.getAttribute("aria-checked") || ""
        } : null;
      })()`, "date groupby year facet selected");
      if (selectedState.facet_label !== "Date" || selectedState.facet_values[0] !== "Year" || selectedState.selected_checked !== "true") {
        throw new Error(`date groupby selected state invalid: ${JSON.stringify(selectedState)}`);
      }
      return { fixture, option_state: optionState, selected_state: selectedState };
    }
  },
  {
    name: "default-date-filter-period-menu-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&date_filter_setup=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "date filter TS webclient ready");
      const fixture = await createDateFilterSmokeAction(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=mail.message&view_type=list`) });
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "date filter action ready");
      await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='mail.message'][data-view='list']", 1, "date filter mail message list");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const optionState = await waitFor(page, `(() => {
        const parents = [...document.querySelectorAll(".o_web_client .o_action_manager .o_filter_menu [data-menu-item-id]")];
        const parent = parents.find((node) => {
          const id = node.dataset.menuItemId || "";
          if (!id.startsWith("filter-")) return false;
          return document.querySelectorAll(\`.o_web_client .o_action_manager .o_filter_menu [data-parent-menu-item-id="\${id}"]\`).length >= 10;
        });
        const parentID = parent?.dataset.menuItemId || "";
        const options = parentID ? [...document.querySelectorAll(\`.o_web_client .o_action_manager .o_filter_menu [data-parent-menu-item-id="\${parentID}"]\`)] : [];
        const labels = options.map((node) => node.textContent.trim()).filter(Boolean);
        return parent && labels.length >= 10 ? { parent_id: parentID, parent: parent.textContent.trim(), labels: labels.slice(0, 10) } : null;
      })()`, "date filter period options");
      const currentMonth = new Date().toLocaleString("en-US", { month: "long" });
      const currentYear = String(new Date().getFullYear());
      if (optionState.labels[0] !== currentMonth || !optionState.labels.includes(currentYear) || !optionState.labels.includes("Q1")) {
        throw new Error(`date filter options invalid: ${JSON.stringify(optionState)}`);
      }
      await clickSelector(page, `.o_web_client .o_action_manager .o_filter_menu [data-menu-item-id='${optionState.parent_id}-month']`);
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "date filter current month action ready");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const selectedState = await waitFor(page, `(() => {
        const selected = [...document.querySelectorAll(${JSON.stringify(`.o_web_client .o_action_manager .o_filter_menu [data-parent-menu-item-id='${optionState.parent_id}'].selected`)})]
          .map((node) => ({ id: node.dataset.menuItemId || "", checked: node.getAttribute("aria-checked") || "", label: node.textContent.trim() }));
        const facets = [...document.querySelectorAll(".o_web_client .o_action_manager .o_searchview_facet_dateFilter")]
          .map((facet) => ({
            id: facet.dataset.facetId || "",
            label: facet.querySelector(".o_searchview_facet_label")?.textContent?.trim() || "",
            values: [...facet.querySelectorAll(".o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean)
          }));
        const rows = document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row").length;
        return selected.length >= 2 && facets.length >= 2 && rows >= 1 ? { selected, facets, rows } : null;
      })()`, "date filter selected current month and year");
      const selectedIDs = selectedState.selected.map((item) => item.id);
      if (!selectedIDs.includes(`${optionState.parent_id}-month`) || !selectedIDs.includes(`${optionState.parent_id}-year`)) {
        throw new Error(`date filter selected options invalid: ${JSON.stringify(selectedState)}`);
      }
      return { fixture, option_state: optionState, selected_state: selectedState };
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
    name: "default-custom-filter-dialog-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      const opened = await openDefaultServerActionsList(page, config, desktopViewport());
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      await clickSelector(page, ".o_web_client .o_action_manager .o_add_custom_filter");
      const dialogState = await waitFor(page, `(() => {
        const dialog = document.querySelector(".o_web_client .o_action_manager .gorp-custom-filter-dialog.o_dialog");
        const field = dialog?.querySelector("[data-custom-filter-field='true']");
        const operator = dialog?.querySelector("[data-custom-filter-operator='true']");
        const value = dialog?.querySelector("[data-custom-filter-value='true']");
        const labels = field ? [...field.querySelectorAll("option")].map((option) => option.textContent.trim()).filter(Boolean) : [];
        return dialog && field && operator && value ? { field: field.value, operator: operator.value, labels } : null;
      })()`, "custom filter dialog controls");
      if (dialogState.field !== "model_name" || dialogState.operator !== "ilike" || !dialogState.labels.includes("Model")) {
        throw new Error(`custom filter dialog invalid: ${JSON.stringify(dialogState)}`);
      }
      await setInput(page, ".o_web_client .o_action_manager [data-custom-filter-value='true']", "mail");
      await clickSelector(page, ".o_web_client .o_action_manager [data-custom-filter-apply='true']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "custom filter applied action ready");
      const appliedState = await waitFor(page, `(() => {
        const rows = [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row")];
        const facet = document.querySelector(".o_web_client .o_action_manager .o_searchview_facet[data-facet-id^='custom-model_name-ilike-mail']");
        if (!facet || !rows.length) return null;
        const label = facet.querySelector(".o_searchview_facet_label")?.textContent?.trim() || "";
        const values = [...facet.querySelectorAll(".o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean);
        const text = rows.map((row) => row.textContent.trim()).join("\\n").toLowerCase();
        return { rows: rows.length, label, values, text_has_mail: text.includes("mail"), raw_field_text: facet.textContent.includes("model_name") };
      })()`, "custom filter facet and filtered rows");
      if (appliedState.label !== "Model" || appliedState.values[0] !== "mail" || appliedState.raw_field_text || !appliedState.text_has_mail || appliedState.rows >= opened.row_count) {
        throw new Error(`custom filter applied state invalid: ${JSON.stringify({ opened, appliedState })}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_facet[data-facet-id^='custom-model_name-ilike-mail'] .o_facet_remove");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "custom filter removed action ready");
      const restoredRows = await waitForCount(page, ".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row", opened.row_count, "custom filter restored rows");
      return { baseline_rows: opened.row_count, filtered_state: appliedState, restored_rows: restoredRows };
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
    name: "default-kanban-view-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "kanban setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const fixture = await createKanbanSmokeAction(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=ir.actions.server&view_type=kanban`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban action ready");
      const kanbanCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='kanban'] .o_kanban_renderer", 1, "TS Server Actions kanban");
      const cardCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='kanban'] .o_kanban_record", 1, "TS Server Actions kanban cards");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='kanban']");
        const card = root?.querySelector(".o_kanban_record");
        const title = card?.querySelector(".o_kanban_record_title")?.textContent?.trim() || "";
        const fields = [...(card?.querySelectorAll(".o_kanban_record_field") || [])].map((node) => ({
          field: node.dataset.field || "",
          text: node.textContent.trim()
        }));
        const recordMenuToggle = card?.querySelector(".o_kanban_record_menu_toggle[data-kanban-record-menu='true']");
        const recordMenu = card?.querySelector(".o_kanban_record_menu_dropdown");
        const recordMenuItems = [...(recordMenu?.querySelectorAll("[role='menuitem']") || [])].map((node) => ({
          action: node.dataset.kanbanRecordMenuAction || "",
          label: node.textContent.trim(),
          disabled: Boolean(node.disabled)
        }));
        const switches = [...document.querySelectorAll(".o_web_client .o_action_manager .o_switch_view")]
          .map((node) => ({ view: node.dataset.viewType || "", active: node.classList.contains("active"), label: node.getAttribute("aria-label") || node.textContent.trim() }));
        return {
          role: card?.getAttribute("role") || "",
          tabindex: card?.getAttribute("tabindex") || "",
          title,
          fields,
          record_menu: {
            toggle_count: card?.querySelectorAll(".o_kanban_record_menu_toggle[data-kanban-record-menu='true']").length || 0,
            expanded: recordMenuToggle?.getAttribute("aria-expanded") || "",
            hidden: recordMenu?.hasAttribute("hidden") ?? true,
            items: recordMenuItems
          },
          switches,
          hash: window.location.hash || ""
        };
      })()`);
      if (state.role !== "link" || state.tabindex !== "0" || !state.title) {
        throw new Error(`kanban card chrome invalid: ${JSON.stringify(state)}`);
      }
      if (state.fields.some((item) => item.field === "state" && item.text.trim() === "code")) {
        throw new Error(`kanban state shows raw value: ${JSON.stringify(state)}`);
      }
      if (!state.switches.some((item) => item.view === "kanban" && item.active)) {
        throw new Error(`kanban switch not active: ${JSON.stringify(state)}`);
      }
      if (state.record_menu.toggle_count !== 1 || state.record_menu.expanded !== "false" || state.record_menu.hidden !== true) {
        throw new Error(`kanban record menu invalid: ${JSON.stringify(state)}`);
      }
      for (const expected of [["open", "Open"], ["duplicate", "Duplicate"], ["delete", "Delete"]]) {
        if (!state.record_menu.items.some((item) => item.action === expected[0] && item.label === expected[1] && item.disabled === false)) {
          throw new Error(`kanban record menu action missing: ${JSON.stringify({ expected, state })}`);
        }
      }
      if (state.record_menu.items.length !== 3) {
        throw new Error(`kanban record menu invalid: ${JSON.stringify(state)}`);
      }
      await clickFirst(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='kanban'] .o_kanban_record .o_kanban_record_menu_toggle");
      const menuState = await waitFor(page, `(() => {
        const card = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='kanban'] .o_kanban_record");
        const toggle = card?.querySelector(".o_kanban_record_menu_toggle[data-kanban-record-menu='true']");
        const menu = card?.querySelector(".o_kanban_record_menu_dropdown");
        return toggle?.getAttribute("aria-expanded") === "true" && menu?.classList.contains("show") && !menu?.hasAttribute("hidden")
          ? { expanded: toggle.getAttribute("aria-expanded"), item_count: menu.querySelectorAll("[role='menuitem']").length }
          : null;
      })()`, "TS kanban record menu opens");
      await clickFirst(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='kanban'] .o_kanban_record .o_kanban_record_menu_item[data-kanban-record-menu-action='open']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban record form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "TS Kanban opened form");
      const formHash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=form") && hash.includes("id=") ? hash : "";
      })()`, "TS kanban opened form hash");
      return { fixture, kanban_count: kanbanCount, card_count: cardCount, state, menu_state: menuState, form_count: formCount, form_hash: formHash };
    }
  },
  {
    name: "default-kanban-load-more-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_load_more_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "kanban load-more setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      await ensureKanbanSmokeRecordCount(page, config, 3);
      const fixture = await createKanbanSmokeAction(page, config, { limit: 1 });
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=ir.actions.server&view_type=kanban`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban load-more TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban load-more action ready");
      const initialState = await waitFor(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='kanban']");
        const cards = [...(root?.querySelectorAll(".o_kanban_record") || [])];
        const button = root?.querySelector(".o_kanban_load_more[data-kanban-load-more='true']");
        return root && cards.length === 1 && button ? {
          card_count: cards.length,
          loaded: button.dataset.loaded || "",
          total: button.dataset.total || "",
          next_limit: button.dataset.nextLimit || "",
          label: button.textContent.trim(),
          hash: window.location.hash || ""
        } : null;
      })()`, "Kanban load-more initial button");
      if (initialState.loaded !== "1" || initialState.next_limit !== "2" || initialState.label !== "Load more") {
        throw new Error(`kanban load-more initial invalid: ${JSON.stringify(initialState)}`);
      }
      await clickFirst(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='kanban'] .o_kanban_load_more[data-kanban-load-more='true']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban load-more rerender ready");
      const loadedState = await waitFor(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='kanban']");
        const cards = [...(root?.querySelectorAll(".o_kanban_record") || [])];
        const button = root?.querySelector(".o_kanban_load_more[data-kanban-load-more='true']");
        return root && cards.length >= 2 ? {
          card_count: cards.length,
          load_more_visible: Boolean(button),
          loaded: button?.dataset.loaded || "",
          next_limit: button?.dataset.nextLimit || "",
          hash: window.location.hash || ""
        } : null;
      })()`, "Kanban load-more increases cards");
      if (loadedState.card_count <= initialState.card_count) {
        throw new Error(`kanban load-more did not increase cards: ${JSON.stringify({ initialState, loadedState })}`);
      }
      if (!loadedState.hash.includes("view_type=kanban")) {
        throw new Error(`kanban load-more lost route state: ${JSON.stringify(loadedState)}`);
      }
      return { fixture, initial_state: initialState, loaded_state: loadedState };
    }
  },
  {
    name: "default-kanban-progressbar-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_progressbar=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban progressbar TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Kanban Progress",
            res_model: "res.partner",
            view_mode: "kanban",
            views: [[false, "kanban"]]
          },
          activeView: "kanban",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              display_name: { type: "char", string: "Name" },
              state: { type: "selection", string: "State", selection: [["code", "Execute Code"], ["multi", "Multi Actions"], ["webhook", "Webhook"]] },
              amount: { type: "float", string: "Amount" },
              color: { type: "integer", string: "Color" }
            },
            relatedModels: {},
            views: {
              kanban: {
                arch: "<kanban><progressbar field='state' colors=\\\"{'code':'success','multi':'warning','webhook':'danger'}\\\" sum_field='amount'/><field name='display_name'/><field name='state'/><field name='amount'/><field name='color'/></kanban>",
                id: 700
              }
            }
          },
          records: [
            { id: 701, display_name: "Progress A", state: "code", amount: 10, color: 1 },
            { id: 702, display_name: "Progress B", state: "multi", amount: 5, color: 2 },
            { id: 703, display_name: "Progress C", state: "webhook", amount: 5, color: 3 },
            { id: 704, display_name: "Progress D", state: "code", amount: 10, color: 1 }
          ],
          length: 4,
          offset: 0,
          countLimited: false
        });
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "kanban-progressbar";
        const progress = root.querySelector(".o_kanban_progressbar");
        const segments = [...(progress?.querySelectorAll(".o_kanban_progressbar_segment") || [])].map((node) => ({
          value: node.dataset.value || "",
          label: node.dataset.label || "",
          count: node.dataset.count || "",
          sum: node.dataset.sum || "",
          width: node.style.width,
          className: node.className
        }));
        const cards = [...root.querySelectorAll(".o_kanban_record")].map((node) => ({
          id: node.dataset.id || "",
          color: node.dataset.kanbanColor || "",
          className: node.className
        }));
        return {
          progress_field: progress?.dataset.field || "",
          progress_sum_field: progress?.dataset.sumField || "",
          segment_count: segments.length,
          segments,
          cards
        };
      })()`);
      const progressCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-progressbar'] .o_kanban_progressbar", 1, "Kanban progressbar");
      const cardCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-progressbar'] .o_kanban_record[data-kanban-color]", 4, "Kanban colored cards");
      if (renderedState.progress_field !== "state" || renderedState.progress_sum_field !== "amount" || renderedState.segment_count !== 3) {
        throw new Error(`kanban progressbar metadata invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.segments.some((item) => item.value === "code" && item.label === "Execute Code" && item.sum === "20" && item.width === "66.67%")) {
        throw new Error(`kanban progressbar code segment invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.segments.some((item) => item.value === "multi" && item.className.includes("o_kanban_progress_color_warning"))) {
        throw new Error(`kanban progressbar color invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.every((card) => card.color && card.className.includes("o_kanban_color_" + card.color))) {
        throw new Error(`kanban card color invalid: ${JSON.stringify(renderedState)}`);
      }
      return { rendered_state: renderedState, progress_count: progressCount, card_count: cardCount };
    }
  },
  {
    name: "default-action-menus-keyboard-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&action_menus_keyboard=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "ActionMenus keyboard TS webclient ready");
      const keyboardState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        window.__actionMenuKeyboardCalls = [];
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Keyboard Actions",
            res_model: "res.partner",
            view_mode: "list,form",
            views: [[false, "list"], [false, "form"]]
          },
          activeView: "list",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              name: { type: "char", string: "Name" },
              active: { type: "boolean", string: "Active" }
            },
            relatedModels: {},
            views: {
              list: {
                arch: "<list><field name='name'/><field name='active'/></list>",
                id: 740,
                actionMenus: {
                  print: [{ id: 741, name: "Print Keyboard", description: "Print Keyboard", sequence: 2, groupNumber: 1 }],
                  action: [{ id: 742, name: "Run Keyboard", sequence: 5, groupNumber: 2 }]
                }
              },
              form: { arch: "<form><field name='name'/></form>", id: 743 }
            }
          },
          records: [{ id: 744, name: "Keyboard Partner", active: true }],
          length: 1,
          offset: 0,
          countLimited: false
        }, {
          services: {
            action: {
              doAction(action, options) {
                window.__actionMenuKeyboardCalls.push({ action, additionalContext: options?.additionalContext || {} });
                return Promise.resolve(true);
              }
            }
          }
        });
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "action-menus-keyboard";
        const checkbox = root.querySelector("input[type='checkbox'][data-record-id='744']");
        checkbox.checked = true;
        checkbox.dispatchEvent(new Event("change", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        const toolbar = root.querySelector(".gorp-action-menus");
        const actionSection = toolbar?.querySelector(".gorp-action-menu-section[data-menu='action']");
        const printSection = toolbar?.querySelector(".gorp-action-menu-section[data-menu='print']");
        const actionToggle = actionSection?.querySelector(".gorp-action-menu-toggle");
        const printToggle = printSection?.querySelector(".gorp-action-menu-toggle");
        const actionMenu = actionSection?.querySelector(".gorp-action-menu-items");
        const printMenu = printSection?.querySelector(".gorp-action-menu-items");
        actionToggle.focus();
        actionToggle.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        const arrow_focus_menuitem = document.activeElement?.classList?.contains("gorp-action-menu-item") === true;
        actionMenu.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true }));
        const end_focus = document.activeElement?.dataset?.actionId || "";
        actionMenu.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
        const action_escape_closed = actionSection?.dataset.open === "false" && actionToggle.getAttribute("aria-expanded") === "false";
        const action_escape_restored = document.activeElement === actionToggle;
        const before_print_hotkey_focus = document.activeElement?.dataset?.actionMenuToggle || "";
        toolbar.dispatchEvent(new KeyboardEvent("keydown", { key: "U", shiftKey: true, bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        const shift_u_focus = document.activeElement?.dataset?.actionId || "";
        const shift_u_open = printSection?.dataset.open === "true" && printToggle.getAttribute("aria-expanded") === "true";
        printMenu.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
        const print_escape_restored_to = document.activeElement?.dataset?.actionMenuToggle || "";
        toolbar.dispatchEvent(new KeyboardEvent("keydown", { key: "u", bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        const u_initial_focus_menuitem = document.activeElement?.classList?.contains("gorp-action-menu-item") === true;
        actionMenu.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true }));
        const u_focus = document.activeElement?.dataset?.actionId || "";
        actionMenu.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
        return {
          arrow_focus_menuitem,
          end_focus,
          action_escape_closed,
          action_escape_restored,
          before_print_hotkey_focus,
          shift_u_focus,
          shift_u_open,
          print_escape_restored_to,
          u_initial_focus_menuitem,
          u_focus,
          action_closed_after_enter: actionSection?.dataset.open === "false" && actionToggle.getAttribute("aria-expanded") === "false",
          calls: window.__actionMenuKeyboardCalls
        };
      })()`);
      if (!keyboardState.arrow_focus_menuitem || keyboardState.end_focus !== "742" || !keyboardState.u_initial_focus_menuitem || keyboardState.u_focus !== "742" || !keyboardState.action_escape_closed || !keyboardState.action_escape_restored || keyboardState.shift_u_focus !== "741" || !keyboardState.shift_u_open || keyboardState.print_escape_restored_to !== keyboardState.before_print_hotkey_focus || !keyboardState.action_closed_after_enter) {
        throw new Error(`ActionMenus keyboard state invalid: ${JSON.stringify(keyboardState)}`);
      }
      const call = keyboardState.calls?.[0];
      if (!call || call.action !== 742 || call.additionalContext?.active_id !== 744 || !Array.isArray(call.additionalContext?.active_ids) || call.additionalContext.active_ids[0] !== 744) {
        throw new Error(`ActionMenus keyboard action context invalid: ${JSON.stringify(keyboardState)}`);
      }
      return { keyboard_state: keyboardState };
    }
  },
  {
    name: "default-kanban-action-menu-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_action_menu=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban action-menu TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        window.__kanbanActionMenuCalls = [];
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Kanban Actions",
            res_model: "res.partner",
            view_mode: "kanban,form",
            views: [[false, "kanban"], [false, "form"]]
          },
          activeView: "kanban",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              display_name: { type: "char", string: "Name" },
              email: { type: "char", string: "Email" }
            },
            relatedModels: {},
            views: {
              kanban: {
                arch: "<kanban><field name='display_name'/><field name='email'/></kanban>",
                id: 730,
                actionMenus: {
                  print: [{ id: 731, name: "Print Card", description: "Print Card", sequence: 2, groupNumber: 1 }],
                  action: [{ id: 732, name: "Run Card Action", sequence: 5, groupNumber: 2 }]
                }
              },
              form: { arch: "<form><field name='display_name'/></form>", id: 733 }
            }
          },
          records: [{ id: 734, display_name: "Action Menu Partner", email: "action-menu@example.test" }],
          length: 1,
          offset: 0,
          countLimited: false
        }, {
          services: {
            action: {
              doAction(action, options) {
                window.__kanbanActionMenuCalls.push({ action, additionalContext: options?.additionalContext || {} });
                return Promise.resolve(true);
              }
            }
          }
        });
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "kanban-action-menu";
        const card = root.querySelector(".o_kanban_record");
        const toggle = card?.querySelector(".o_kanban_record_menu_toggle[data-kanban-record-menu='true']");
        toggle?.click();
        const menu = card?.querySelector(".o_kanban_record_menu_dropdown");
        const items = [...(menu?.querySelectorAll("[data-kanban-record-server-action='true']") || [])].map((node) => ({
          kind: node.dataset.kanbanRecordMenuAction || "",
          action_id: node.dataset.actionId || "",
          record_id: node.dataset.recordId || "",
          label: node.textContent.trim()
        }));
        const actionButton = menu?.querySelector("[data-kanban-record-server-action='true'][data-kanban-record-menu-action='action']");
        actionButton?.click();
        await new Promise((resolve) => setTimeout(resolve, 0));
        return {
          card_id: card?.dataset.id || "",
          menu_open_after_click: toggle?.getAttribute("aria-expanded") || "",
          menu_hidden_after_click: menu?.hasAttribute("hidden") ?? true,
          items,
          calls: window.__kanbanActionMenuCalls
        };
      })()`);
      const cardCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-action-menu'] .o_kanban_record", 1, "Kanban action-menu card");
      const serverItemCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-action-menu'] [data-kanban-record-server-action='true']", 2, "Kanban server menu entries");
      if (renderedState.card_id !== "734" || renderedState.items.length !== 2) {
        throw new Error(`kanban action-menu entries invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.items.some((item) => item.kind === "print" && item.action_id === "731" && item.record_id === "734" && item.label === "Print Card")) {
        throw new Error(`kanban print menu entry invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.items.some((item) => item.kind === "action" && item.action_id === "732" && item.record_id === "734" && item.label === "Run Card Action")) {
        throw new Error(`kanban action menu entry invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.menu_open_after_click !== "false" || renderedState.menu_hidden_after_click !== true) {
        throw new Error(`kanban action menu did not close: ${JSON.stringify(renderedState)}`);
      }
      const call = renderedState.calls?.[0];
      if (!call || call.action !== 732 || call.additionalContext?.active_id !== 734 || !Array.isArray(call.additionalContext?.active_ids) || call.additionalContext.active_ids[0] !== 734) {
        throw new Error(`kanban action menu execution context invalid: ${JSON.stringify(renderedState)}`);
      }
      return { rendered_state: renderedState, card_count: cardCount, server_item_count: serverItemCount };
    }
  },
  {
    name: "default-kanban-drag-drop-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_drag_drop=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban drag-drop TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        const writeCalls = [];
        const dropEvents = [];
        let refreshCount = 0;
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Kanban Drag",
            res_model: "res.partner",
            view_mode: "kanban,form",
            views: [[false, "kanban"], [false, "form"]]
          },
          activeView: "kanban",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              display_name: { type: "char", string: "Name" },
              stage_id: { type: "many2one", relation: "crm.stage", string: "Stage" }
            },
            relatedModels: {},
            views: {
              kanban: { arch: "<kanban><field name='display_name'/><field name='stage_id'/></kanban>", id: 740 },
              form: { arch: "<form><field name='display_name'/></form>", id: 741 }
            }
          },
          search: {
            state: { query: "", facets: [], groupBy: ["stage_id"], domain: [] },
            suggestions: [],
            filters: [],
            groupBys: [],
            favorites: []
          },
          records: [
            { id: 742, display_name: "Drag Source", stage_id: [10, "New"] },
            { id: 743, display_name: "Drag Target", stage_id: [20, "Qualified"] }
          ],
          length: 2,
          offset: 0,
          countLimited: false
        }, {
          context: { active_id: 700 },
          onRefresh() {
            refreshCount += 1;
          },
          services: {
            orm: {
              write(model, ids, data, kwargs) {
                writeCalls.push({ model, ids, data, kwargs });
                return Promise.resolve(true);
              }
            }
          }
        });
        root.addEventListener("action:kanban-record-drop", (event) => dropEvents.push(event.detail));
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "kanban-drag-drop";
        const renderer = root.querySelector(".o_kanban_renderer.o_kanban_grouped");
        const groups = [...(renderer?.querySelectorAll(".o_kanban_group") || [])];
        const firstCard = groups[0]?.querySelector(".o_kanban_record");
        const secondRecords = groups[1]?.querySelector(".o_kanban_records");
        const data = new Map();
        const dataTransfer = {
          dropEffect: "",
          effectAllowed: "",
          setData(type, value) { data.set(type, String(value)); },
          getData(type) { return data.get(type) || ""; }
        };
        function dragEvent(type) {
          const event = new Event(type, { bubbles: true, cancelable: true });
          Object.defineProperty(event, "dataTransfer", { value: dataTransfer });
          return event;
        }
        firstCard?.dispatchEvent(dragEvent("dragstart"));
        const over = dragEvent("dragover");
        groups[1]?.dispatchEvent(over);
        const overState = {
          default_prevented: over.defaultPrevented,
          target_active: groups[1]?.dataset.dropTargetActive || "",
          target_class: groups[1]?.className || "",
          records_class: secondRecords?.className || "",
          dragging_id: renderer?.dataset.kanbanDraggingId || ""
        };
        groups[1]?.dispatchEvent(dragEvent("drop"));
        await new Promise((resolve) => setTimeout(resolve, 0));
        firstCard?.dispatchEvent(dragEvent("dragend"));
        return {
          group_count: groups.length,
          card_draggable: firstCard?.getAttribute("draggable") || "",
          card_group: firstCard?.dataset.groupValue || "",
          over_state: overState,
          write_calls: writeCalls,
          drop_events: dropEvents,
          refresh_count: refreshCount,
          drop_field: renderer?.dataset.kanbanDropField || "",
          drop_value: renderer?.dataset.kanbanDropValue || "",
          dragging_id_after: renderer?.dataset.kanbanDraggingId || ""
        };
      })()`);
      const groupCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-drag-drop'] .o_kanban_group", 2, "Kanban drag-drop groups");
      const draggableCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-drag-drop'] .o_kanban_record[draggable='true'][data-kanban-draggable='true']", 2, "Kanban draggable cards");
      if (renderedState.group_count !== 2 || renderedState.card_draggable !== "true" || renderedState.card_group !== "10") {
        throw new Error(`kanban drag metadata invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.over_state.default_prevented || renderedState.over_state.target_active !== "true" || !renderedState.over_state.target_class.includes("o_kanban_group_drop_target") || !renderedState.over_state.records_class.includes("o_kanban_records_drop_target")) {
        throw new Error(`kanban drop target state invalid: ${JSON.stringify(renderedState)}`);
      }
      const write = renderedState.write_calls?.[0];
      if (!write || write.model !== "res.partner" || write.ids?.[0] !== 742 || write.data?.stage_id !== 20 || write.kwargs?.context?.active_id !== 700) {
        throw new Error(`kanban drop write invalid: ${JSON.stringify(renderedState)}`);
      }
      const drop = renderedState.drop_events?.[0];
      if (!drop || drop.id !== 742 || drop.field !== "stage_id" || drop.value !== 20 || drop.previousGroupKey !== "10" || drop.groupKey !== "20") {
        throw new Error(`kanban drop event invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.refresh_count !== 1 || renderedState.drop_field !== "stage_id" || renderedState.drop_value !== "20" || renderedState.dragging_id_after) {
        throw new Error(`kanban drop final state invalid: ${JSON.stringify(renderedState)}`);
      }
      return { rendered_state: renderedState, group_count: groupCount, draggable_count: draggableCount };
    }
  },
  {
    name: "default-kanban-group-load-more-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_group_load_more=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban group load-more TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        const loadEvents = [];
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Kanban Column Load",
            res_model: "res.partner",
            view_mode: "kanban,form",
            views: [[false, "kanban"], [false, "form"]],
            __kanban_group_limit: 2
          },
          activeView: "kanban",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              display_name: { type: "char", string: "Name" },
              stage_id: { type: "many2one", relation: "crm.stage", string: "Stage" }
            },
            relatedModels: {},
            views: {
              kanban: { arch: "<kanban><field name='display_name'/><field name='stage_id'/></kanban>", id: 750 },
              form: { arch: "<form><field name='display_name'/></form>", id: 751 }
            }
          },
          search: {
            state: { query: "", facets: [], groupBy: ["stage_id"], domain: [] },
            suggestions: [],
            filters: [],
            groupBys: [],
            favorites: []
          },
          records: [
            { id: 752, display_name: "Column A", stage_id: [30, "New"] },
            { id: 753, display_name: "Column B", stage_id: [30, "New"] },
            { id: 754, display_name: "Column C", stage_id: [30, "New"] },
            { id: 755, display_name: "Column D", stage_id: [30, "New"] },
            { id: 756, display_name: "Column E", stage_id: [30, "New"] },
            { id: 757, display_name: "Done", stage_id: [40, "Done"] }
          ],
          length: 6,
          offset: 0,
          countLimited: false
        });
        root.addEventListener("action:kanban-group-load-more", (event) => loadEvents.push(event.detail));
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "kanban-group-load-more";
        const firstGroup = root.querySelector(".o_kanban_group");
        const button = firstGroup?.querySelector(".o_kanban_group_load_more[data-kanban-group-load-more='true']");
        function state() {
          const cards = [...(firstGroup?.querySelectorAll(".o_kanban_record") || [])].map((node) => ({
            id: node.dataset.id || "",
            hidden: node.hasAttribute("hidden"),
            group_hidden: node.dataset.kanbanGroupHidden || ""
          }));
          return {
            group_count: root.querySelectorAll(".o_kanban_group").length,
            button_count: firstGroup?.querySelectorAll(".o_kanban_group_load_more[data-kanban-group-load-more='true']").length || 0,
            loaded: button?.dataset.loaded || "",
            total: button?.dataset.total || "",
            remaining: button?.dataset.remaining || "",
            button_hidden: button?.hasAttribute("hidden") || false,
            cards,
            events: [...loadEvents]
          };
        }
        const before = state();
        button?.click();
        const afterFirst = state();
        button?.click();
        const afterSecond = state();
        return { before, after_first: afterFirst, after_second: afterSecond };
      })()`);
      const groupCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-group-load-more'] .o_kanban_group", 2, "Kanban group-load groups");
      const loadButtonCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-group-load-more'] .o_kanban_group_load_more[data-kanban-group-load-more='true']", 1, "Kanban group-load button");
      if (renderedState.before.group_count !== 2 || renderedState.before.button_count !== 1 || renderedState.before.loaded !== "2" || renderedState.before.total !== "5" || renderedState.before.remaining !== "3") {
        throw new Error(`kanban group load-more initial invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.before.cards.filter((card) => card.hidden).length !== 3) {
        throw new Error(`kanban group load-more hidden records invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.after_first.loaded !== "4" || renderedState.after_first.remaining !== "1" || renderedState.after_first.cards.filter((card) => card.hidden).length !== 1) {
        throw new Error(`kanban group load-more first reveal invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.after_second.loaded !== "5" || renderedState.after_second.remaining !== "0" || renderedState.after_second.button_hidden !== true || renderedState.after_second.cards.some((card) => card.hidden)) {
        throw new Error(`kanban group load-more final invalid: ${JSON.stringify(renderedState)}`);
      }
      if (renderedState.after_second.events.length !== 2 || renderedState.after_second.events[0].revealed !== 2 || renderedState.after_second.events[1].remaining !== 0) {
        throw new Error(`kanban group load-more events invalid: ${JSON.stringify(renderedState)}`);
      }
      return { rendered_state: renderedState, group_count: groupCount, load_button_count: loadButtonCount };
    }
  },
  {
    name: "default-kanban-template-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_template=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban template TS webclient ready");
      const renderedState = await evaluate(page, `(async () => {
        const module = await import("/web/static/frontend/packages/webclient/src/index.js");
        const outlet = document.querySelector(".o_web_client .o_action_manager") || document.body;
        const root = module.renderWindowAction({
          type: "ir.actions.act_window",
          action: {
            name: "Kanban Template",
            res_model: "res.partner",
            view_mode: "kanban,form",
            views: [[false, "kanban"], [false, "form"]]
          },
          activeView: "kanban",
          resModel: "res.partner",
          viewDescriptions: {
            fields: {
              display_name: { type: "char", string: "Name" },
              email: { type: "char", string: "Email" },
              state: { type: "selection", string: "State", selection: [["new", "New"], ["done", "Done"]] },
              tags: { type: "many2many", string: "Tags", relation: "res.partner.category" },
              employee_id: { type: "many2one", string: "Employee", relation: "hr.employee" },
              url: { type: "char", string: "URL" }
            },
            relatedModels: {},
            views: {
              kanban: {
                arch: "<kanban><field name='display_name'/><field name='email'/><field name='state'/><field name='tags'/><field name='employee_id'/><field name='url'/><templates><t t-name='kanban-box'><div class='tmpl-card' t-att-data-state='record.state.raw_value' t-att-data-id='record.id.raw_value' t-att-title='record.display_name.value' t-attf-aria-label='Partner #{record.display_name.value}' t-attf-class='state-#{record.state.raw_value}'><t t-set='badge' t-value='record.state.value'/><t t-set='body_note'><span class='tmpl-captured'>Captured:<t t-esc='record.display_name.value'/></span></t><strong class='tmpl-title'><field name='display_name'/></strong><field name='employee_id' widget='many2one_avatar_employee' class='tmpl-assignee'/><field name='state' widget='badge' decoration-success='state == &quot;new&quot;' class='tmpl-state-badge'/><field name='tags' widget='many2many_tags' class='tmpl-tag-widget'/><span class='tmpl-badge' t-att-data-badge='badge' t-esc='badge'/><t t-out='body_note'/><a class='tmpl-link' t-att-href='record.url.raw_value' rel='noopener'>Open</a><t t-if='record.email.raw_value'><span class='tmpl-email'><field name='email'/></span></t><span class='tmpl-state' t-esc='record.state.value'/><t t-call='kanban-tag-list'><span class='tmpl-slot'>Slot:<t t-esc='record.state.value'/></span></t></div></t><t t-name='kanban-tag-list'><section class='tmpl-subtemplate' data-called='tag-list'><t t-out='0'/><ul class='tmpl-tags'><t t-foreach='record.tags.raw_value' t-as='tag'><li class='tmpl-tag' t-att-data-index='tag_index' t-attf-class='tag-#{tag_index}' t-esc='tag[1]'/></t></ul></section></t><t t-inherit='kanban-box' t-inherit-mode='extension'><xpath expr='//div[hasclass(&quot;tmpl-card&quot;)]' position='inside'><span class='tmpl-inherited-inside' t-esc='record.email.value'/></xpath><xpath expr='//strong[hasclass(&quot;tmpl-title&quot;)]' position='after'><span class='tmpl-inherited-after'>After Title</span></xpath><xpath expr='//field[@name=&quot;employee_id&quot;]' position='attributes'><attribute name='class' add='tmpl-inherited-avatar' separator=' '/></xpath></t></templates></kanban>",
                id: 760
              },
              form: { arch: "<form><field name='display_name'/></form>", id: 761 }
            }
          },
          records: [
            { id: 762, display_name: "Template A", email: "template-a@example.test", state: "new", tags: [[12, "VIP"], [13, "Supplier"]], employee_id: [17, "Mina Reyes"], url: "#record-762" },
            { id: 763, display_name: "Template B", email: "", state: "done", tags: [], employee_id: [18, "Omar Vale"], url: "#record-763" }
          ],
          length: 2,
          offset: 0,
          countLimited: false
        });
        outlet.replaceChildren(root);
        root.dataset.smokeRendered = "kanban-template";
        const cards = [...root.querySelectorAll(".o_kanban_record")].map((card) => ({
          id: card.dataset.id || "",
          template: card.querySelector(".o_kanban_template_details")?.dataset.kanbanTemplate || "",
          body_count: card.querySelectorAll(".o_kanban_template_body[data-kanban-template-body='true']").length,
          fallback_field_count: card.querySelectorAll(".o_kanban_record_field").length,
          title: card.querySelector(".tmpl-title")?.textContent?.trim() || "",
          email_count: card.querySelectorAll(".tmpl-email").length,
          email: card.querySelector(".tmpl-email")?.textContent?.trim() || "",
          state: card.querySelector(".tmpl-state")?.textContent?.trim() || "",
          root_class: card.querySelector(".tmpl-card")?.className || "",
          root_data_id: card.querySelector(".tmpl-card")?.dataset.id || "",
          root_data_state: card.querySelector(".tmpl-card")?.dataset.state || "",
          root_title: card.querySelector(".tmpl-card")?.getAttribute("title") || "",
          root_aria: card.querySelector(".tmpl-card")?.getAttribute("aria-label") || "",
          badge: card.querySelector(".tmpl-badge")?.textContent?.trim() || "",
          badge_data: card.querySelector(".tmpl-badge")?.dataset.badge || "",
          field_badge: card.querySelector(".gorp-badge[data-field='state']")?.textContent?.trim() || "",
          field_badge_decoration: card.querySelector(".gorp-badge[data-field='state']")?.dataset.decoration || "",
          x2many_count: card.querySelector(".gorp-x2many-tags[data-field='tags']")?.dataset.count || "",
          x2many_first_tag: card.querySelector(".gorp-x2many-tags[data-field='tags'] .gorp-x2many-tag")?.textContent?.trim() || "",
          avatar_relation: card.querySelector(".gorp-many2one-avatar[data-field='employee_id']")?.dataset.relation || "",
          avatar_res_id: card.querySelector(".gorp-many2one-avatar[data-field='employee_id']")?.dataset.resId || "",
          inherited_inside: card.querySelector(".tmpl-inherited-inside")?.textContent?.trim() || "",
          inherited_after: card.querySelector(".tmpl-inherited-after")?.textContent?.trim() || "",
          inherited_avatar_field: card.querySelector(".tmpl-inherited-avatar")?.dataset.field || "",
          captured: card.querySelector(".tmpl-captured")?.textContent?.trim() || "",
          slot: card.querySelector(".tmpl-slot")?.textContent?.trim() || "",
          subtemplate_called: card.querySelector(".tmpl-subtemplate")?.dataset.called || "",
          link_href: card.querySelector(".tmpl-link")?.getAttribute("href") || "",
          link_rel: card.querySelector(".tmpl-link")?.getAttribute("rel") || "",
          tag_list_called: card.querySelector(".tmpl-subtemplate")?.dataset.called || "",
          tags: [...card.querySelectorAll(".tmpl-tag")].map((tag) => ({
            text: tag.textContent?.trim() || "",
            index: tag.dataset.index || "",
            class_name: tag.className || ""
          }))
        }));
        return { card_count: cards.length, cards };
      })()`);
      const cardCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-template'] .o_kanban_record", 2, "Kanban template cards");
      const templateCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-smoke-rendered='kanban-template'] .o_kanban_template_body[data-kanban-template-body='true']", 2, "Kanban template bodies");
      if (renderedState.card_count !== 2 || renderedState.cards.some((card) => card.template !== "kanban-box" || card.body_count !== 1 || card.fallback_field_count !== 0)) {
        throw new Error(`kanban template body invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.title === "Template A" && card.email === "template-a@example.test" && card.state === "New" && card.root_class.includes("state-new"))) {
        throw new Error(`kanban template first card invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.root_data_id === "762" && card.root_data_state === "new" && card.root_title === "Template A" && card.root_aria === "Partner Template A" && card.link_href === "#record-762" && card.link_rel === "noopener")) {
        throw new Error(`kanban template dynamic attributes invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.badge === "New" && card.badge_data === "New" && card.tag_list_called === "tag-list")) {
        throw new Error(`kanban template t-set/t-call invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.field_badge === "New" && card.field_badge_decoration === "success" && card.x2many_count === "2" && card.x2many_first_tag === "VIP" && card.avatar_relation === "hr.employee" && card.avatar_res_id === "17")) {
        throw new Error(`kanban template field widgets invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.inherited_inside === "template-a@example.test" && card.inherited_after === "After Title" && card.inherited_avatar_field === "employee_id")) {
        throw new Error(`kanban template inheritance invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.captured === "Captured:Template A" && card.slot === "Slot:New" && card.subtemplate_called === "tag-list")) {
        throw new Error(`kanban template body capture/slot invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "762" && card.tags.length === 2 && card.tags[0].text === "VIP" && card.tags[0].index === "0" && card.tags[0].class_name.includes("tag-0") && card.tags[1].text === "Supplier" && card.tags[1].index === "1" && card.tags[1].class_name.includes("tag-1"))) {
        throw new Error(`kanban template loop invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "763" && card.title === "Template B" && card.email_count === 0 && card.state === "Done" && card.root_class.includes("state-done"))) {
        throw new Error(`kanban template conditional card invalid: ${JSON.stringify(renderedState)}`);
      }
      if (!renderedState.cards.some((card) => card.id === "763" && card.tags.length === 0 && card.link_href === "#record-763")) {
        throw new Error(`kanban template empty loop invalid: ${JSON.stringify(renderedState)}`);
      }
      return { rendered_state: renderedState, card_count: cardCount, template_count: templateCount };
    }
  },
  {
    name: "default-kanban-groupby-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&kanban_groupby_setup=1`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "kanban groupby setup document ready");
      await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
      const fixture = await createKanbanSmokeAction(page, config);
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#action=${fixture.actionID}&model=ir.actions.server&view_type=kanban`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Kanban groupby TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban groupby action ready");
      await clickSelector(page, ".o_web_client .o_action_manager .o_searchview_dropdown_toggler");
      const groupOption = await waitFor(page, `(() => {
        const item = document.querySelector(".o_web_client .o_action_manager .o_group_by_menu .o_menu_item[data-menu-item-id]");
        return item ? { id: item.dataset.menuItemId || "", label: item.textContent.trim() } : null;
      })()`, "Kanban groupby option");
      await clickFirst(page, ".o_web_client .o_action_manager .o_group_by_menu .o_menu_item[data-menu-item-id]");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Kanban state groupby action ready");
      const groupedState = await waitFor(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='kanban']");
        const renderer = root?.querySelector(".o_kanban_renderer.o_kanban_grouped");
        const groups = [...(renderer?.querySelectorAll(".o_kanban_group") || [])].map((group) => ({
          label: group.querySelector(".o_kanban_header_title")?.textContent?.trim() || "",
          count: group.querySelector(".o_kanban_counter")?.textContent?.trim() || "",
          cards: group.querySelectorAll(".o_kanban_record").length,
          groupby: group.dataset.groupby || "",
          folded: group.dataset.folded || "",
          fold_toggle_count: group.querySelectorAll(".o_kanban_group_fold_toggle[data-kanban-group-fold='true']").length,
          fold_expanded: group.querySelector(".o_kanban_group_fold_toggle[data-kanban-group-fold='true']")?.getAttribute("aria-expanded") || "",
          quick_add_count: group.querySelectorAll(".o_kanban_quick_add[data-kanban-quick-create='true']").length,
          quick_add_label: group.querySelector(".o_kanban_quick_add[data-kanban-quick-create='true']")?.textContent?.trim() || "",
          quick_add_group_field: group.querySelector(".o_kanban_quick_add[data-kanban-quick-create='true']")?.dataset.groupField || "",
          quick_add_group_default: group.querySelector(".o_kanban_quick_add[data-kanban-quick-create='true']")?.dataset.groupDefault || ""
        }));
        const facet = document.querySelector(".o_web_client .o_action_manager .o_searchview_facet[data-facet-id^='group-by-']");
        return renderer && groups.length && facet ? {
          renderer_groupby: renderer.dataset.groupby || "",
          renderer_group_field: renderer.dataset.groupField || "",
          groups,
          facet: facet.textContent.trim(),
          hash: window.location.hash || ""
        } : null;
      })()`, "Kanban grouped columns");
      if (!groupedState.renderer_groupby || !groupedState.renderer_group_field) {
        throw new Error(`kanban grouped metadata invalid: ${JSON.stringify(groupedState)}`);
      }
      if (!groupedState.groups.some((group) => group.label && Number(group.cards) >= 1 && group.groupby === groupedState.renderer_groupby)) {
        throw new Error(`kanban grouped columns invalid: ${JSON.stringify(groupedState)}`);
      }
      if (!groupedState.groups.every((group) => group.quick_add_count === 1 && group.quick_add_label === "+ Add" && group.quick_add_group_field === groupedState.renderer_group_field)) {
        throw new Error(`kanban grouped quick create invalid: ${JSON.stringify(groupedState)}`);
      }
      if (!groupedState.groups.every((group) => group.folded === "false" && group.fold_toggle_count === 1 && group.fold_expanded === "true")) {
        throw new Error(`kanban grouped fold controls invalid: ${JSON.stringify(groupedState)}`);
      }
      await clickFirst(page, ".o_web_client .o_action_manager .o_kanban_group .o_kanban_group_fold_toggle");
      const foldedState = await waitFor(page, `(() => {
        const group = document.querySelector(".o_web_client .o_action_manager .o_kanban_group");
        const records = group?.querySelector(".o_kanban_records");
        const quick = group?.querySelector(".o_kanban_quick_add");
        const toggle = group?.querySelector(".o_kanban_group_fold_toggle");
        return group?.dataset.folded === "true" && group.classList.contains("o_column_folded") && records?.hasAttribute("hidden") && quick?.hasAttribute("hidden") && toggle?.getAttribute("aria-expanded") === "false"
          ? { folded: group.dataset.folded, expanded: toggle.getAttribute("aria-expanded"), cards_hidden: records.hasAttribute("hidden"), quick_hidden: quick.hasAttribute("hidden") }
          : null;
      })()`, "Kanban grouped column folds");
      await clickFirst(page, ".o_web_client .o_action_manager .o_kanban_group .o_kanban_group_fold_toggle");
      const unfoldedState = await waitFor(page, `(() => {
        const group = document.querySelector(".o_web_client .o_action_manager .o_kanban_group");
        const records = group?.querySelector(".o_kanban_records");
        const quick = group?.querySelector(".o_kanban_quick_add");
        const toggle = group?.querySelector(".o_kanban_group_fold_toggle");
        return group?.dataset.folded === "false" && !group.classList.contains("o_column_folded") && !records?.hasAttribute("hidden") && !quick?.hasAttribute("hidden") && toggle?.getAttribute("aria-expanded") === "true"
          ? { folded: group.dataset.folded, expanded: toggle.getAttribute("aria-expanded"), cards_hidden: records.hasAttribute("hidden"), quick_hidden: quick.hasAttribute("hidden") }
          : null;
      })()`, "Kanban grouped column unfolds");
      return { fixture, group_option: groupOption, grouped_state: groupedState, folded_state: foldedState, unfolded_state: unfoldedState };
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
    name: "default-direct-model-route-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}#model=res.users&view_type=list`) });
      await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "direct model route document ready");
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "direct model route TS webclient ready");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "direct model route action ready");
      const state = await evaluate(page, `(() => ({
        url: location.href,
        root_view: document.querySelector(".o_web_client")?.dataset.view || "",
        model: document.querySelector(".o_web_client .gorp-window-action")?.dataset.model || "",
        view: document.querySelector(".o_web_client .gorp-window-action")?.dataset.view || "",
        list_count: document.querySelectorAll(".o_web_client .o_list_renderer").length,
        row_count: document.querySelectorAll(".o_web_client .o_data_row, .o_web_client tbody tr").length,
        home_count: document.querySelectorAll(".o_web_client .o_home_menu .o_app").length
      }))()`);
      if (
        state.root_view !== "action" ||
        state.model !== "res.users" ||
        state.view !== "list" ||
        state.list_count !== 1 ||
        state.row_count < 1 ||
        state.home_count !== 0 ||
        !state.url.includes("#model=res.users&view_type=list") ||
        state.url.includes("menu_id=model")
      ) {
        throw new Error(`direct model route invalid: ${JSON.stringify(state)}`);
      }
      return state;
    }
  },
  {
    name: "default-mobile-launcher-parity",
    viewport: { width: 390, height: 844, mobile: true },
    run: async (page, config) => {
      await setViewport(page, mobileViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "mobile launcher TS webclient ready");
      const appCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app", 2, "mobile launcher app tiles");
      const launcherState = await evaluate(page, `(() => {
        const grid = document.querySelector(".o_web_client .o_home_menu .o_apps");
        const launcher = document.querySelector(".o_web_client .o-app-launcher-view");
        const launcherStyle = launcher ? getComputedStyle(launcher) : null;
        const wrappers = [...document.querySelectorAll(".o_web_client .o_home_menu .o_draggable")];
        const cards = [...document.querySelectorAll(".o_web_client .o_home_menu .o_app")];
        const cardRects = wrappers.length ? wrappers.map((card) => card.getBoundingClientRect()) : cards.map((card) => card.getBoundingClientRect());
        const top = Math.min(...cardRects.map((rect) => Math.round(rect.top)));
        const firstRow = cardRects.filter((rect) => Math.abs(Math.round(rect.top) - top) <= 2);
        const icon = document.querySelector(".o_web_client .o_home_menu .o_app .o_app_icon");
        const iconRect = icon?.getBoundingClientRect();
        const search = document.querySelector(".o_web_client .o_home_menu .o_home_menu_search");
        const searchStyle = search ? getComputedStyle(search) : null;
        const hiddenSearch = document.querySelector(".o_web_client .o_home_menu .o_search_hidden");
        const avatar = document.querySelector(".o_web_client[data-view='apps'] .o_user_menu .o_user_avatar");
        const avatarRect = avatar?.getBoundingClientRect();
        const avatarStyle = avatar ? getComputedStyle(avatar) : null;
        const systrayItems = [...document.querySelectorAll(".o_web_client[data-view='apps'] .o_menu_systray [role='menuitem']")];
        const visibleSystrayItems = systrayItems.filter((node) => {
          const rect = node.getBoundingClientRect();
          const style = getComputedStyle(node);
          return rect.width > 0 && rect.height > 0 && style.display !== "none" && style.visibility !== "hidden";
        });
        return {
          launcher_bg: launcherStyle?.backgroundColor || "",
          launcher_bg_image: launcherStyle?.backgroundImage || "",
          draggable_count: wrappers.length,
          first_row_count: firstRow.length,
          icon_width_px: iconRect ? Math.round(iconRect.width) : 0,
          icon_height_px: iconRect ? Math.round(iconRect.height) : 0,
          hidden_search_count: hiddenSearch ? 1 : 0,
          banner_count: document.querySelectorAll(".o_web_client .o_home_menu .o_home_menu_registration_banner").length,
          search_hidden: Boolean(searchStyle && searchStyle.opacity === "0" && Number.parseFloat(searchStyle.maxHeight) === 0),
          user_avatar_visible: Boolean(avatarRect && avatarRect.width >= 24 && avatarRect.height >= 24 && avatarStyle?.display !== "none"),
          systray_visible_count: visibleSystrayItems.length,
          horizontal_overflow_px: document.documentElement.scrollWidth - window.innerWidth
        };
      })()`);
      if (!isDarkLauncherBackground(launcherState.launcher_bg)) throw new Error(`mobile launcher background is not dark: ${JSON.stringify(launcherState)}`);
      if (!isEnterpriseHomeBackgroundImage(launcherState.launcher_bg_image)) throw new Error(`mobile launcher enterprise background missing: ${JSON.stringify(launcherState)}`);
      if (launcherState.horizontal_overflow_px > 1) throw new Error(`mobile launcher horizontal overflow: ${launcherState.horizontal_overflow_px}px`);
      if (launcherState.draggable_count < appCount) throw new Error(`mobile launcher wrapper contract invalid: ${JSON.stringify(launcherState)}`);
      if (appCount >= 4 && launcherState.first_row_count !== 4) throw new Error(`mobile launcher first row invalid: ${JSON.stringify(launcherState)}`);
      if (launcherState.icon_width_px < 62 || launcherState.icon_width_px > 74) throw new Error(`mobile launcher icon width invalid: ${JSON.stringify(launcherState)}`);
      if (launcherState.icon_height_px < 62 || launcherState.icon_height_px > 74) throw new Error(`mobile launcher icon height invalid: ${JSON.stringify(launcherState)}`);
      if (launcherState.banner_count !== 1) throw new Error(`mobile launcher banner missing: ${JSON.stringify(launcherState)}`);
      if (launcherState.hidden_search_count !== 1) throw new Error(`mobile launcher hidden search missing: ${JSON.stringify(launcherState)}`);
      if (!launcherState.search_hidden) throw new Error(`mobile launcher search should start hidden: ${JSON.stringify(launcherState)}`);
      if (!launcherState.user_avatar_visible) throw new Error(`mobile launcher user avatar hidden: ${JSON.stringify(launcherState)}`);
      if (launcherState.systray_visible_count < 3) throw new Error(`mobile launcher systray too sparse: ${JSON.stringify(launcherState)}`);
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Settings", ".o_app_name");
      await waitFor(page, `document.querySelector("main.o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "mobile launcher Settings action ready");
      const settingsContentCount = await waitForCount(page, "main.o_web_client .o_action_manager .o_settings_container", 1, "mobile launcher Settings content");
      const settingsControlPanelCount = await waitForCount(page, "main.o_web_client .o_action_manager .o_control_panel", 1, "mobile launcher Settings control panel");
      const settingsOpenState = await evaluate(page, `(() => {
        const shell = document.querySelector("main.o_web_client");
        const body = document.body;
        return {
          shell_view: shell?.dataset.view || "",
          body_view: body.dataset.view || "",
          shell_home_mode: shell?.dataset.homeMenuMode || "",
          body_home_mode: body.dataset.homeMenuMode || "",
          home_count: document.querySelectorAll("main.o_web_client .o_action_manager .o_home_menu").length,
          banner_count: document.querySelectorAll("main.o_web_client .o_action_manager .o_home_menu_registration_banner").length,
          settings_count: document.querySelectorAll("main.o_web_client .o_action_manager .o_settings_container").length,
          control_panel_count: document.querySelectorAll("main.o_web_client .o_action_manager .o_control_panel").length,
          hash: window.location.hash || ""
        };
      })()`);
      if (settingsOpenState.shell_view !== "action" || settingsOpenState.body_view !== "action" || settingsOpenState.shell_home_mode || settingsOpenState.body_home_mode || settingsOpenState.home_count !== 0 || settingsOpenState.banner_count !== 0 || settingsOpenState.settings_count !== 1 || settingsOpenState.control_panel_count !== 1 || !settingsOpenState.hash.includes("model=res.config.settings")) {
        throw new Error(`mobile launcher Settings action invalid: ${JSON.stringify(settingsOpenState)}`);
      }
      return { app_count: appCount, launcher_state: launcherState, settings_content_count: settingsContentCount, settings_control_panel_count: settingsControlPanelCount, settings_open_state: settingsOpenState };
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
      const cardState = await evaluate(page, `(() => {
        const card = document.querySelector(".o_web_client .o_action_manager .o_mobile_list_cards .o_mobile_record_card");
        return {
          role: card?.getAttribute("role") || "",
          title: card?.querySelector(".o_mobile_record_title")?.textContent?.trim() || "",
          state: card?.querySelector(".o_mobile_record_state")?.textContent?.trim() || "",
          open_buttons: document.querySelectorAll(".o_web_client .o_action_manager .o_mobile_list_cards .o_mobile_record_open").length
        };
      })()`);
      if (cardState.role !== "link" || !cardState.title || cardState.state === "code" || cardState.open_buttons !== 0) {
        throw new Error(`default TS mobile Server Actions card invalid: ${JSON.stringify(cardState)}`);
      }
      await clickFirst(page, ".o_web_client .o_action_manager .o_mobile_list_cards .o_mobile_record_card");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "default TS mobile form action ready");
      const formCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form'] .gorp-form-view", 1, "default TS mobile Server Actions form");
      const breadcrumbCount = await waitForCount(page, ".o_web_client .o_action_manager .o_control_panel_breadcrumbs", 1, "default TS mobile breadcrumbs");
      const sheetCount = await waitForCount(page, ".o_web_client .o_action_manager .o_form_sheet", 1, "default TS mobile form sheet");
      const formControlState = await evaluate(page, `(() => ({
        search_inputs: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_searchview_input").length,
        search_toggles: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_searchview_dropdown_toggler").length
      }))()`);
      if (formControlState.search_inputs !== 0 || formControlState.search_toggles !== 0) {
        throw new Error(`default TS mobile form exposes list search controls: ${JSON.stringify(formControlState)}`);
      }
      const mobileFormChrome = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const mainButtons = root?.querySelector(".o_control_panel_main_buttons");
        const actions = root?.querySelector(".o_control_panel_actions");
        const actionMenu = actions?.querySelector(".gorp-form-action-menu");
        const actionToggle = actionMenu?.querySelector(".gorp-action-menu-toggle");
        const actionToggleRect = actionToggle?.getBoundingClientRect();
        const actionToggleStyle = actionToggle ? getComputedStyle(actionToggle) : null;
        const looseActionMenus = root?.querySelectorAll(".gorp-form-view .gorp-form-action-menu").length || 0;
        const visibleSwitchButtons = [...(root?.querySelectorAll(".o_cp_switch_buttons .o_switch_view") || [])].filter((node) => {
          const rect = node.getBoundingClientRect();
          const style = getComputedStyle(node);
          return rect.width > 0 && rect.height > 0 && style.display !== "none" && style.visibility !== "hidden";
        });
        return {
          action_menu_in_actions: Boolean(actionMenu),
          action_menu_in_main_buttons: Boolean(mainButtons?.querySelector(".gorp-form-action-menu")),
          action_menu_placement: actionMenu?.dataset.controlPanelPlacement || "",
          action_toggle_width_px: actionToggleRect ? Math.round(actionToggleRect.width) : 0,
          action_toggle_font_size_px: actionToggleStyle ? Number.parseFloat(actionToggleStyle.fontSize) || 0 : -1,
          loose_action_menus: looseActionMenus,
          visible_switch_buttons: visibleSwitchButtons.length
        };
      })()`);
      if (!mobileFormChrome.action_menu_in_actions || mobileFormChrome.action_menu_in_main_buttons || mobileFormChrome.action_menu_placement !== "actions" || mobileFormChrome.loose_action_menus !== 0 || mobileFormChrome.visible_switch_buttons !== 0) {
        throw new Error(`default TS mobile form chrome invalid: ${JSON.stringify(mobileFormChrome)}`);
      }
      if (mobileFormChrome.action_toggle_width_px < 32 || mobileFormChrome.action_toggle_width_px > 40 || mobileFormChrome.action_toggle_font_size_px !== 0) {
        throw new Error(`default TS mobile form action toggle not compact: ${JSON.stringify(mobileFormChrome)}`);
      }
      const mobileFormVisualState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='form']");
        const isVisible = (node) => {
          if (!node) return false;
          const rect = node.getBoundingClientRect();
          const style = getComputedStyle(node);
          return rect.width > 0 && rect.height > 0 && style.display !== "none" && style.visibility !== "hidden";
        };
        const body = root?.querySelector(".gorp-form-body.o_form_sheet_bg");
        const sheet = root?.querySelector(".gorp-form-sheet.o_form_sheet");
        const title = root?.querySelector(".oe_title h1");
        const modelValueNode = root?.querySelector(".gorp-form-field[data-field='model_id'] output, .gorp-form-field[data-field='model_id'] .gorp-many2one-link, .gorp-form-field[data-field='model_id'] .gorp-field-value");
        const groupField = root?.querySelector(".gorp-form-field[data-field='group_ids']");
        const activeField = root?.querySelector(".gorp-form-field[data-field='active']");
        const nameField = root?.querySelector(".gorp-form-field[data-field='name']");
        const code = root?.querySelector(".gorp-code-viewer");
        const sheetRect = sheet?.getBoundingClientRect();
        const bodyRect = body?.getBoundingClientRect();
        const sheetStyle = sheet ? getComputedStyle(sheet) : null;
        const titleRect = title?.getBoundingClientRect();
        const titleStyle = title ? getComputedStyle(title) : null;
        const lineHeight = titleStyle ? Number.parseFloat(titleStyle.lineHeight) || 0 : 0;
        const codeRect = code?.getBoundingClientRect();
        const labels = [...(root?.querySelectorAll(".gorp-form-field > .o_form_label") || [])]
          .filter(isVisible)
          .map((node) => node.textContent.trim());
        const stateLabels = [...(root?.querySelectorAll(".gorp-selection-pills[data-field='state'] .gorp-selection-pill") || [])]
          .filter(isVisible)
          .map((node) => node.textContent.trim());
        return {
          body_left_px: bodyRect ? Math.round(bodyRect.left) : -1,
          sheet_left_px: sheetRect ? Math.round(sheetRect.left) : -1,
          sheet_width_px: sheetRect ? Math.round(sheetRect.width) : 0,
          viewport_width_px: window.innerWidth,
          sheet_border_top_width: sheetStyle?.borderTopWidth || "",
          sheet_radius: sheetStyle?.borderTopLeftRadius || "",
          title_text: title?.textContent?.trim() || "",
          title_line_count: titleRect && lineHeight ? Number((titleRect.height / lineHeight).toFixed(2)) : 0,
          model_value: modelValueNode?.textContent?.trim() || "",
          labels,
          state_labels: stateLabels,
          group_visible: isVisible(groupField),
          active_visible: isVisible(activeField),
          name_visible: isVisible(nameField),
          code_height_px: codeRect ? Math.round(codeRect.height) : 0
        };
      })()`);
      const expectedStateLabels = ["Update Record", "Create Record", "Duplicate Record", "Execute Code", "Send Webhook Notification", "Multi Actions"];
      if (
        Math.abs(mobileFormVisualState.sheet_left_px) > 1 ||
        Math.abs(mobileFormVisualState.sheet_width_px - mobileFormVisualState.viewport_width_px) > 2 ||
        mobileFormVisualState.sheet_border_top_width !== "0px" ||
        mobileFormVisualState.sheet_radius !== "0px" ||
        mobileFormVisualState.title_text !== "Base: Auto-vacuum internal data" ||
        mobileFormVisualState.title_line_count > 1.15 ||
        mobileFormVisualState.model_value !== "Automatic Vacuum" ||
        !mobileFormVisualState.labels.includes("Model") ||
        !mobileFormVisualState.labels.includes("Allowed Groups") ||
        !mobileFormVisualState.labels.includes("Type") ||
        mobileFormVisualState.labels.includes("Name") ||
        mobileFormVisualState.labels.includes("Code") ||
        mobileFormVisualState.active_visible ||
        mobileFormVisualState.name_visible ||
        !mobileFormVisualState.group_visible ||
        JSON.stringify(mobileFormVisualState.state_labels) !== JSON.stringify(expectedStateLabels) ||
        mobileFormVisualState.code_height_px < 16 ||
        mobileFormVisualState.code_height_px > 60
      ) {
        throw new Error(`default TS mobile form visual parity invalid: ${JSON.stringify(mobileFormVisualState)}`);
      }
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_control_panel_actions .gorp-action-menu-toggle[data-action-menu-toggle='action']");
      const actionMenuOpenState = await waitFor(page, `(() => {
        const section = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .gorp-action-menu-section[data-menu='action']");
        const toggle = section?.querySelector(".gorp-action-menu-toggle");
        const menu = section?.querySelector(".gorp-action-menu-items");
        const rect = menu?.getBoundingClientRect();
        const style = menu ? getComputedStyle(menu) : null;
        const itemCount = menu?.querySelectorAll(".gorp-action-menu-item").length || 0;
        return section?.dataset.open === "true" && toggle?.getAttribute("aria-expanded") === "true" && itemCount > 0 && rect && rect.width > 0 && rect.height > 0 && style?.display !== "none"
          ? { open: section.dataset.open, expanded: toggle.getAttribute("aria-expanded"), item_count: itemCount, width: Math.round(rect.width), height: Math.round(rect.height), display: style.display }
          : null;
      })()`, "default TS mobile form action menu opens");
      await evaluate(page, `document.body.click()`);
      const actionMenuOutsideClosedState = await waitFor(page, `(() => {
        const section = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .gorp-action-menu-section[data-menu='action']");
        const toggle = section?.querySelector(".gorp-action-menu-toggle");
        return section?.dataset.open === "false" && toggle?.getAttribute("aria-expanded") === "false"
          ? { open: section.dataset.open, expanded: toggle.getAttribute("aria-expanded") }
          : null;
      })()`, "default TS mobile form action menu closes by outside click");
      await clickSelector(page, ".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .o_control_panel_actions .gorp-action-menu-toggle[data-action-menu-toggle='action']");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .gorp-action-menu-section[data-menu='action']")?.dataset.open === "true"`, "default TS mobile form action menu reopens");
      await evaluate(page, `document.dispatchEvent(new KeyboardEvent("keydown", {key: "Escape", bubbles: true}))`);
      const actionMenuEscapeClosedState = await waitFor(page, `(() => {
        const section = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-view='form'] .gorp-action-menu-section[data-menu='action']");
        const toggle = section?.querySelector(".gorp-action-menu-toggle");
        return section?.dataset.open === "false" && toggle?.getAttribute("aria-expanded") === "false"
          ? { open: section.dataset.open, expanded: toggle.getAttribute("aria-expanded") }
          : null;
      })()`, "default TS mobile form action menu closes by document Escape");
      const hash = await waitFor(page, `(() => {
        const hash = window.location.hash || "";
        return hash.includes("model=ir.actions.server") && hash.includes("view_type=form") && hash.includes("id=") ? hash : "";
      })()`, "default TS mobile form hash");
      const overflow = await evaluate(page, `document.documentElement.scrollWidth - window.innerWidth`);
      if (overflow > 1) throw new Error(`default TS mobile action horizontal overflow: ${overflow}px`);
      return { card_count: cardCount, card_state: cardState, form_count: formCount, breadcrumb_count: breadcrumbCount, sheet_count: sheetCount, form_control_state: formControlState, mobile_form_chrome: mobileFormChrome, mobile_form_visual_state: mobileFormVisualState, action_menu_open_state: actionMenuOpenState, action_menu_outside_closed_state: actionMenuOutsideClosedState, action_menu_escape_closed_state: actionMenuEscapeClosedState, hash, horizontal_overflow_px: overflow };
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
    name: "default-apps-catalog-parity-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await openDefaultAppsCatalog(page, config, desktopViewport());
      const cardCount = await waitForCount(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='kanban'] .gorp-apps-catalog-card.o_kanban_record", 30, "Apps catalog kanban cards");
      const state = await evaluate(page, `(() => {
        const action = document.querySelector(".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='kanban']");
        const catalog = action?.querySelector(".gorp-apps-catalog");
        const sidebar = action?.querySelector(".gorp-apps-catalog-sidebar.o_search_panel");
        const renderer = action?.querySelector(".o_kanban_renderer");
        const cards = [...(renderer?.querySelectorAll(".o_kanban_record") || [])];
        const firstRect = cards[0]?.getBoundingClientRect();
        const firstStyle = cards[0] ? getComputedStyle(cards[0]) : null;
        const secondRect = cards[1]?.getBoundingClientRect();
        const firstText = cards[0]?.textContent || "";
        const visible = (node) => {
          const rect = node.getBoundingClientRect();
          return rect.bottom > 104 && rect.top < window.innerHeight && rect.right > 0 && rect.left < window.innerWidth;
        };
        return {
          window_count: action ? 1 : 0,
          catalog_total: catalog?.dataset.catalogTotal || "",
          catalog_visible_count: catalog?.dataset.visibleCount || "",
          renderer_count: renderer ? 1 : 0,
          brand: document.querySelector(".o_web_client .o_menu_brand")?.textContent?.trim() || "",
          navbar_sections: [...document.querySelectorAll(".o_web_client .o_navbar_sections .o_nav_entry")].map((node) => node.textContent.trim()).filter(Boolean),
          pager: action?.querySelector(".o_pager")?.textContent?.trim() || "",
          create_button_count: action?.querySelectorAll("[data-create-action='true']").length || 0,
          top_actions: [...(action?.querySelectorAll("[data-catalog-action]") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          sidebar_count: sidebar ? 1 : 0,
          sidebar_labels: [...(sidebar?.querySelectorAll(".o_search_panel_label") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          first_cards: cards.slice(0, 24).map((card) => card.querySelector(".o_kanban_record_title, .o_app_name")?.textContent?.trim() || ""),
          first_card_actions: [...(cards[0]?.querySelectorAll(".o_module_actions button, .o_module_info_button") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          first_card_technical_code: cards[0]?.querySelector(".o_module_technical_name")?.textContent?.trim() || "",
          first_card_image_count: cards[0]?.querySelectorAll(".o_module_icon_container .o_module_icon").length || 0,
          first_card_text: firstText.replace(/\\s+/g, " ").trim(),
          first_card_width: Math.round(firstRect?.width || 0),
          first_card_height: Math.round(firstRect?.height || 0),
          first_card_display: firstStyle?.display || "",
          second_card_top_delta: secondRect && firstRect ? Math.round(secondRect.top - firstRect.top) : 0,
          first_icon_width: Math.round(cards[0]?.querySelector(".o_module_icon")?.getBoundingClientRect()?.width || 0),
          first_icon_height: Math.round(cards[0]?.querySelector(".o_module_icon")?.getBoundingClientRect()?.height || 0),
          visible_card_count: cards.filter(visible).length,
          generic_field_count: cards.slice(0, 24).reduce((sum, card) => sum + card.querySelectorAll(".o_kanban_record_field").length, 0),
          module_card_count: cards.filter((card) => card.classList.contains("gorp-apps-catalog-card") && card.dataset.moduleName).length,
          module_state_count: cards.filter((card) => card.querySelector(".o_module_state")).length,
          install_button_count: cards.filter((card) => card.querySelector(".o_module_install_button, .o_module_upgrade_button, .o_module_uninstall_button, .o_module_cancel_button")).length,
          activate_button_count: cards.filter((card) => card.querySelector(".o_module_install_button")).length,
          learn_more_count: cards.filter((card) => [...card.querySelectorAll(".o_module_info_button")].some((node) => node.textContent.trim() === "Learn More")).length,
          info_button_count: cards.filter((card) => card.querySelector(".o_module_info_button")).length,
          generated_icon_count: cards.filter((card) => card.querySelector("img.o_module_icon[data-generated-icon='clean-room']")).length
        };
      })()`);
      const expectedSections = ["Apps"];
      if (JSON.stringify(state.navbar_sections) !== JSON.stringify(expectedSections)) throw new Error(`Apps catalog navbar invalid: ${JSON.stringify(state)}`);
      const expectedSidebar = ["All", "Official Apps", "Industries", "All", "Sales", "Website", "Services"];
      if (
        state.window_count !== 1 ||
        state.renderer_count !== 1 ||
        state.brand !== "Apps" ||
        state.catalog_total !== "77" ||
        state.catalog_visible_count !== "77" ||
        !state.pager.includes("1-77") ||
	        state.create_button_count !== 0 ||
	        JSON.stringify(state.top_actions) !== JSON.stringify(["Update Apps List", "Apply Scheduled Upgrades", "Import Module"]) ||
	        state.sidebar_count !== 1 ||
	        JSON.stringify(state.sidebar_labels.slice(0, expectedSidebar.length)) !== JSON.stringify(expectedSidebar) ||
	        !state.sidebar_labels.includes("Technical") ||
        state.module_card_count !== 77 ||
        state.module_state_count !== 77 ||
        state.install_button_count < 60 ||
        state.activate_button_count < 60 ||
        state.learn_more_count < 60 ||
        state.info_button_count !== 77 ||
        state.generated_icon_count !== 77 ||
        state.first_card_technical_code !== "sale_management" ||
        state.first_card_image_count !== 1 ||
        state.generic_field_count !== 0 ||
        state.first_cards[0] !== "Sales" ||
        JSON.stringify(state.first_card_actions) !== JSON.stringify(["Activate", "Learn More"]) ||
        state.first_card_text.includes("Technical Name") ||
        state.first_card_text.includes("State") ||
        state.first_card_width < 250 ||
        state.first_card_height < 92 ||
        state.first_card_height > 96 ||
        state.first_icon_width !== 50 ||
        state.first_icon_height !== 50 ||
        state.second_card_top_delta !== 0 ||
        state.visible_card_count < 12
      ) throw new Error(`Apps catalog parity invalid: ${JSON.stringify(state)}`);
      return { card_count: cardCount, ...state };
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
      await setInput(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", "install");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Apps", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Apps catalog action ready");
      const catalogCount = await waitForCount(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='kanban'] .o_kanban_renderer", 1, "TS Apps kanban catalog");
      await setInput(page, ".o_web_client .o_action_manager .o_searchview_input", "ai");
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
      await openDefaultAppsCatalog(page, config, desktopViewport(), "apps_catalog_detail");
      const cardCount = await waitForCount(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='equity']", 1, "Equity module card for detail");
      await clickSelector(page, ".o_web_client .gorp-apps-catalog-card[data-module-name='equity'] .o_module_info_button");
      const formCount = await waitForCount(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form'][data-module-info-action='equity']", 1, "Apps Module Info action form");
      const tabCount = await waitForCount(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form'] .gorp-module-info-notebook .gorp-form-notebook-tab", 2, "Apps Module Info tabs");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form']");
        const moduleBody = root?.querySelector(".gorp-module-info-body");
        return {
          breadcrumb: [...(root?.querySelectorAll(".o_breadcrumb .breadcrumb-item") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          tabs: [...(root?.querySelectorAll(".gorp-module-info-notebook .gorp-form-notebook-tab") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          author: root?.querySelector(".o_module_author")?.textContent?.trim() || "",
          activate_count: root?.querySelectorAll(".o_module_install_button").length || 0,
          activate_in_sheet_count: root?.querySelectorAll(".gorp-module-info-title .o_module_install_button").length || 0,
          activate_in_control_panel_count: root?.querySelectorAll(".gorp-module-info-control-panel .o_module_install_button").length || 0,
          control_panel_rect: (() => {
            const rect = root?.querySelector(".gorp-module-info-control-panel")?.getBoundingClientRect?.();
            return { y: Math.round(rect?.y || 0), height: Math.round(rect?.height || 0) };
          })(),
          breadcrumb_rect: (() => {
            const rect = root?.querySelector(".gorp-module-info-control-panel .o_breadcrumb")?.getBoundingClientRect?.();
            return { y: Math.round(rect?.y || 0), height: Math.round(rect?.height || 0) };
          })(),
          pager_rect: (() => {
            const rect = root?.querySelector(".gorp-module-info-control-panel .o_pager")?.getBoundingClientRect?.();
            return { y: Math.round(rect?.y || 0), height: Math.round(rect?.height || 0) };
          })(),
          body_rect: (() => {
            const rect = moduleBody?.getBoundingClientRect?.();
            return { width: Math.round(rect?.width || 0), x: Math.round(rect?.x || 0) };
          })(),
          sheet_rect: (() => {
            const rect = root?.querySelector(".gorp-module-info-sheet")?.getBoundingClientRect?.();
            return { width: Math.round(rect?.width || 0), x: Math.round(rect?.x || 0) };
          })(),
          control_panel_height: Math.round(root?.querySelector(".gorp-module-info-control-panel")?.getBoundingClientRect?.().height || 0),
          dialog_count: document.querySelectorAll(".o_web_client .gorp-action-dialog[data-model='ir.module.module']").length || 0,
          icon_count: root?.querySelectorAll(".gorp-module-info-icon[alt='Binary file']").length || 0,
          icon_rect: (() => {
            const rect = root?.querySelector(".gorp-module-info-icon")?.getBoundingClientRect?.();
            return { width: Math.round(rect?.width || 0), height: Math.round(rect?.height || 0) };
          })(),
          information_labels: [...(root?.querySelectorAll(".gorp-form-notebook-page[data-page='information'] .o_form_label") || [])].map((node) => node.textContent.trim().replace(/\\?$/, "")).filter(Boolean),
          information_values: [...(root?.querySelectorAll(".gorp-form-notebook-page[data-page='information'] .o_field_widget") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          text: moduleBody?.textContent || ""
        };
      })()`);
      if (!state.breadcrumb.includes("Apps") || !state.breadcrumb.includes("Equity") || JSON.stringify(state.tabs) !== JSON.stringify(["Information", "Technical Data"]) || state.author !== "By Odoo S.A." || state.activate_count !== 1 || state.activate_in_sheet_count !== 1 || state.activate_in_control_panel_count !== 0 || state.control_panel_rect.y < 44 || state.breadcrumb_rect.y < state.control_panel_rect.y || state.pager_rect.y < state.control_panel_rect.y || state.body_rect.width < 1200 || state.sheet_rect.width < 1180 || state.sheet_rect.x > 24 || state.control_panel_height > 90 || state.dialog_count !== 0 || state.icon_count !== 1 || state.icon_rect.width !== 88 || state.icon_rect.height !== 88 || JSON.stringify(state.information_labels) !== JSON.stringify(["Category", "Technical Name", "License", "Latest Version"]) || !state.information_values.includes("Invoicing") || !state.information_values.includes("equity") || !state.information_values.includes("LGPL Version 3") || !state.information_values.includes("19.0.1.0") || state.text.includes("Summary") || state.text.includes("Description") || state.text.includes("Website")) {
        throw new Error(`Apps Module Info action form invalid: ${JSON.stringify(state)}`);
      }
      await clickExactText(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form'] .gorp-form-notebook-tab", "Technical Data");
      const technicalState = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form']");
        return {
          selected_tab: root?.querySelector(".gorp-module-info-notebook .gorp-form-notebook-tab[aria-selected='true']")?.textContent?.trim() || "",
          labels: [...(root?.querySelectorAll(".gorp-form-notebook-page:not([hidden]) .o_form_label") || [])].map((node) => node.textContent.trim().replace(/\\?$/, "")).filter(Boolean),
          checkbox_state: [...(root?.querySelectorAll(".gorp-form-notebook-page:not([hidden]) input[type='checkbox']") || [])].map((node) => ({ field: node.dataset.field || "", checked: node.checked, disabled: node.disabled })),
          table_titles: [...(root?.querySelectorAll(".gorp-form-notebook-page:not([hidden]) .gorp-module-info-relation-title") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          dependency_rows: [...(root?.querySelectorAll(".gorp-form-notebook-page:not([hidden]) [data-module-info-table='dependencies'] tbody tr") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          exclusion_table_count: root?.querySelectorAll(".gorp-form-notebook-page:not([hidden]) [data-module-info-table='exclusions']").length || 0,
          text: root?.querySelector(".gorp-form-notebook-page:not([hidden])")?.textContent || ""
        };
      })()`);
      for (const label of ["Demo Data", "Application", "Status"]) {
        if (!technicalState.labels.includes(label)) throw new Error(`Apps Module Info Technical Data missing ${label}: ${JSON.stringify(technicalState)}`);
      }
      if (JSON.stringify(technicalState.checkbox_state) !== JSON.stringify([{ field: "demo-data", checked: false, disabled: true }, { field: "application", checked: true, disabled: true }]) || JSON.stringify(technicalState.table_titles) !== JSON.stringify(["DEPENDENCIES", "EXCLUSIONS"]) || !technicalState.dependency_rows.some((row) => row.includes("portal") && row.includes("Not Installed")) || technicalState.exclusion_table_count !== 1) {
        throw new Error(`Apps Module Info Technical Data structure invalid: ${JSON.stringify(technicalState)}`);
      }
      await delay(250);
      await waitFor(page, `document.querySelector(".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='form'][data-module-info-action='equity'] .gorp-form-notebook-tab[aria-selected='true']")?.textContent?.trim() === "Technical Data"`, "Apps Module Info final screenshot state");
      return { module: "equity", card_count: cardCount, form_count: formCount, tab_count: tabCount, state, technical_state: technicalState };
    }
  },
  {
    name: "default-users-list-parity-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&users_list=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "users list TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", "Users");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Users", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Users list action ready");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.users'][data-view='list']");
        const list = root?.querySelector(".gorp-list-view");
        return {
          headers: [...(list?.querySelectorAll(".o_list_header_button") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          row_count: list?.querySelectorAll("tbody tr.o_data_row").length || 0,
          pager: document.querySelector(".o_web_client .o_control_panel_navigation .o_pager")?.textContent?.trim() || "",
          chips: [...document.querySelectorAll(".o_web_client .o_action_manager .o_searchview_facet .o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean),
          role_badge: list?.querySelector(".gorp-user-role-badge")?.textContent?.trim() || "",
          avatar_count: list?.querySelectorAll(".gorp-user-avatar.o_avatar").length || 0,
          avatar_header_text: list?.querySelector("th[data-name='avatar_128'] .o_list_header_button")?.textContent?.trim() || "",
          avatar_header_label: list?.querySelector("th[data-name='avatar_128'] .o_list_header_button")?.getAttribute("aria-label") || "",
          avatar_cell_label: list?.querySelector("tbody tr.o_data_row td[data-field='avatar_128']")?.getAttribute("aria-label") || "",
          avatar_image_label: list?.querySelector(".gorp-user-list-avatar-cell")?.getAttribute("aria-label") || "",
          text: list?.textContent || ""
        };
      })()`);
      if (JSON.stringify(state.headers) !== JSON.stringify(["Name", "Login", "Role"]) || state.row_count !== 1 || !state.chips.includes("Internal Users") || state.role_badge !== "Administrator" || state.avatar_count < 1 || state.avatar_header_text !== "" || state.avatar_header_label !== "" || state.avatar_cell_label !== "Binary file" || state.avatar_image_label !== "Binary file" || !state.text.includes("Administrator")) {
        throw new Error(`Users list parity invalid: ${JSON.stringify(state)}`);
      }
      return state;
    }
  },
  {
    name: "default-groups-list-parity-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&groups_list=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "groups list TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", "Groups");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Groups", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Groups list action ready");
      const state = await evaluate(page, `(() => {
        const root = document.querySelector(".o_web_client .o_action_manager .gorp-window-action[data-model='res.groups'][data-view='list']");
        const list = root?.querySelector(".gorp-list-view");
        return {
          headers: [...(list?.querySelectorAll(".o_list_header_button") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          row_count: list?.querySelectorAll("tbody tr.o_data_row").length || 0,
          pager: document.querySelector(".o_web_client .o_control_panel_navigation .o_pager")?.textContent?.trim() || "",
          chips: [...document.querySelectorAll(".o_web_client .o_action_manager .o_searchview_facet .o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean),
          text: list?.textContent || ""
        };
      })()`);
      if (JSON.stringify(state.headers) !== JSON.stringify(["Privilege", "Name"]) || state.row_count !== 13 || !state.chips.includes("Internal Groups") || !state.text.includes("Technical Documentation")) {
        throw new Error(`Groups list parity invalid: ${JSON.stringify(state)}`);
      }
      return state;
    }
  },
  {
    name: "default-users-flow-desktop",
    viewport: { width: 1366, height: 900, mobile: false },
    run: async (page, config) => {
      await setViewport(page, desktopViewport());
      await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&users_flow=1`) });
      await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "users flow TS webclient ready");
      await setInput(page, ".o_web_client .o_home_menu input[aria-label='Search apps and menus']", "Users");
      await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Users", ".o_app_name");
      await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Users action ready");
      const listRows = await waitForCount(page, ".o_web_client .o_data_row", 1, "Users list rows");
      const listState = await evaluate(page, `(() => {
        const list = document.querySelector(".o_web_client .gorp-list-view");
        return {
          headers: [...(list?.querySelectorAll(".o_list_header_button") || [])].map((node) => node.textContent.trim()).filter(Boolean),
          text: list?.textContent || "",
          role_badge: list?.querySelector(".gorp-user-role-badge")?.textContent?.trim() || "",
          avatar_count: list?.querySelectorAll(".gorp-user-avatar.o_avatar").length || 0,
          avatar_header_text: list?.querySelector("th[data-name='avatar_128'] .o_list_header_button")?.textContent?.trim() || "",
          avatar_header_label: list?.querySelector("th[data-name='avatar_128'] .o_list_header_button")?.getAttribute("aria-label") || "",
          avatar_cell_label: list?.querySelector("tbody tr.o_data_row td[data-field='avatar_128']")?.getAttribute("aria-label") || "",
          avatar_image_label: list?.querySelector(".gorp-user-list-avatar-cell")?.getAttribute("aria-label") || ""
        };
      })()`);
      if (JSON.stringify(listState.headers) !== JSON.stringify(["Name", "Login", "Role"]) || listState.role_badge !== "Administrator" || listState.avatar_count < 1 || listState.avatar_header_text !== "" || listState.avatar_header_label !== "" || listState.avatar_cell_label !== "Binary file" || listState.avatar_image_label !== "Binary file" || (!listState.text.includes("Administrator") && !listState.text.includes("admin"))) {
        throw new Error(`Users list invalid: ${JSON.stringify(listState)}`);
      }
      await clickSelector(page, ".o_web_client .o_data_row");
      await waitForCount(page, ".o_web_client .gorp-form-view[data-model='res.users']", 1, "Users form");
      const formState = await evaluate(page, `(() => {
        const form = document.querySelector(".o_web_client .gorp-form-view[data-model='res.users']");
        const labels = [...(form?.querySelectorAll(".o_form_label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
        const notebooks = [...(form?.querySelectorAll(".gorp-form-notebook") || [])];
        const accessNotebook = notebooks.find((notebook) => [...notebook.querySelectorAll(".gorp-form-notebook-tab[role='tab']")].some((node) => node.textContent.trim() === "Access Rights"));
	        const accessTabs = [...(accessNotebook?.querySelectorAll(".gorp-form-notebook-tab[role='tab']") || [])].map((node) => node.textContent.trim()).filter(Boolean);
	        const groupWidget = accessNotebook?.querySelector(".gorp-res-user-group-ids[data-field='group_ids']");
	        const accessSections = [...(groupWidget?.querySelectorAll(".gorp-res-user-access-section h2") || [])].map((node) => node.textContent.trim()).filter(Boolean);
	        const groupWidgetStyle = groupWidget ? getComputedStyle(groupWidget) : null;
	        const sheet = form?.querySelector(".gorp-form-sheet.o_form_sheet");
	        const boxedAccessChrome = Boolean(groupWidgetStyle && (
	          parseFloat(groupWidgetStyle.borderTopWidth || "0") > 0 ||
	          parseFloat(groupWidgetStyle.borderRightWidth || "0") > 0 ||
	          parseFloat(groupWidgetStyle.borderBottomWidth || "0") > 0 ||
	          parseFloat(groupWidgetStyle.borderLeftWidth || "0") > 0
	        ));
		        const identity = form?.querySelector(".gorp-user-identity.o_user_identity_block");
		        const smartButtons = [...(form?.querySelectorAll(".gorp-access-smart-button") || [])].map((node) => node.textContent.trim()).filter(Boolean);
		        const identityLines = [...(identity?.querySelectorAll(".gorp-user-identity-inline") || [])].map((node) => ({
		          field: node.dataset.field || "",
		          text: node.querySelector("input")?.value || node.querySelector("input")?.placeholder || node.textContent.trim(),
		          label: node.getAttribute("aria-label") || "",
		          icon: node.querySelector("i")?.className || ""
		        }));
		        const verticalIdentityLabels = [...(identity?.querySelectorAll(".gorp-user-identity-label") || [])].map((node) => node.textContent.trim()).filter(Boolean);
		        const text = form?.textContent || "";
        return {
          has_form: Boolean(form),
          labels,
          access_notebook: accessNotebook?.dataset?.notebook || "",
          access_tabs: accessTabs,
          has_access_notebook: Boolean(accessNotebook),
          has_group_widget: Boolean(groupWidget),
	          group_widget_role: groupWidget?.dataset?.role || "",
	          access_sections: accessSections,
	          role_radios: [...(groupWidget?.querySelectorAll("input[name='res-user-role']") || [])].map((node) => node.value),
	          role_options: [...(groupWidget?.querySelectorAll(".gorp-res-user-role-option") || [])].map((node) => node.textContent.trim()).filter(Boolean),
	          access_select_count: groupWidget?.querySelectorAll(".gorp-res-user-access-select").length || 0,
	          master_data_rows: groupWidget?.querySelectorAll(".gorp-res-user-access-master-data .gorp-res-user-group-privilege").length || 0,
	          master_data_values: [...(groupWidget?.querySelectorAll(".gorp-res-user-access-master-data .gorp-res-user-group-privilege") || [])].map((row) => ({
	            label: row.querySelector(".gorp-res-user-access-label")?.textContent?.replace("?", "").trim() || "",
	            value: row.querySelector(".gorp-res-user-access-select")?.selectedOptions?.[0]?.textContent?.trim() || "",
	            style: (() => {
	              const select = row.querySelector(".gorp-res-user-access-select");
	              const computed = select ? getComputedStyle(select) : null;
	              return {
	                bg: computed?.backgroundColor || "",
	                color: computed?.color || "",
	                width: Math.round(select?.getBoundingClientRect?.().width || 0)
	              };
	            })()
	          })),
	          extra_right_rows: groupWidget?.querySelectorAll(".gorp-res-user-access-extra-rights .gorp-res-user-group-option").length || 0,
	          boxed_access_chrome: boxedAccessChrome,
	          access_width_ratio: groupWidget && sheet ? Number((groupWidget.getBoundingClientRect().width / sheet.getBoundingClientRect().width).toFixed(2)) : 0,
		          has_identity_block: Boolean(identity),
		          add_photo: identity?.querySelector(".gorp-user-add-photo")?.textContent?.trim() || "",
		          identity_lines: identityLines,
		          vertical_identity_labels: verticalIdentityLabels,
		          action_menu_count: document.querySelectorAll(".o_web_client .o_action_manager .gorp-window-action[data-model='res.users'][data-view='form'] .gorp-form-action-menu, .o_web_client .o_control_panel .gorp-form-action-menu[data-model='res.users']").length,
	          identity_title: identity?.querySelector(".gorp-form-title")?.textContent?.trim() || "",
          related_partner: identity?.querySelector(".gorp-user-related-partner")?.textContent?.trim() || "",
          smart_buttons: smartButtons,
          has_identity_value: text.includes("Administrator") || text.includes("admin")
        };
      })()`);
      for (const tab of ["Access Rights", "Preferences", "Calendar", "Security"]) {
        if (!formState.access_tabs.includes(tab)) throw new Error(`Users form missing tab ${tab}: ${JSON.stringify(formState)}`);
      }
      for (const smart of ["Groups", "Access Rights", "Record Rules"]) {
        if (!formState.smart_buttons.some((item) => item.includes(smart))) throw new Error(`Users form missing smart button ${smart}: ${JSON.stringify(formState)}`);
      }
      for (const section of ["Roles", "Master Data", "Extra Rights"]) {
        if (!formState.access_sections.includes(section)) throw new Error(`Users form missing access section ${section}: ${JSON.stringify(formState)}`);
      }
	      if (JSON.stringify(formState.role_radios) !== JSON.stringify(["group_user", "group_system"]) || JSON.stringify(formState.role_options) !== JSON.stringify(["User", "Administrator"]) || formState.access_select_count < 1 || formState.master_data_rows < 1 || formState.extra_right_rows < 6 || formState.boxed_access_chrome || formState.access_width_ratio < 0.74) {
	        throw new Error(`Users access layout invalid: ${JSON.stringify(formState)}`);
	      }
	      const masterValues = new Map(formState.master_data_values.map((item) => [item.label, item.value]));
	      if (masterValues.get("Contact") !== "Creation" || masterValues.get("Export") !== "Allowed") {
	        throw new Error(`Users master data access values invalid: ${JSON.stringify(formState)}`);
	      }
	      if (formState.master_data_values.some((item) => item.style.bg !== "rgb(255, 255, 255)" || item.style.color !== "rgb(31, 41, 51)" || item.style.width < 320)) {
	        throw new Error(`Users master data controls invalid: ${JSON.stringify(formState)}`);
	      }
	      if (!formState.has_form || !formState.has_identity_block || formState.identity_title !== "Administrator" || !formState.has_identity_value || !formState.has_access_notebook || !formState.has_group_widget) {
	        throw new Error(`Users form invalid: ${JSON.stringify(formState)}`);
	      }
	      const identityByField = new Map(formState.identity_lines.map((item) => [item.field, item]));
	      if (formState.add_photo !== "Add Photo" || formState.vertical_identity_labels.length || identityByField.get("login")?.text !== "admin" || identityByField.get("email")?.label !== "Email" || !identityByField.get("email")?.icon?.includes("oi-envelope-closed") || identityByField.get("phone")?.label !== "Phone" || !identityByField.get("phone")?.icon?.includes("oi-phone") || formState.action_menu_count < 1) {
	        throw new Error(`Users identity/action chrome invalid: ${JSON.stringify(formState)}`);
	      }
      return { list_rows: listRows, list_state: listState, form_state: formState };
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
      list_header_bg: styleValue(".o_web_client .o_action_manager .gorp-list-view th", "background-color"),
      list_headers: [...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view th")]
        .slice(0, 5)
        .map((node) => node.textContent.replace(/\\s+/g, " ").trim()),
      body_action_toolbar_count: document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-shell > .gorp-list-toolbar").length,
      control_panel_action_toolbar_count: document.querySelectorAll(".o_web_client .o_action_manager .o_control_panel_main_buttons .gorp-list-toolbar").length
    };
  })()`);
  const issues = [];
  const acceptedControlPanelBG = new Set(["rgb(38, 42, 54)"]);
  const acceptedListHeaderBG = new Set(["rgb(27, 29, 39)"]);
  if (!acceptedControlPanelBG.has(snapshot.control_panel_bg)) issues.push(`control panel bg ${snapshot.control_panel_bg}`);
  if (snapshot.control_panel_bg === "rgb(255, 255, 255)" && (!snapshot.control_panel_shadow || snapshot.control_panel_shadow === "none")) issues.push("control panel shadow missing");
  if (snapshot.control_panel_min_height_px < 60 || snapshot.control_panel_min_height_px > 66) issues.push(`control panel min-height ${snapshot.control_panel_min_height_px}`);
  if (snapshot.search_width_px < 400 || snapshot.search_width_px > 450) issues.push(`search width ${snapshot.search_width_px}`);
  if (snapshot.search_radius_px !== 4) issues.push(`search radius ${snapshot.search_radius_px}`);
  if (!acceptedListHeaderBG.has(snapshot.list_header_bg)) issues.push(`list header bg ${snapshot.list_header_bg}`);
  if (JSON.stringify(snapshot.list_headers) !== JSON.stringify(["", "Name", "Model", "Type", "Usage"])) issues.push(`list headers ${JSON.stringify(snapshot.list_headers)}`);
  if (snapshot.body_action_toolbar_count !== 0) issues.push(`body action toolbar count ${snapshot.body_action_toolbar_count}`);
  if (snapshot.control_panel_action_toolbar_count < 1) issues.push(`control panel action toolbar count ${snapshot.control_panel_action_toolbar_count}`);
  if (issues.length) throw new Error(`enterprise polish style audit failed: ${issues.join("; ")}`);
  return snapshot;
}

async function assertLauncherSearchActivation(page) {
  const snapshot = await evaluate(page, `(() => {
    const root = document.querySelector(".o_web_client .o-app-launcher-view");
    const input = document.querySelector(".o_web_client .o_home_menu input[aria-label='Search apps and menus']");
    const wrap = document.querySelector(".o_web_client .o_home_menu .o_home_menu_search");
    if (!root || !input || !wrap) return { ok: false, reason: "missing launcher search nodes" };
    root.focus?.();
    root.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true, cancelable: true }));
    const activeStyle = getComputedStyle(wrap);
    const activeRect = input.getBoundingClientRect();
    const visibleAppCount = document.querySelectorAll(".o_web_client .o_home_menu .o_app").length;
    const activeDataset = wrap.dataset.searchActive || "";
    const activeValue = input.value;
    const active = activeDataset === "true" && activeValue === "a" && activeRect.width >= 300 && activeRect.height >= 30 && visibleAppCount >= 1;
    input.value = "";
    input.dispatchEvent(new Event("input", { bubbles: true, cancelable: true }));
    const idleStyle = getComputedStyle(wrap);
    const idle = wrap.dataset.searchActive === "false" && Number.parseFloat(idleStyle.maxHeight) === 0 && Number.parseFloat(idleStyle.marginBottom) <= 1;
    return {
      ok: active && idle,
      active,
      idle,
      active_dataset: activeDataset,
      active_value: activeValue,
      active_width_px: Math.round(activeRect.width),
      active_height_px: Math.round(activeRect.height),
      active_max_height_px: Math.round(Number.parseFloat(activeStyle.maxHeight) || 0),
      idle_margin_bottom_px: Math.round(Number.parseFloat(idleStyle.marginBottom) || 0),
      visible_app_count: visibleAppCount
    };
  })()`);
  if (!snapshot.ok) throw new Error(`launcher search activation invalid: ${JSON.stringify(snapshot)}`);
  return snapshot;
}

async function assertLegacyLauncherChromeSnapshot(page) {
  const snapshot = await evaluate(page, `(() => {
    const header = document.querySelector("body[data-view='apps'] > .o_navbar");
    const navbar = document.querySelector("body[data-view='apps'] > .o_navbar > .o_main_navbar");
    const launcher = document.querySelector("body[data-view='apps'] #appsView.o-app-launcher-view");
    const grid = document.querySelector("body[data-view='apps'] #appGrid.o_apps");
    const headerStyle = header ? getComputedStyle(header) : null;
    const navbarStyle = navbar ? getComputedStyle(navbar) : null;
    const launcherRect = launcher?.getBoundingClientRect();
    const gridRect = grid?.getBoundingClientRect();
    const launcherStyle = launcher ? getComputedStyle(launcher) : null;
    const firstIcon = document.querySelector("body[data-view='apps'] #appGrid .o_app .o_app_icon");
    const firstIconRect = firstIcon?.getBoundingClientRect();
    return {
      header_position: headerStyle?.position || "",
      header_bg: headerStyle?.backgroundColor || "",
      navbar_bg: navbarStyle?.backgroundColor || "",
      launcher_bg: launcherStyle?.backgroundColor || "",
      launcher_bg_image: launcherStyle?.backgroundImage || "",
      launcher_top_px: launcherRect ? Math.round(launcherRect.top) : -1,
      grid_top_px: gridRect ? Math.round(gridRect.top) : -1,
      icon_width_px: firstIconRect ? Math.round(firstIconRect.width) : 0,
      icon_height_px: firstIconRect ? Math.round(firstIconRect.height) : 0,
      icon_text: firstIcon?.textContent?.trim() || "",
      generic_card_count: document.querySelectorAll("body[data-view='apps'] #appGrid .app-card").length
    };
  })()`);
  const transparent = new Set(["rgba(0, 0, 0, 0)", "transparent"]);
  const issues = [];
  if (snapshot.header_position !== "absolute") issues.push(`header position ${snapshot.header_position}`);
  if (!transparent.has(snapshot.header_bg)) issues.push(`header background ${snapshot.header_bg}`);
  if (!transparent.has(snapshot.navbar_bg)) issues.push(`navbar background ${snapshot.navbar_bg}`);
  if (!isDarkLauncherBackground(snapshot.launcher_bg)) issues.push(`launcher background is not dark ${snapshot.launcher_bg}`);
  if (!isEnterpriseHomeBackgroundImage(snapshot.launcher_bg_image)) issues.push(`enterprise background image missing ${snapshot.launcher_bg_image}`);
  if (snapshot.launcher_top_px > 1 || snapshot.launcher_top_px < 0) issues.push(`launcher top ${snapshot.launcher_top_px}`);
  if (snapshot.grid_top_px < 145 || snapshot.grid_top_px > 250) issues.push(`grid top ${snapshot.grid_top_px}`);
  if (snapshot.icon_width_px < 66 || snapshot.icon_width_px > 74 || snapshot.icon_height_px < 66 || snapshot.icon_height_px > 74) issues.push(`icon size ${snapshot.icon_width_px}x${snapshot.icon_height_px}`);
  if (snapshot.icon_text) issues.push(`synthetic icon text ${snapshot.icon_text}`);
  if (snapshot.generic_card_count) issues.push(`generic card count ${snapshot.generic_card_count}`);
  if (issues.length) throw new Error(`legacy launcher chrome audit failed: ${issues.join("; ")}`);
  return snapshot;
}

async function assertAppsCatalogIconState(page) {
  const snapshot = await evaluate(page, `(() => {
    const icons = [...document.querySelectorAll(".o_web_client .gorp-apps-catalog-card .o_app_icon")];
    const generatedIcons = icons.filter((icon) => icon.matches("img.o_module_icon[data-generated-icon='clean-room']"));
    const first = icons[0];
    const rect = first?.getBoundingClientRect();
    return {
      count: icons.length,
      image_count: icons.filter((icon) => icon.matches("img.o_module_icon")).length,
      generated_count: generatedIcons.length,
      text_count: icons.filter((icon) => icon.textContent.trim()).length,
      first_src_is_svg_data: String(first?.getAttribute("src") || "").startsWith("data:image/svg+xml"),
      first_width_px: rect ? Math.round(rect.width) : 0,
      first_height_px: rect ? Math.round(rect.height) : 0
    };
  })()`);
  if (snapshot.count < 1 || snapshot.image_count !== snapshot.count || snapshot.generated_count !== snapshot.count || snapshot.text_count || !snapshot.first_src_is_svg_data || snapshot.first_width_px !== 50 || snapshot.first_height_px !== 50) {
    throw new Error(`Apps catalog icons invalid: ${JSON.stringify(snapshot)}`);
  }
  return snapshot;
}

async function assertEnterpriseLauncherSnapshot(page) {
  const snapshot = await evaluate(page, `(() => {
    const navbar = document.querySelector(".o_navbar > .o_main_navbar");
    const webclient = document.querySelector(".o_web_client");
    const home = document.querySelector(".o_web_client .o_home_menu");
    const launcher = document.querySelector(".o_web_client .o-app-launcher-view");
	    const search = document.querySelector(".o_web_client .o_home_menu .o_home_menu_search");
	    const banner = document.querySelector(".o_web_client .o_home_menu_registration_banner");
	    const bannerClose = document.querySelector(".o_web_client .o_home_menu_registration_close");
	    const userName = document.querySelector(".o_web_client .o_user_menu_name");
	    const databaseName = document.querySelector(".o_web_client .o_database_name");
	    const card = document.querySelector(".o_web_client .o_home_menu .o_app");
	      const launcherIcons = [...document.querySelectorAll(".o_web_client .o_home_menu .o_app")].map((node) => {
	      const icon = node.querySelector(".o_app_icon");
	      const before = icon ? getComputedStyle(icon, "::before") : null;
	      const after = icon ? getComputedStyle(icon, "::after") : null;
	      return {
	        key: node.dataset.appKey || "",
	        kind: icon?.dataset?.iconKind || node.dataset.iconKind || "",
	        token: icon?.dataset?.iconToken || "",
	        generated: icon?.dataset?.generatedIcon || "",
	        src_prefix: icon?.getAttribute("src")?.slice(0, 32) || "",
	        before_bg: before?.backgroundImage || before?.backgroundColor || "",
	        after_bg: after?.backgroundImage || after?.backgroundColor || ""
	      };
	    });
	    const wrapper = document.querySelector(".o_web_client .o_home_menu .o_draggable");
	    const icon = card?.querySelector(".o_app_icon");
	    const hiddenSearch = document.querySelector(".o_web_client .o_home_menu .o_search_hidden");
	    const navbarStyle = navbar ? getComputedStyle(navbar) : null;
    const homeStyle = home ? getComputedStyle(home.closest(".o-app-launcher-view") || home) : null;
    const searchStyle = search ? getComputedStyle(search) : null;
    const bannerStyle = banner ? getComputedStyle(banner) : null;
    const bannerCloseStyle = bannerClose ? getComputedStyle(bannerClose) : null;
    const userNameStyle = userName ? getComputedStyle(userName) : null;
    const cardStyle = card ? getComputedStyle(card) : null;
    const iconStyle = icon ? getComputedStyle(icon) : null;
    const navbarRect = navbar?.getBoundingClientRect();
    const launcherRect = launcher?.getBoundingClientRect();
    const searchRect = search?.getBoundingClientRect();
	    const bannerRect = banner?.getBoundingClientRect();
	    const bannerCloseRect = bannerClose?.getBoundingClientRect();
	    const rect = card?.getBoundingClientRect();
	    const wrapperRect = wrapper?.getBoundingClientRect();
	    const iconRect = icon?.getBoundingClientRect();
	    return {
      navbar_height_px: navbarRect ? Math.round(navbarRect.height) : 0,
      navbar_contract: Boolean(document.querySelector(".o_navbar > .o_main_navbar")),
	      home_background_class: Boolean(webclient?.classList?.contains("o_home_menu_background")),
	      legacy_card_count: document.querySelectorAll(".o_web_client .o_home_menu .app-card").length,
	      draggable_count: document.querySelectorAll(".o_web_client .o_home_menu .o_draggable").length,
	      hidden_search_count: document.querySelectorAll(".o_web_client .o_home_menu .o_search_hidden").length,
	      menu_sections_contract: Boolean(document.querySelector(".o_web_client .o_navbar .o_menu_sections")),
	      mobile_toggle_contract: Boolean(document.querySelector(".o_web_client .o_navbar .o_mobile_menu_toggle")),
	      app_tag: card?.tagName || "",
	      app_href: card?.getAttribute("href") || "",
	      hidden_search_role: hiddenSearch?.getAttribute("role") || "",
	      navbar_bg: navbarStyle?.backgroundColor || "",
	      launcher_top_px: launcherRect ? Math.round(launcherRect.top) : -1,
      launcher_bg_color: homeStyle?.backgroundColor || "",
      launcher_bg_image: homeStyle?.backgroundImage || "",
      launcher_box_shadow: homeStyle?.boxShadow || "",
      search_height_px: searchRect ? Math.round(searchRect.height) : 0,
      search_margin_bottom_px: searchStyle ? Math.round(Number.parseFloat(searchStyle.marginBottom) || 0) : -1,
	      banner_visible: Boolean(banner) && !banner.hidden && bannerStyle?.display !== "none" && (bannerRect?.width || 0) > 400,
	      banner_count: document.querySelectorAll(".o_web_client .o_home_menu_registration_banner").length,
	      banner_text: banner?.textContent?.replace(/\\s+/g, " ").trim() || "",
	      banner_close_text: bannerClose?.textContent?.trim() || "",
	      banner_close_visible: Boolean(bannerClose) && bannerCloseStyle?.display !== "none" && bannerCloseStyle?.visibility !== "hidden" && (bannerCloseRect?.width || 0) >= 20 && (bannerCloseRect?.height || 0) >= 20,
	      banner_top_px: bannerRect ? Math.round(bannerRect.top) : 0,
	      banner_width_px: bannerRect ? Math.round(bannerRect.width) : 0,
		      launcher_mail_activity_visible_count: [...document.querySelectorAll(".o_web_client[data-view='apps'] .o_mail_systray_item, .o_web_client[data-view='apps'] .o_activity_menu")]
		        .filter((node) => {
		          const style = getComputedStyle(node);
		          const rect = node.getBoundingClientRect();
		          return style.display !== "none" && style.visibility !== "hidden" && rect.width > 0 && rect.height > 0;
		        }).length,
		      developer_tools_count: document.querySelectorAll(".o_web_client[data-view='apps'] .o_debug_manager[aria-label='Open developer tools']").length,
		      studio_count: document.querySelectorAll(".o_web_client[data-view='apps'] .o_web_studio_navbar_item[aria-label='Odoo Studio']").length,
	      user_name_display: userNameStyle?.display || "",
	      database_badge_text: databaseName?.textContent?.trim() || "",
	      database_badge_count: document.querySelectorAll(".o_web_client .o_database_name").length,
	      app_card_left_px: wrapperRect ? Math.round(wrapperRect.left) : rect ? Math.round(rect.left) : 0,
	      app_card_top_px: wrapperRect ? Math.round(wrapperRect.top) : rect ? Math.round(rect.top) : 0,
	      app_card_width_px: rect ? Math.round(rect.width) : 0,
      app_card_height_px: rect ? Math.round(rect.height) : 0,
      app_card_bg: cardStyle?.backgroundColor || "",
      app_card_border_color: cardStyle?.borderTopColor || "",
      app_card_border_radius_px: cardStyle ? Number.parseFloat(cardStyle.borderTopLeftRadius) || 0 : 0,
      app_icon_width_px: iconRect ? Math.round(iconRect.width) : 0,
      app_icon_height_px: iconRect ? Math.round(iconRect.height) : 0,
      app_icon_radius_px: iconStyle ? Number.parseFloat(iconStyle.borderTopLeftRadius) || 0 : 0,
      app_icon_text: icon?.textContent?.trim() || "",
      launcher_icons: launcherIcons
    };
  })()`);
  const issues = [];
  const transparent = new Set(["rgba(0, 0, 0, 0)", "transparent"]);
  if (!snapshot.navbar_contract) issues.push("navbar contract missing");
	  if (!snapshot.home_background_class) issues.push("home background class missing");
	  if (snapshot.legacy_card_count !== 0) issues.push(`legacy app-card count ${snapshot.legacy_card_count}`);
	  if (snapshot.draggable_count < 2) issues.push(`draggable wrapper count ${snapshot.draggable_count}`);
	  if (snapshot.hidden_search_count < 1 || snapshot.hidden_search_role !== "combobox") issues.push(`hidden search contract ${JSON.stringify({ count: snapshot.hidden_search_count, role: snapshot.hidden_search_role })}`);
	  if (!snapshot.menu_sections_contract) issues.push("menu sections contract missing");
	  if (!snapshot.mobile_toggle_contract) issues.push("mobile toggle contract missing");
	  if (snapshot.app_tag !== "A") issues.push(`app tag ${snapshot.app_tag}`);
	  if (!snapshot.app_href.startsWith("#menu_id=")) issues.push(`app href ${snapshot.app_href}`);
	  if (snapshot.navbar_height_px < 44 || snapshot.navbar_height_px > 48) issues.push(`navbar height ${snapshot.navbar_height_px}`);
	  if (!transparent.has(snapshot.navbar_bg)) issues.push(`navbar background ${snapshot.navbar_bg}`);
	  if (snapshot.launcher_top_px > 48 || snapshot.launcher_top_px < 0) issues.push(`launcher top ${snapshot.launcher_top_px}`);
		  if (!isDarkLauncherBackground(snapshot.launcher_bg_color)) issues.push(`launcher background is not dark ${snapshot.launcher_bg_color}`);
	  if (!isEnterpriseHomeBackgroundImage(snapshot.launcher_bg_image)) issues.push(`enterprise background image missing ${snapshot.launcher_bg_image}`);
	  if (snapshot.search_height_px > 1) issues.push(`idle search height ${snapshot.search_height_px}`);
	  if (snapshot.search_margin_bottom_px > 1) issues.push(`idle search margin ${snapshot.search_margin_bottom_px}`);
	  if (snapshot.banner_count !== 1 || !snapshot.banner_visible) issues.push(`registration banner missing ${JSON.stringify({ count: snapshot.banner_count, visible: snapshot.banner_visible })}`);
	  if (!snapshot.banner_text.includes("You will be able to register your database once you have installed your first app.")) issues.push(`registration banner text ${snapshot.banner_text}`);
		  if (snapshot.banner_close_text !== "Dismiss") issues.push(`registration close text ${snapshot.banner_close_text}`);
		  if (!snapshot.banner_close_visible) issues.push("registration close hidden");
		      if (snapshot.launcher_mail_activity_visible_count !== 0) issues.push(`launcher mail/activity systray visible ${snapshot.launcher_mail_activity_visible_count}`);
		      if (snapshot.developer_tools_count !== 1) issues.push(`developer tools control count ${snapshot.developer_tools_count}`);
		      if (snapshot.studio_count !== 1) issues.push(`studio control count ${snapshot.studio_count}`);
	  if (snapshot.user_name_display === "none") issues.push("launcher user name hidden");
	  if (snapshot.database_badge_count !== 1 || !snapshot.database_badge_text) issues.push(`launcher database badge ${JSON.stringify({ count: snapshot.database_badge_count, text: snapshot.database_badge_text })}`);
	  if (snapshot.app_card_left_px < 240 || snapshot.app_card_left_px > 330) issues.push(`app card left ${snapshot.app_card_left_px}`);
	  if (snapshot.app_card_top_px < 165 || snapshot.app_card_top_px > 230) issues.push(`app card top ${snapshot.app_card_top_px}`);
	  if (snapshot.app_card_width_px < 120 || snapshot.app_card_width_px > 150) issues.push(`app card width ${snapshot.app_card_width_px}`);
	  if (snapshot.app_card_height_px < 98 || snapshot.app_card_height_px > 122) issues.push(`app card height ${snapshot.app_card_height_px}`);
  if (!transparent.has(snapshot.app_card_bg)) issues.push(`app card background ${snapshot.app_card_bg}`);
  if (!transparent.has(snapshot.app_card_border_color)) issues.push(`app card border ${snapshot.app_card_border_color}`);
  if (snapshot.app_icon_width_px < 66 || snapshot.app_icon_width_px > 74) issues.push(`app icon width ${snapshot.app_icon_width_px}`);
  if (snapshot.app_icon_height_px < 66 || snapshot.app_icon_height_px > 74) issues.push(`app icon height ${snapshot.app_icon_height_px}`);
  if (snapshot.app_icon_radius_px < 4 || snapshot.app_icon_radius_px > 8) issues.push(`app icon radius ${snapshot.app_icon_radius_px}`);
  if (snapshot.app_icon_text) issues.push(`synthetic app icon text ${snapshot.app_icon_text}`);
  const appIcon = snapshot.launcher_icons.find((item) => item.key === "apps");
  const settingsIcon = snapshot.launcher_icons.find((item) => item.key === "settings");
  if (!appIcon || appIcon.kind !== "apps" || appIcon.generated !== "clean-room" || !String(appIcon.src_prefix).startsWith("data:image/svg+xml")) {
    issues.push(`apps launcher icon invalid ${JSON.stringify(appIcon)}`);
  }
  if (!settingsIcon || settingsIcon.kind !== "settings" || settingsIcon.generated !== "clean-room" || !String(settingsIcon.src_prefix).startsWith("data:image/svg+xml")) {
    issues.push(`settings launcher icon invalid ${JSON.stringify(settingsIcon)}`);
  }
  if (issues.length) throw new Error(`enterprise launcher style audit failed: ${issues.join("; ")}`);
  return snapshot;
}

function isDarkLauncherBackground(value) {
  const match = String(value || "").match(/rgba?\(([^)]+)\)/i);
  if (!match) return false;
  const [red, green, blue] = match[1]
    .split(",")
    .slice(0, 3)
    .map((part) => Number.parseFloat(part.trim()));
  if (![red, green, blue].every(Number.isFinite)) return false;
  const luminance = 0.2126 * red + 0.7152 * green + 0.0722 * blue;
  return luminance < 120;
}

function isEnterpriseHomeBackgroundImage(value) {
  const image = String(value || "").toLowerCase();
  return image.includes("/web_enterprise/static/img/background-dark.jpg");
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

async function openDefaultServerActionsList(page, config, viewport, options = {}) {
  await setViewport(page, viewport);
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&server_actions_auth=1`) });
  await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "Server Actions auth document ready");
  await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}`) });
  await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "TS webclient ready");
  await setInput(page, ".o_web_client input[aria-label='Search apps and menus']", "Server Actions");
  const actionCardCount = await waitForCount(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", 1, "TS technical search actions");
  await clickExactText(page, ".o_web_client .o_home_menu .o_app[data-menu-action='true']", "Server Actions", ".o_app_name");
  await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "TS technical action ready");
  const windowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-window-action[data-model='ir.actions.server'][data-view='list']", 1, "TS Server Actions list");
  const rowCount = await waitForCount(page, ".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row", 7, "TS Server Actions reference rows");
  const chips = await evaluate(page, `[...document.querySelectorAll(".o_web_client .o_action_manager .o_searchview_facet .o_facet_value")].map((node) => node.textContent.trim()).filter(Boolean)`);
  if (!chips.includes("Top-level actions")) throw new Error(`Server Actions default chip missing: ${JSON.stringify(chips)}`);
  const referenceRows = await evaluate(page, `[...document.querySelectorAll(".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row")]
    .map((row) => [...row.querySelectorAll("td")].map((cell) => cell.textContent.trim()).filter(Boolean))`);
  const actualNames = referenceRows.map((row) => row[0]);
  const usageLabels = referenceRows.map((row) => row[row.length - 1]).filter(Boolean);
  if (actualNames.length < 7 || !actualNames.some((name) => name === "Base: Auto-vacuum internal data") || actualNames.some((name) => !name) || usageLabels.some((label) => /^ir_/.test(label) || label.includes("_")) || !usageLabels.some((label) => label === "Scheduled Action" || label === "Server Action")) {
    throw new Error(`Server Actions reference rows invalid: ${JSON.stringify({ actualNames, usageLabels, referenceRows })}`);
  }
  const routeState = await evaluate(page, `(() => {
    const params = new URLSearchParams((window.location.hash || "").replace(/^#/, ""));
    const row = document.querySelector(".o_web_client .o_action_manager .gorp-list-view tbody tr.o_data_row");
    return {
      action_id: params.get("action") || "",
      menu_id: params.get("menu_id") || "",
      record_id: row?.dataset?.id || ""
    };
  })()`);
  const preferredName = typeof options.preferredName === "string" ? options.preferredName : "";
  const preferredRows = preferredName ? await webCallKW(page, config, "ir.actions.server", "search_read", {
    args: [[["name", "=", preferredName]]],
    kwargs: { fields: ["id", "name", "display_name", "model_id", "usage", "ir_cron_ids"], limit: 1 }
  }) : [];
  const relationRecords = await webCallKW(page, config, "ir.actions.server", "search_read", {
    args: [[]],
    kwargs: { fields: ["id", "name", "display_name", "model_id", "usage", "ir_cron_ids"], limit: 30, order: "id" }
  });
  const preferredRecord = (record) => preferredName && String(record?.name || record?.display_name || "") === preferredName;
  const scheduledRecord = Array.isArray(relationRecords)
    ? (Array.isArray(preferredRows) ? preferredRows.find((record) => preferredRecord(record)) : null) ||
      relationRecords.find((record) => preferredRecord(record)) ||
      relationRecords.find((record) => (Array.isArray(record?.model_id) || Number(record?.model_id)) && (record?.usage === "ir_cron" || (Array.isArray(record?.ir_cron_ids) && record.ir_cron_ids.length > 0)))
    : null;
  const relationRecord = Array.isArray(relationRecords)
    ? scheduledRecord || relationRecords.find((record) => Array.isArray(record?.model_id) || Number(record?.model_id))
    : null;
  if (relationRecord?.id !== undefined && relationRecord?.id !== null) {
    routeState.record_id = String(relationRecord.id);
    routeState.scheduled_action = relationRecord?.usage === "ir_cron" || (Array.isArray(relationRecord?.ir_cron_ids) && relationRecord.ir_cron_ids.length > 0);
  }
  if (!routeState.action_id || !routeState.menu_id || !routeState.record_id) {
    throw new Error(`TS Server Actions route metadata missing: ${JSON.stringify(routeState)}`);
  }
  return { action_card_count: actionCardCount, window_count: windowCount, row_count: rowCount, chips, route_state: routeState };
}

async function openDefaultAppsCatalog(page, config, viewport = desktopViewport(), marker = "apps_catalog") {
  await setViewport(page, viewport);
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&${marker}_auth=1`) });
  await waitFor(page, `document.readyState === "interactive" || document.readyState === "complete"`, "Apps catalog auth document ready");
  await webRequestJSON(page, config, "/web/session/authenticate", { login: "admin", password: "admin" });
  await page.send("Page.navigate", { url: appURL(config.baseURL, `/web?smoke=${++navigationCounter}&${marker}=1`) });
  await waitFor(page, `document.documentElement.dataset.tsWebclient === "ready"`, "Apps catalog TS webclient ready");
  await clickExactText(page, ".o_web_client .o_home_menu .o_app", "Apps", ".o_app_name");
  await waitFor(page, `document.querySelector(".o_web_client .o_action_manager")?.dataset.tsActionStatus === "ready"`, "Apps catalog action ready");
  await waitForCount(page, ".o_web_client .gorp-window-action[data-model='ir.module.module'][data-view='kanban'] .o_kanban_renderer", 1, "TS Apps kanban catalog");
}

async function openDefaultAppsCatalogForModule(page, config, moduleName, marker) {
  await openDefaultAppsCatalog(page, config, desktopViewport(), marker);
  await setInput(page, ".o_web_client .o_action_manager .o_searchview_input", moduleName);
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

async function ensureScheduledActionSmokeRecord(page, config) {
  const ids = await externalResIDs(page, config, [
    "base.action_ir_cron",
    "base.menu_ir_cron"
  ]);
  const actionID = Number(ids["base.action_ir_cron"] || 0);
  if (!actionID) throw new Error("base.action_ir_cron not found for scheduled action smoke");
  const rows = await webCallKW(page, config, "ir.cron", "search_read", {
    args: [[["name", "=", "Base: Auto-vacuum internal data"]]],
    kwargs: { fields: ["id", "name"], limit: 1 }
  });
  const existingID = Number(rows?.[0]?.id || 0);
  if (existingID) {
    return {
      actionID,
      menuID: Number(ids["base.menu_ir_cron"] || 0),
      cronID: existingID
    };
  }
  const created = await webCallKW(page, config, "ir.cron", "create", {
    values: {
      name: "Visual Scheduled Action",
      active: true,
      interval_number: 4,
      interval_type: "hours",
      nextcall: "2099-01-01 00:00:00",
      state: "code",
      code: "model.search([])"
    }
  });
  const cronID = Number(created?.id || created || 0);
  if (!cronID) throw new Error(`scheduled action smoke record invalid: ${JSON.stringify(created)}`);
  return {
    actionID,
    menuID: Number(ids["base.menu_ir_cron"] || 0),
    cronID
  };
}

async function createAutomationSmokeRecord(page, config) {
  const ids = await externalResIDs(page, config, [
    "base.action_base_automation",
    "base.menu_base_automation"
  ]);
  const actionID = Number(ids["base.action_base_automation"] || 0);
  if (!actionID) throw new Error("base.action_base_automation not found for automation smoke");
  const modelRows = await webCallKW(page, config, "ir.model", "search_read", {
    args: [[["model", "=", "mail.mail"]]],
    kwargs: { fields: ["id", "model", "name"], limit: 1 }
  });
  const modelID = Number(modelRows?.[0]?.id || 0);
  if (!modelID) throw new Error("mail.mail model not found for automation smoke");
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const created = await webCallKW(page, config, "base.automation", "create", {
    values: {
      name: `Visual Automation ${suffix}`,
      active: true,
      model_id: modelID,
      model_name: "mail.mail",
      trigger: "create_or_write",
      description: "Visual automation smoke"
    }
  });
  const automationID = Number(created?.id || created || 0);
  if (!automationID) throw new Error(`automation smoke record invalid: ${JSON.stringify(created)}`);
  return {
    actionID,
    menuID: Number(ids["base.menu_base_automation"] || 0),
    automationID,
    modelID
  };
}

async function createDateGroupBySmokeAction(page, config) {
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const messageCreated = await webCallKW(page, config, "mail.message", "create", {
    values: {
      subject: `Visual Date Group ${suffix}`,
      body: "Date group-by smoke",
      message_type: "comment",
      model: "res.partner",
      res_id: 0,
      date: "2026-06-22 09:00:00"
    }
  });
  const messageID = Number(messageCreated?.id || messageCreated || 0);
  if (!messageID) throw new Error("date groupby smoke message was not created");
  const actionCreated = await webCallKW(page, config, "ir.actions.act_window", "create", {
    values: {
      name: "Message Date Grouping",
      type: "ir.actions.act_window",
      res_model: "mail.message",
      view_mode: "list",
      limit: 40
    }
  });
  const actionID = Number(actionCreated?.id || actionCreated || 0);
  if (!actionID) throw new Error(`date groupby smoke action invalid: ${JSON.stringify(actionCreated)}`);
  return { actionID, messageID };
}

async function createDateFilterSmokeAction(page, config) {
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  const today = new Date();
  const messageCreated = await webCallKW(page, config, "mail.message", "create", {
    values: {
      subject: `Visual Date Filter ${suffix}`,
      body: "Date filter smoke",
      message_type: "comment",
      model: "res.partner",
      res_id: 0,
      date: `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")} 09:00:00`
    }
  });
  const messageID = Number(messageCreated?.id || messageCreated || 0);
  if (!messageID) throw new Error("date filter smoke message was not created");
  const actionCreated = await webCallKW(page, config, "ir.actions.act_window", "create", {
    values: {
      name: "Message Date Filtering",
      type: "ir.actions.act_window",
      res_model: "mail.message",
      view_mode: "list",
      limit: 40
    }
  });
  const actionID = Number(actionCreated?.id || actionCreated || 0);
  if (!actionID) throw new Error(`date filter smoke action invalid: ${JSON.stringify(actionCreated)}`);
  return { actionID, messageID };
}

async function createKanbanSmokeAction(page, config, options = {}) {
  const limit = Number.isFinite(options.limit) ? Math.max(1, Math.trunc(options.limit)) : 40;
  const actionCreated = await webCallKW(page, config, "ir.actions.act_window", "create", {
    values: {
      name: "Server Actions Kanban",
      type: "ir.actions.act_window",
      res_model: "ir.actions.server",
      view_mode: "kanban,form,list",
      limit
    }
  });
  const actionID = Number(actionCreated?.id || actionCreated || 0);
  if (!actionID) throw new Error(`kanban smoke action invalid: ${JSON.stringify(actionCreated)}`);
  return { actionID };
}

async function ensureKanbanSmokeRecordCount(page, config, minimum) {
  const existing = await webCallKW(page, config, "ir.actions.server", "search_read", {
    args: [[]],
    kwargs: { fields: ["id"], limit: minimum }
  });
  if (Array.isArray(existing) && existing.length >= minimum) return existing.length;
  const modelRows = await webCallKW(page, config, "ir.model", "search_read", {
    args: [[["model", "=", "ir.actions.server"]]],
    kwargs: { fields: ["id", "model"], limit: 1 }
  });
  const modelID = Number(modelRows?.[0]?.id || 0);
  const current = Array.isArray(existing) ? existing.length : 0;
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
  for (let index = current; index < minimum; index += 1) {
    await webCallKW(page, config, "ir.actions.server", "create", {
      values: {
        name: `Visual Kanban Load More ${suffix}-${index}`,
        active: true,
        state: "code",
        model_id: modelID || undefined,
        model_name: "ir.actions.server",
        code: ""
      }
    });
  }
  return minimum;
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
