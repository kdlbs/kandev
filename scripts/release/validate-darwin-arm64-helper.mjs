#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";

const MACHO_MAGIC_64_LE = 0xfeedfacf;
const CPU_TYPE_ARM64 = 0x0100000c;
const LC_CODE_SIGNATURE = 0x1d;
const MACHO_HEADER_SIZE = 32;
const LOAD_COMMAND_HEADER_SIZE = 8;
const MAX_SCAN_BYTES = 64 * 1024;

function fail(file, message) {
  process.stderr.write(`Runtime binary ${path.basename(file)} ${message}\n`);
  process.exit(1);
}

function readHeader(file) {
  const descriptor = fs.openSync(file, "r");
  try {
    const data = Buffer.alloc(MAX_SCAN_BYTES);
    const size = fs.readSync(descriptor, data, 0, data.length, 0);
    return data.subarray(0, size);
  } finally {
    fs.closeSync(descriptor);
  }
}

function hasCodeSignature(data) {
  const commandCount = data.readUInt32LE(16);
  let offset = MACHO_HEADER_SIZE;

  for (let index = 0; index < commandCount; index += 1) {
    if (offset + LOAD_COMMAND_HEADER_SIZE > data.length) return undefined;
    const command = data.readUInt32LE(offset);
    const commandSize = data.readUInt32LE(offset + 4);
    if (command === LC_CODE_SIGNATURE) return true;
    if (commandSize < LOAD_COMMAND_HEADER_SIZE) return undefined;
    offset += commandSize;
  }

  return false;
}

const file = process.argv[2];
if (!file) {
  process.stderr.write(`usage: ${path.basename(process.argv[1])} FILE\n`);
  process.exit(2);
}

let data;
try {
  data = readHeader(file);
} catch (error) {
  fail(file, `could not be read: ${error.message}`);
}

if (
  data.length < MACHO_HEADER_SIZE ||
  data.readUInt32LE(0) !== MACHO_MAGIC_64_LE ||
  data.readUInt32LE(4) !== CPU_TYPE_ARM64
) {
  fail(file, "is not a parsable thin darwin/arm64 Mach-O");
}

const signed = hasCodeSignature(data);
if (signed === undefined) {
  fail(file, "is not a parsable thin darwin/arm64 Mach-O");
}
if (!signed) {
  fail(file, "is not code-signed; Apple Silicon will refuse to run it");
}
