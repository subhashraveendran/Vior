// postinstall — runs after npm install. Downloads the binary immediately
// so `npx vior` is instant on first use. Silent by default — the vior.js
// shim will also download on-demand if this step was skipped.
const { execSync } = require('child_process');
try {
  execSync(`node "${__dirname}/vior.js" version`, { stdio: 'ignore', timeout: 30000 });
} catch {
  // binary will be downloaded on first actual invocation
}
