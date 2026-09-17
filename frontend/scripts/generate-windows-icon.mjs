import { chromium } from "@playwright/test";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const projectDirectory = path.resolve(scriptDirectory, "..", "..");
const svgPath = path.join(projectDirectory, "icons", "paneacea-app-icon.svg");
const outputDirectory = path.join(projectDirectory, "build", "icon-assets");
const outputPath = path.join(outputDirectory, "paneacea.ico");
const sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256];
const svg = await readFile(svgPath, "utf8");
const browser = await chromium.launch({ channel: "msedge", headless: true });
const images = [];

try {
  const page = await browser.newPage();
  await page.setContent(
    `<style>html,body,svg{width:100%;height:100%;margin:0;display:block}</style>${svg}`,
  );
  for (const size of sizes) {
    await page.setViewportSize({ width: size, height: size });
    images.push(
      await page.locator("svg").screenshot({ type: "png", omitBackground: true }),
    );
  }
} finally {
  await browser.close();
}

const headerLength = 6 + images.length * 16;
const header = Buffer.alloc(headerLength);
header.writeUInt16LE(0, 0);
header.writeUInt16LE(1, 2);
header.writeUInt16LE(images.length, 4);
let offset = headerLength;
for (let index = 0; index < images.length; index += 1) {
  const image = images[index];
  const size = sizes[index];
  const entryOffset = 6 + index * 16;
  header.writeUInt8(size === 256 ? 0 : size, entryOffset);
  header.writeUInt8(size === 256 ? 0 : size, entryOffset + 1);
  header.writeUInt8(0, entryOffset + 2);
  header.writeUInt8(0, entryOffset + 3);
  header.writeUInt16LE(1, entryOffset + 4);
  header.writeUInt16LE(32, entryOffset + 6);
  header.writeUInt32LE(image.length, entryOffset + 8);
  header.writeUInt32LE(offset, entryOffset + 12);
  offset += image.length;
}

await mkdir(outputDirectory, { recursive: true });
await writeFile(outputPath, Buffer.concat([header, ...images]));
