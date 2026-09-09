// Keep compiled acceptance independent of platform, checkout location and temp root.
const {mkdirSync, mkdtempSync} = require('node:fs');
const {join, resolve} = require('node:path');

function smokePaths(name, binary) {
  const root = resolve(__dirname, '..');
  const suffix = process.platform === 'win32' ? '.exe' : '';
  const exe = resolve(binary || process.env.HEIMDALL_BIN || join(root, 'bin', 'heimdall' + suffix));
  const temp = resolve(process.env.HEIMDALL_TEST_TMP || join(root, '.tools'));
  mkdirSync(temp, {recursive: true});
  const dir = mkdtempSync(join(temp, name + '-test-'));
  return {root, exe, dir, suffix};
}

// Native messaging reads stdin until EOF. Windows termination hid this cleanup
// requirement; close the pipe on Unix and bound shutdown of synthetic children.
async function stopProcess(child, signal = 'SIGTERM') {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => child.kill('SIGKILL'), 3000);
    const deadline = setTimeout(() => { cleanup(); reject(Error('test process did not exit')); }, 6000);
    const cleanup = () => { clearTimeout(timer); clearTimeout(deadline); };
    child.once('exit', () => { cleanup(); resolve(); });
    child.once('error', error => { cleanup(); reject(error); });
    child.stdin?.end();
    child.kill(signal);
  });
}

module.exports = {smokePaths, stopProcess};
