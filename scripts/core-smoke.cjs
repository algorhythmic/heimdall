// Compiled core acceptance on Windows and Unix, using only synthetic local data.
const {spawn, execFileSync} = require('node:child_process');
const {join} = require('node:path');
const assert = require('node:assert/strict');
const {root, exe, dir} = require('./smoke-paths.cjs').smokePaths('core');
const data = join(dir, 'data');
const now = '2026-09-04T18:00:00Z';
const raw = (...args) => execFileSync(exe, [...args, '--data-dir', data, '--now', now], {encoding: 'utf8', windowsHide: true}).trim();
const cli = (...args) => JSON.parse(raw(...args));
let daemon;

async function start() {
  daemon = spawn(exe, ['start', '--data-dir', data, '--now', now], {windowsHide: true, stdio: ['ignore', 'pipe', 'pipe']});
  await new Promise((resolve, reject) => {
    let errors = '';
    daemon.stderr.on('data', value => errors += value);
    const timer = setTimeout(() => reject(Error('daemon readiness timeout: ' + errors)), 10000);
    daemon.stdout.once('data', () => { clearTimeout(timer); resolve(); });
    daemon.once('error', error => { clearTimeout(timer); reject(error); });
    daemon.once('exit', code => { clearTimeout(timer); reject(Error('daemon exit ' + code + ': ' + errors)); });
  });
}

const stop = signal => require('./smoke-paths.cjs').stopProcess(daemon, signal);

(async () => {
  try {
    cli('init');
    await start();
    cli('doctor');
    cli('import-tasks', join(root, 'testdata', 'tasks.yaml'));
    cli('add', 'Review core', '--id', 'review-core', '--status', 'active');
    cli('update', 'review-core', '--title', 'Review replay behavior');
    cli('capture', 'heimdall-core/reference: source notes', '--pointer', 'https://example.test/design');
    cli('complete', 'heimdall-core#store');
    cli('complete', 'heimdall-core#tasks');
    const proposals = cli('ratify');
    assert.equal(proposals.length, 1);
    cli('ratify', proposals[0].id, '--accept');
    const before = raw('state');
    cli('replay');
    assert.equal(raw('state'), before, 'replay changed serialized state');
    await stop('SIGKILL');
    await start();
    assert.equal(raw('state'), before, 'abrupt restart changed serialized state');
    console.log(JSON.stringify({status: 'passed', checks: ['compiled core CLI', 'task edits and capture', 'completion proposal acceptance', 'byte-identical replay', 'abrupt process restart'], data: dir}, null, 2));
  } finally {
    await stop();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
