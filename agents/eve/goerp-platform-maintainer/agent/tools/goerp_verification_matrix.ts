import { defineTool } from "eve/tools";
import { z } from "zod";

const matrices = {
  base: [
    "go test ./internal/base ./internal/record ./internal/security",
    "go test ./internal/runtime -run '(Bootstrap|Menu|Action|Security)'",
  ],
  mail: [
    "go test ./internal/http -run '(Mail|Chatter|Message|Follower|Subscription)'",
    "go test ./internal/mail -run '(Thread|Activity|Store|Follower|Subscription)'",
  ],
  oi: [
    "go test ./addons/oi_base ./addons/oi_workflow ./addons/oi_workflow_advance ./addons/oi_delegation ./addons/oi_login_as",
    "go test ./internal/workflow ./internal/delegation ./internal/impersonation",
  ],
  ui: [
    "go test ./internal/http -run '(Web|Shell|Asset|Action|Menu)'",
    "pnpm -C frontend typecheck",
    "pnpm -C frontend lint",
    "pnpm -C frontend test",
    "pnpm -C frontend build",
  ],
  release: [
    "go run ./tools/progress_dashboard --out reports/progress_dashboard.html",
    "go test ./tools/progress_dashboard -count=1",
    "make ci",
  ],
};

export default defineTool({
  description: "Return the recommended GoERP verification commands for a bounded delivery lane.",
  inputSchema: z.object({
    lane: z.enum(["base", "mail", "oi", "ui", "release"]),
  }),
  async execute({ lane }) {
    return { lane, commands: matrices[lane] };
  },
});
