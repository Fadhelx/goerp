import { defineTool } from "eve/tools";
import { z } from "zod";

const selectors = [
  ".o_web_client",
  ".o_main_navbar",
  ".o_action_manager",
  ".o_control_panel",
  ".o_list_renderer",
  ".o_form_view",
  ".o_form_sheet",
  ".o_Chatter",
];

const forbiddenText = [
  "Gorp",
  "Developer RPC",
  "Build dashboard",
  "Create Demo Partner",
  "Backend connected",
];

export default defineTool({
  description: "Return GoERP UI selectors and forbidden strings for Odoo Enterprise-style parity checks.",
  inputSchema: z.object({
    surface: z.enum(["web", "list", "form", "chatter", "all"]).default("all"),
  }),
  async execute({ surface }) {
    return { surface, selectors, forbiddenText };
  },
});
