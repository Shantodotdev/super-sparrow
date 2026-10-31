import { existsSync, readFileSync } from "node:fs";

const clientDirectory = "web/dist/client";
const shell = readFileSync(`${clientDirectory}/_shell.html`, "utf8");
const assets = [...shell.matchAll(/(?:src|href)="(\/assets\/[^"?#]+)"/g)].map((match) => match[1]);

if (assets.length === 0) {
  throw new Error("The frontend shell contains no asset references");
}

for (const asset of assets) {
  if (!existsSync(`${clientDirectory}${asset}`)) {
    throw new Error(`The frontend shell references a missing asset: ${asset}`);
  }
}

console.log(`Verified ${assets.length} frontend shell asset references`);
