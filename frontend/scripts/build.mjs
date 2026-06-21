import { access, mkdir, writeFile } from "node:fs/promises";

const webclientEntry = "apps/webclient/src/main.js";
const webclientEntryURL = new URL(`../dist/${webclientEntry}`, import.meta.url);

await access(webclientEntryURL);

await mkdir(new URL("../dist/", import.meta.url), { recursive: true });
await writeFile(new URL("../dist/manifest.json", import.meta.url), JSON.stringify({
  name: "gorp-frontend",
  status: "bootstrap",
  entrypoints: {
    webclient: webclientEntry
  },
  served: {
    webclient: `/web/static/frontend/${webclientEntry}`
  }
}, null, 2));
console.log("frontend build ok");
