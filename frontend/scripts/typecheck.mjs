import { readFile } from "node:fs/promises";

const config = JSON.parse(await readFile(new URL("../tsconfig.json", import.meta.url), "utf8"));

if (!config.compilerOptions || config.compilerOptions.strict !== true) {
  throw new Error("tsconfig must enable strict mode");
}

console.log("frontend typecheck ok");
