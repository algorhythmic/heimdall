// Terminal continuity through the compiled daemon, with a shared worktree and
// an abrupt restart. No agents, private files or live workspaces are touched.
const {spawn, execFileSync, spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const assert = require('node:assert/strict');
const {smokePaths, stopProcess} = require('./smoke-paths.cjs');
const {exe, dir} = smokePaths('resume');
const data = join(dir, 'data'), work = join(dir, 'work');
const now = '2026-09-08T18:00:00Z';
const argsFor = args => [...args, '--data-dir', data, '--now', now];
const raw = (...args) => execFileSync(exe, argsFor(args), {encoding: 'utf8', windowsHide: true});
const cli = (...args) => JSON.parse(raw(...args));
const refused = (...args) => spawnSync(exe, argsFor(args), {encoding: 'utf8', windowsHide: true});
const input = (name, value) => { const path = join(dir, name + '.json'); fs.writeFileSync(path, JSON.stringify(value)); return path; };
let daemon;

async function start() {
  daemon = spawn(exe, argsFor(['start']), {windowsHide: true, stdio: ['ignore', 'pipe', 'pipe']});
  await new Promise((resolve, reject) => {
    let errors = '';
    daemon.stderr.on('data', b => errors += b);
    const timer = setTimeout(() => reject(Error('daemon readiness timeout: ' + errors)), 10000);
    daemon.stdout.once('data', () => { clearTimeout(timer); resolve(); });
    daemon.once('error', error => { clearTimeout(timer); reject(error); });
    daemon.once('exit', code => { clearTimeout(timer); reject(Error('daemon exit ' + code + ': ' + errors)); });
  });
}

function draft(target, name, summary) {
  const path = join(dir, name + '.json');
  cli('checkpoint', 'draft', target, '--output', path);
  const request = JSON.parse(fs.readFileSync(path, 'utf8'));
  request.checkpoint.summary = summary;
  request.checkpoint.next_action = 'Review ' + target + ' progress';
  fs.writeFileSync(path, JSON.stringify(request, null, 2));
  return path;
}

(async () => {
  try {
    fs.mkdirSync(work);
    fs.writeFileSync(join(work, 'notes.md'), 'Initial planning artifact');
    cli('init');
    await start();
    for (const target of ['alpha', 'beta']) {
      cli('add', 'Planning ' + target, '--id', target, '--status', 'active');
      const rev = String(cli('state', target).revision);
      const resource = cli('resource', 'bind', target, '--expected-task-revision', rev, '--file', input(target + '-resource', {kind: 'tree', root: work, path: '.'}));
      cli('contract', 'accept', target, '--expected-task-revision', rev, '--file', input(target + '-contract', {previous: 'none', objective: 'Continue ' + target + ' planning', resource_ids: [resource.id]}));
      const file = draft(target, target + '-initial', 'Saved ' + target + ' direction');
      cli('checkpoint', 'submit', target, '--file', file);
    }
    const before = cli('events');
    const alpha = raw('resume', 'alpha'), beta = raw('resume', 'beta');
    assert(alpha.includes('Saved alpha direction') && !alpha.includes('Saved beta direction'));
    assert(beta.includes('Saved beta direction') && !beta.includes('Saved alpha direction'));
    assert.deepEqual(cli('events'), before, 'resume must not write');
    fs.writeFileSync(join(work, 'notes.md'), 'Changed planning artifact');
    assert(cli('resume', 'alpha', '--json').issues.some(i => i.code === 'resource_changed'));
    assert.notEqual(refused('resume', 'alpha', '--budget', '1').status, 0);
    const losing = draft('alpha', 'losing', 'Competing planning notes');
    const winning = draft('alpha', 'winning', 'Reviewed the changed artifact');
    const retained = fs.readFileSync(losing, 'utf8');
    assert.notEqual(refused('checkpoint', 'submit', 'beta', '--file', winning).status, 0);
    const saved = cli('checkpoint', 'submit', 'alpha', '--file', winning);
    assert.match(refused('checkpoint', 'submit', 'alpha', '--file', losing).stderr, /409/);
    assert.equal(fs.readFileSync(losing, 'utf8'), retained);
    await stopProcess(daemon, 'SIGKILL');
    const winningBytes = fs.readFileSync(winning, 'utf8');
    assert.notEqual(refused('checkpoint', 'submit', 'alpha', '--file', winning).status, 0);
    assert.equal(fs.readFileSync(winning, 'utf8'), winningBytes, 'transport failure changed the draft');
    await start();
    assert.deepEqual(cli('checkpoint', 'submit', 'alpha', '--file', winning), saved, 'restart retry changed receipt');
    assert.equal(cli('checkpoint', 'list', 'alpha').length, 2, 'retry appended a checkpoint');
    assert(raw('resume', 'alpha').includes('Reviewed the changed artifact'));
    const state = cli('state');
    cli('replay');
    assert.deepEqual(cli('state'), state);
    console.log(JSON.stringify({status: 'passed', checks: ['explicit same-worktree task selection', 'readable and JSON resume', 'read-only context and resource drift', 'checkpoint draft/submit', 'cross-target and competing-head refusal', 'draft retained on conflict and transport failure', 'abrupt restart and exact retry', 'inert replay'], data: dir}, null, 2));
  } finally {
    await stopProcess(daemon);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
