const fs = require('fs');
const path = require('path');

const src = require.resolve('ghostty-web/ghostty-vt.wasm');
const dest = path.join(__dirname, '..', 'public', 'wasm', 'ghostty-vt.wasm');

fs.mkdirSync(path.dirname(dest), { recursive: true });
fs.copyFileSync(src, dest);

console.log('Copied ghostty-vt.wasm to public/wasm/');
