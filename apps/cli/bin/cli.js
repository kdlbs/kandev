#!/usr/bin/env node

const { run } = require("./native-shim");

process.exitCode = run(process.argv.slice(2));
