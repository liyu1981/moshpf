#!/usr/bin/env node

const os = require('os');
const path = require('path');
const fs = require('fs');
const { spawnSync } = require('child_process');
const https = require('https');

const VERSION = require('../package.json').version;
const REPO = 'liyu1981/moshpf';
const BINARY_NAME = 'mpf';
const TAG = `v${VERSION}`;

function getPlatform() {
    const p = os.platform();
    if (p === 'linux') return 'linux';
    if (p === 'darwin') return 'darwin';
    return p;
}

function getArch() {
    const a = os.arch();
    if (a === 'x64') return 'amd64';
    if (a === 'arm64') return 'arm64';
    return a;
}

const platform = getPlatform();
const arch = getArch();

const SUPPORTED_PLATFORMS = [
    'linux-amd64',
    'linux-arm64',
    'darwin-arm64'
];

if (!SUPPORTED_PLATFORMS.includes(`${platform}-${arch}`)) {
    console.error(`Unsupported platform: ${platform}-${arch}`);
    console.error(`Supported platforms: ${SUPPORTED_PLATFORMS.join(', ')}`);
    process.exit(1);
}

const binDir = path.join(os.homedir(), '.mpf', 'bin');
const binPath = path.join(binDir, `${BINARY_NAME}-${TAG}-${platform}-${arch}`);

if (!fs.existsSync(binPath)) {
    console.error(`Binary not found locally. Downloading ${BINARY_NAME} ${TAG} for ${platform}-${arch}...`);
    
    if (!fs.existsSync(binDir)) {
        fs.mkdirSync(binDir, { recursive: true });
    }

    const tarName = `${BINARY_NAME}-${TAG}-${platform}-${arch}.tar.gz`;
    const url = `https://github.com/${REPO}/releases/download/${TAG}/${tarName}`;
    const tmpTarPath = path.join(binDir, tarName);

    download(url, tmpTarPath)
        .then(() => {
            // Extract
            const tarResult = spawnSync('tar', ['-xzf', tmpTarPath, '-C', binDir]);
            if (tarResult.status !== 0) {
                throw new Error(`Failed to extract tarball: ${tarResult.stderr}`);
            }
            
            // The tarball contains 'mpf' binary
            const extractedBin = path.join(binDir, BINARY_NAME);
            fs.renameSync(extractedBin, binPath);
            fs.chmodSync(binPath, 0o755);
            fs.unlinkSync(tmpTarPath);
            
            run();
        })
        .catch(err => {
            console.error(`Error: ${err.message}`);
            process.exit(1);
        });
} else {
    run();
}

function run() {
    const args = process.argv.slice(2);
    const result = spawnSync(binPath, args, { stdio: 'inherit' });
    process.exit(result.status);
}

function download(url, dest) {
    return new Promise((resolve, reject) => {
        https.get(url, (res) => {
            if (res.statusCode === 302 || res.statusCode === 301) {
                download(res.headers.location, dest).then(resolve).catch(reject);
                return;
            }
            if (res.statusCode !== 200) {
                reject(new Error(`Failed to download: ${res.statusCode}`));
                return;
            }
            const file = fs.createWriteStream(dest);
            res.pipe(file);
            file.on('finish', () => {
                file.close();
                resolve();
            });
        }).on('error', reject);
    });
}
