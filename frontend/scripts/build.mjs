import { mkdir, writeFile } from "node:fs/promises";

await mkdir(new URL("../dist/", import.meta.url), { recursive: true });
await writeFile(new URL("../dist/manifest.json", import.meta.url), JSON.stringify({
  name: "gorp-frontend",
  status: "bootstrap"
}, null, 2));
console.log("frontend build ok");
