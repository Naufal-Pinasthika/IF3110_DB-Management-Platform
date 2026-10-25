import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const directory = process.env.TEST_ARTIFACT_DIR || fileURLToPath(new URL("../artifacts/", import.meta.url));
mkdirSync(directory, { recursive: true });

export function artifact(name) {
  return path.join(directory, name);
}
