import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { inspectArchive, integrityOf } from "../pluginsdk/pack-candidate.mjs";
import { assetNames, makeReceipt, validateReceipt } from "./sdk-artifact.mjs";

export class CandidateStore {
  constructor(options) {
    Object.assign(this, options);
  }
  async checkArchive(archive, source, integrity) {
    if ((await integrityOf(archive)) !== integrity)
      throw new Error("retained or rebuilt candidate integrity conflict");
    const inspected = await inspectArchive(archive, source.packConfig);
    if (inspected.recordDigest !== source.recordDigest)
      throw new Error("candidate record differs from original source");
    return inspected;
  }
  async obtain(source, { allowCreate, bootstrapAllowed = false }) {
    await mkdir(this.directory, { recursive: true });
    const names = assetNames(source.record.sdk_version);
    const retained = await this.assets.download(
      source.hostTag,
      names,
      this.directory,
    );
    let { archive, receiptPath, signaturePath } = retained;
    let receipt;
    if (!receiptPath) {
      if (!allowCreate || signaturePath)
        throw new Error("authenticated candidate receipt is missing");
      const built = await this.build(source);
      await this.checkArchive(built.archive, source, built.integrity);
      if (archive) await this.checkArchive(archive, source, built.integrity);
      archive ??= built.archive;
      receipt = makeReceipt(source, built, this.workflow, bootstrapAllowed);
      receiptPath = join(this.directory, names.receipt);
      await writeFile(receiptPath, JSON.stringify(receipt, null, 2) + "\n");
    } else {
      receipt = JSON.parse(await readFile(receiptPath, "utf8"));
      validateReceipt(receipt, source);
      if (signaturePath) await this.verify(receiptPath, signaturePath);
      else {
        if (!allowCreate)
          throw new Error("candidate receipt signature is missing");
        await this.verifyWorkflow(receipt);
      }
      if (!archive || !signaturePath) {
        const built = await this.build(source);
        await this.checkArchive(
          built.archive,
          source,
          receipt.archive_integrity,
        );
        archive ??= built.archive;
      }
    }
    await this.checkArchive(archive, source, receipt.archive_integrity);
    if (!signaturePath) {
      signaturePath = join(this.directory, names.signature);
      await this.sign(receiptPath, signaturePath);
    }
    const fingerprint = await this.verify(receiptPath, signaturePath);
    return {
      archive,
      receiptPath,
      signaturePath,
      receipt,
      fingerprint,
      integrity: receipt.archive_integrity,
      files: receipt.files,
      node: receipt.node,
      npm: receipt.npm,
      sdk_version: receipt.sdk_version,
    };
  }
}
