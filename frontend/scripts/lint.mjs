import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";

const root = new URL("..", import.meta.url).pathname;
const failures = [];

async function walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name === "dist") continue;
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      await walk(path);
    } else if (/\.(mjs|js|ts|json)$/.test(entry.name)) {
      const text = await readFile(path, "utf8");
      if (/\t/.test(text)) failures.push(`${path}: contains tab indentation`);
    }
  }
}

await walk(root);

if (failures.length) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log("frontend lint ok");
