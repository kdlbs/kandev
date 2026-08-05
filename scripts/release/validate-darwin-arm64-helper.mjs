#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";

const MACHO_MAGIC_64_LE = 0xfeedfacf;
const CPU_TYPE_ARM64 = 0x0100000c;
const LC_CODE_SIGNATURE = 0x1d;
const MACHO_HEADER_SIZE = 32;
const LOAD_COMMAND_HEADER_SIZE = 8;
const LINKEDIT_DATA_COMMAND_SIZE = 16;
const CSMAGIC_EMBEDDED_SIGNATURE = 0xfade0cc0;
const CSMAGIC_CODEDIRECTORY = 0xfade0c02;
const CSSLOT_CODEDIRECTORY = 0;
const CODE_DIRECTORY_BASE_SIZE = 44;

function fail(file, message) {
  process.stderr.write(`Runtime binary ${path.basename(file)} ${message}\n`);
  process.exit(1);
}

function isValidCodeDirectory(blob, signedDataOffset) {
  if (blob.length < CODE_DIRECTORY_BASE_SIZE) return false;
  if (blob.readUInt32BE(0) !== CSMAGIC_CODEDIRECTORY) return false;

  const length = blob.readUInt32BE(4);
  if (length < CODE_DIRECTORY_BASE_SIZE || length > blob.length) return false;

  const version = blob.readUInt32BE(8);
  const hashOffset = blob.readUInt32BE(16);
  const identifierOffset = blob.readUInt32BE(20);
  const specialSlotCount = blob.readUInt32BE(24);
  const codeSlotCount = blob.readUInt32BE(28);
  const codeLimit = blob.readUInt32BE(32);
  const hashSize = blob.readUInt8(36);
  const hashType = blob.readUInt8(37);
  const pageSize = blob.readUInt8(39);

  if (version < 0x20000 || hashSize === 0 || hashType === 0 || pageSize > 31) return false;
  if (identifierOffset < CODE_DIRECTORY_BASE_SIZE || identifierOffset >= length) return false;
  const identifierEnd = blob.indexOf(0, identifierOffset);
  if (identifierEnd < identifierOffset || identifierEnd >= length) return false;
  if (codeSlotCount === 0 || codeLimit === 0 || codeLimit > signedDataOffset) return false;

  const specialHashBytes = specialSlotCount * hashSize;
  const codeHashBytes = codeSlotCount * hashSize;
  if (hashOffset < CODE_DIRECTORY_BASE_SIZE + specialHashBytes) return false;
  if (hashOffset > length || codeHashBytes > length - hashOffset) return false;
  return true;
}

function isValidSuperBlob(data, dataOffset, dataSize) {
  if (dataSize < 12 || dataOffset > data.length || dataSize > data.length - dataOffset) return false;
  const blob = data.subarray(dataOffset, dataOffset + dataSize);
  if (blob.readUInt32BE(0) !== CSMAGIC_EMBEDDED_SIGNATURE) return false;

  const length = blob.readUInt32BE(4);
  const count = blob.readUInt32BE(8);
  if (length < 12 || length > blob.length || count > Math.floor((length - 12) / 8)) return false;
  const indexEnd = 12 + count * 8;
  let hasCodeDirectory = false;

  for (let index = 0; index < count; index += 1) {
    const entryOffset = 12 + index * 8;
    const slotType = blob.readUInt32BE(entryOffset);
    const nestedOffset = blob.readUInt32BE(entryOffset + 4);
    if (nestedOffset < indexEnd || nestedOffset > length - 8) return false;
    const nestedLength = blob.readUInt32BE(nestedOffset + 4);
    if (nestedLength < 8 || nestedLength > length - nestedOffset) return false;
    if (slotType === CSSLOT_CODEDIRECTORY) {
      const nestedBlob = blob.subarray(nestedOffset, nestedOffset + nestedLength);
      if (!isValidCodeDirectory(nestedBlob, dataOffset)) return false;
      hasCodeDirectory = true;
    }
  }

  return hasCodeDirectory;
}

function codeSignatureStatus(data) {
  const commandCount = data.readUInt32LE(16);
  const commandBytes = data.readUInt32LE(20);
  if (
    commandBytes > data.length - MACHO_HEADER_SIZE ||
    commandCount > Math.floor(commandBytes / LOAD_COMMAND_HEADER_SIZE)
  ) {
    return "malformed";
  }
  const commandsEnd = MACHO_HEADER_SIZE + commandBytes;
  let offset = MACHO_HEADER_SIZE;

  for (let index = 0; index < commandCount; index += 1) {
    if (offset + LOAD_COMMAND_HEADER_SIZE > commandsEnd) return "malformed";
    const command = data.readUInt32LE(offset);
    const commandSize = data.readUInt32LE(offset + 4);
    if (commandSize < LOAD_COMMAND_HEADER_SIZE || commandSize > commandsEnd - offset) {
      return "malformed";
    }
    if (command === LC_CODE_SIGNATURE) {
      if (commandSize !== LINKEDIT_DATA_COMMAND_SIZE) return "invalid";
      const dataOffset = data.readUInt32LE(offset + 8);
      const dataSize = data.readUInt32LE(offset + 12);
      return isValidSuperBlob(data, dataOffset, dataSize) ? "valid" : "invalid";
    }
    offset += commandSize;
  }

  return offset === commandsEnd ? "missing" : "malformed";
}

const file = process.argv[2];
if (!file) {
  process.stderr.write(`usage: ${path.basename(process.argv[1])} FILE\n`);
  process.exit(2);
}

let data;
try {
  data = fs.readFileSync(file);
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

const signatureStatus = codeSignatureStatus(data);
if (signatureStatus === "malformed") {
  fail(file, "is not a parsable thin darwin/arm64 Mach-O");
}
if (signatureStatus === "invalid") {
  fail(file, "does not contain a valid code signature");
}
if (signatureStatus === "missing") {
  fail(file, "is not code-signed; Apple Silicon will refuse to run it");
}
