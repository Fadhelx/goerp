import { defineSandbox, vercelSandboxBackend } from "eve/sandbox";

export default defineSandbox({
  backend: vercelSandboxBackend({
    runtime: "node24",
  }),
});
