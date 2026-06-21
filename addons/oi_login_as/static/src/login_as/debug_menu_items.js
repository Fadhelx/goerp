/** @odoo-module **/

import { browser } from "@web/core/browser/browser";
import { _t } from "@web/core/l10n/translation";
import { registry } from "@web/core/registry";
import { user } from "@web/core/user";
import _ from "@web/core/debug/debug_menu_items";

export function becomeSuperuser({ env }) {
    const redirect = browser.location.pathname + browser.location.search + browser.location.hash;
    const becomeSuperuserURL = `/web/become/debug?redirect=${redirect}`;
    return {
        type: "item",
        description: _t("Become Superuser"),
        hide: !user.isAdmin,
        href: becomeSuperuserURL,
        callback: () => browser.open(becomeSuperuserURL, "_self"),
        sequence: 560,
        section: "tools",
    };
}

delete registry.category("debug").category("default").content.becomeSuperuser;
registry.category("debug").category("default").add("becomeSuperuser", becomeSuperuser);
