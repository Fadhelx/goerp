---
description: Verify Odoo Enterprise-style web client quality.
---

# Odoo Enterprise Web Check

The UI should read as a restrained business application.

Required visible signals:

- top navigation in `.o_main_navbar`.
- app launcher as the primary entry.
- action area in `.o_action_manager`.
- list renderer with `.o_list_renderer` and `.o_data_row`.
- form sheet with `.o_form_view` and `.o_form_sheet`.

Forbidden normal-user signals:

- `Gorp`
- `Developer RPC`
- `Build dashboard`
- `Create Demo Partner`
- `Backend connected`
- visible model/field/debug controls

Use stable DOM checks plus a screenshot or visible observation.
