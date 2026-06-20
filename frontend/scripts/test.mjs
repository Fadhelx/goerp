import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const pkg = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));

if (pkg.name !== "gorp-frontend") {
  throw new Error("unexpected frontend package name");
}

const root = new URL("..", import.meta.url).pathname;
const filters = process.argv.slice(2).filter((arg) => !arg.startsWith("-"));
const testFiles = [];

async function walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      await walk(path);
    } else if (entry.name.endsWith(".test.mjs")) {
      testFiles.push(path);
    }
  }
}

await walk(join(root, "packages"));

let ran = 0;
for (const file of testFiles.sort()) {
  if (filters.length && !filters.some((filter) => file.includes(filter))) continue;
  await import(pathToFileURL(file));
  ran += 1;
}

if (filters.length && ran === 0) {
  throw new Error(`no tests matched ${filters.join(", ")}`);
}

console.log(`frontend test ok: ${ran} files`);
