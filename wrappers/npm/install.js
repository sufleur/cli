#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const https = require('https');
const http = require('http');
const { URL } = require('url');
const { execFileSync, spawnSync } = require('child_process');

const PKG = require('./package.json');
const VERSION = PKG.version;

const PLATFORM_MAP = { linux: 'linux', darwin: 'darwin', win32: 'windows' };
const ARCH_MAP = { x64: 'amd64', arm64: 'arm64' };
const SUPPORTED = 'linux/x64, linux/arm64, darwin/x64, darwin/arm64, win32/x64, win32/arm64';

const DEFAULT_BASE = 'https://github.com/WTomas/sufleur-cli/releases/download';
const PREFIX = '[@sufleur/cli postinstall]';

function fail(msg) {
  console.error(`\n${PREFIX} ERROR: ${msg}\n`);
  process.exit(1);
}

function info(msg) {
  console.log(`${PREFIX} ${msg}`);
}

if (process.env.npm_config_offline === 'true') {
  info('offline mode detected; skipping binary download. Run `npm rebuild @sufleur/cli` once online.');
  process.exit(0);
}
if (process.env.SUFLEUR_SKIP_POSTINSTALL === '1') {
  info('SUFLEUR_SKIP_POSTINSTALL=1; skipping binary download.');
  process.exit(0);
}

const goos = PLATFORM_MAP[process.platform];
const goarch = ARCH_MAP[process.arch];
if (!goos || !goarch) {
  fail(
    `unsupported platform: ${process.platform}/${process.arch}.\n` +
    `  Supported: ${SUPPORTED}.`
  );
}

const isWindows = goos === 'windows';
const ext = isWindows ? 'zip' : 'tar.gz';
const archive = `sufleur_${VERSION}_${goos}_${goarch}.${ext}`;
const binaryName = isWindows ? 'sufleur.exe' : 'sufleur';

const base = (process.env.SUFLEUR_BINARY_MIRROR || DEFAULT_BASE).replace(/\/$/, '');
const archiveUrl = `${base}/v${VERSION}/${archive}`;
const checksumsUrl = `${base}/v${VERSION}/checksums.txt`;

const binDir = path.join(__dirname, 'bin');
const binTarget = path.join(binDir, binaryName);
const cacheMarker = path.join(binDir, '.sufleur.sha256');
fs.mkdirSync(binDir, { recursive: true });

function getFollowingRedirects(rawUrl, depth = 0) {
  return new Promise((resolve, reject) => {
    if (depth > 5) return reject(new Error(`too many redirects: ${rawUrl}`));
    const lib = rawUrl.startsWith('https:') ? https : http;
    const req = lib.get(
      rawUrl,
      { headers: { 'user-agent': '@sufleur/cli installer' } },
      (res) => {
        const status = res.statusCode;
        if (status >= 300 && status < 400 && res.headers.location) {
          res.resume();
          const next = new URL(res.headers.location, rawUrl).toString();
          return resolve(getFollowingRedirects(next, depth + 1));
        }
        if (status !== 200) {
          res.resume();
          return reject(new Error(`GET ${rawUrl} -> HTTP ${status}`));
        }
        resolve(res);
      }
    );
    req.on('error', (err) => reject(new Error(`fetch failed: ${rawUrl}: ${err.message}`)));
  });
}

async function fetchText(url) {
  const res = await getFollowingRedirects(url);
  return await new Promise((resolve, reject) => {
    let buf = '';
    res.setEncoding('utf8');
    res.on('data', (chunk) => { buf += chunk; });
    res.on('end', () => resolve(buf));
    res.on('error', reject);
  });
}

async function downloadAndHash(url, destPath) {
  const res = await getFollowingRedirects(url);
  return await new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const out = fs.createWriteStream(destPath);
    res.on('data', (chunk) => { hash.update(chunk); });
    res.on('error', reject);
    out.on('error', reject);
    out.on('finish', () => resolve(hash.digest('hex')));
    res.pipe(out);
  });
}

function lookupChecksum(body, filename) {
  for (const line of body.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const parts = trimmed.split(/\s+/);
    if (parts.length < 2) continue;
    const [hash, name] = parts;
    if (name === filename || name === `*${filename}`) return hash;
  }
  return null;
}

function readCachedHash() {
  try {
    return fs.readFileSync(cacheMarker, 'utf8').trim();
  } catch {
    return null;
  }
}

function extractArchive(archivePath, destDir, member) {
  // Try the system `tar` first. Modern macOS, Linux, and Windows 10 1803+
  // all ship a libarchive-based tar that handles both .tar.gz and .zip.
  try {
    execFileSync('tar', ['-xf', archivePath, '-C', destDir, member], { stdio: 'inherit' });
    return;
  } catch (err) {
    if (!isWindows) throw err;
  }
  // Windows fallback: PowerShell Expand-Archive (extracts the whole zip;
  // the binary we want is the only member of interest).
  const ps = spawnSync(
    'powershell.exe',
    [
      '-NoProfile',
      '-NonInteractive',
      '-Command',
      `Expand-Archive -Force -Path '${archivePath.replace(/'/g, "''")}' -DestinationPath '${destDir.replace(/'/g, "''")}'`
    ],
    { stdio: 'inherit' }
  );
  if (ps.status !== 0) {
    throw new Error('PowerShell Expand-Archive failed');
  }
}

(async function main() {
  let expectedHex;
  try {
    info(`fetching checksums: ${checksumsUrl}`);
    const checksumsBody = await fetchText(checksumsUrl);
    expectedHex = lookupChecksum(checksumsBody, archive);
    if (!expectedHex) {
      fail(`no checksum entry for ${archive} in ${checksumsUrl}`);
    }
  } catch (err) {
    fail(err.message);
  }

  if (fs.existsSync(binTarget) && readCachedHash() === expectedHex) {
    info(`${binaryName} already installed (sha256 matches); skipping download.`);
    return;
  }

  const tmp = path.join(binDir, `${archive}.partial`);
  try {
    info(`downloading: ${archiveUrl}`);
    const actualHex = await downloadAndHash(archiveUrl, tmp);
    if (actualHex !== expectedHex) {
      try { fs.unlinkSync(tmp); } catch {}
      fail(
        `SHA256 mismatch for ${archive}\n` +
        `  expected: ${expectedHex}\n` +
        `  actual:   ${actualHex}\n` +
        `  url:      ${archiveUrl}`
      );
    }

    info(`extracting ${binaryName} from ${archive}`);
    try {
      extractArchive(tmp, binDir, binaryName);
    } catch (err) {
      fail(`extraction failed for ${archive}: ${err.message}`);
    }

    if (!fs.existsSync(binTarget)) {
      fail(
        `extraction completed but ${binTarget} is missing. ` +
        `The archive layout may have changed (e.g. wrap_in_directory).`
      );
    }

    if (!isWindows) {
      fs.chmodSync(binTarget, 0o755);
    }

    fs.writeFileSync(cacheMarker, `${expectedHex}\n`);
    info(`installed ${binaryName} at ${binTarget}`);
  } catch (err) {
    fail(err.message);
  } finally {
    try { fs.unlinkSync(tmp); } catch {}
  }
})();
