#!/usr/bin/env node
// vior — npm CLI shim
// Automatically downloads the correct platform binary from GitHub Releases
// and forwards all arguments to it.

const { execSync } = require('child_process');
const { existsSync, mkdirSync, chmodSync } = require('fs');
const path = require('path');
const os = require('os');
const https = require('https');

const VERSION = 'v0.2.0';
const BIN_NAME = process.platform === 'win32' ? 'vior.exe' : 'vior';
const RELEASE_URL = `https://github.com/subhashraveendran/Vior/releases/download/${VERSION}`;

function platformArtifact() {
  const p = process.platform;
  const a = process.arch;
  if (p === 'darwin') return a === 'arm64' ? 'vior-cli-macOS-arm64' : 'vior-cli-macOS';
  if (p === 'linux') return a === 'arm64' ? 'vior-cli-Linux-arm64' : 'vior-cli-Linux';
  if (p === 'win32') return 'vior-cli-Windows';
  throw new Error(`Unsupported platform: ${p}-${a}`);
}

const dir = path.join(os.homedir(), '.vior', 'bin');
const bin = path.join(dir, BIN_NAME);

async function download(url, dest) {
  const fs = require('fs');
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https.get(url, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        file.close();
        download(res.headers.location, dest).then(resolve).catch(reject);
        return;
      }
      if (res.statusCode !== 200) {
        reject(new Error(`HTTP ${res.statusCode}`));
        return;
      }
      res.pipe(file);
      file.on('finish', () => { file.close(); resolve(); });
    }).on('error', reject);
  });
}

async function ensureBinary() {
  if (existsSync(bin)) return;
  mkdirSync(dir, { recursive: true });

  const artifact = platformArtifact();
  const url = `${RELEASE_URL}/${artifact}`;
  console.log(`vior: downloading ${artifact} for ${process.platform}-${process.arch}…`);

  try {
    await download(url, bin);
    if (process.platform !== 'win32') chmodSync(bin, 0o755);
    console.log('vior: ready.');
  } catch (e) {
    console.error(`vior: failed to download binary: ${e.message}`);
    process.exit(1);
  }
}

ensureBinary().then(() => {
  try {
    execSync(`"${bin}" ${process.argv.slice(2).join(' ')}`, { stdio: 'inherit' });
  } catch (e) {
    process.exit(e.status || 1);
  }
});
