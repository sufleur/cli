#!/usr/bin/env node
'use strict';

const path = require('path');
const fs = require('fs');
const os = require('os');
const { spawnSync } = require('child_process');

const isWindows = process.platform === 'win32';
const binary = path.join(__dirname, 'bin', isWindows ? 'sufleur.exe' : 'sufleur');

if (!fs.existsSync(binary)) {
  console.error(
    `\n[@sufleur/cli] Binary not found at ${binary}.\n` +
    `Postinstall did not run, or the download failed. Try one of:\n` +
    `  npm rebuild @sufleur/cli\n` +
    `  node ${path.join(__dirname, 'install.js')}\n`
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  const code = result.error.code;
  if (code === 'ENOENT') {
    console.error(`[@sufleur/cli] could not exec ${binary}. Try: npm rebuild @sufleur/cli`);
  } else if (code === 'EACCES') {
    console.error(`[@sufleur/cli] permission denied executing ${binary}. Try: chmod +x ${binary}`);
  } else {
    console.error(`[@sufleur/cli] spawn error: ${result.error.message}`);
  }
  process.exit(1);
}

if (result.signal) {
  const signo = os.constants.signals[result.signal] || 0;
  process.exit(128 + signo);
}

process.exit(result.status === null ? 0 : result.status);
