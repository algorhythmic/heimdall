// Real executable, isolated synthetic data, crash/restart and optional legacy upgrade.
const {spawn, execFileSync, spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join, resolve} = require('node:path');
const {randomBytes} = require('node:crypto');
const assert = require('node:assert/strict');
const id = () => randomBytes(16).toString('hex');
(async () => {
  const root = resolve(__dirname, '..');
  fs.mkdirSync(join(root, '.tools'), {recursive: true});
  const exe = join(root, 'bin', 'heimdall.exe');
  const legacy = process.argv[2] && resolve(process.argv[2]);
  const dir = fs.mkdtempSync(join(root, '.tools', 'continuity-test-'));
  let data = join(dir, 'data');
  let daemon;
  let previousContract = 'none', previousCheckpoint = 'none';
  const legacyContinuity = process.argv[3] === '--legacy-continuity';
  const legacyGrants = process.argv.includes('--legacy-grants');
  const legacyCredential = join(dir,'legacy-reader.credential.json');
  const cli = (args, binary = exe) => JSON.parse(execFileSync(binary, [...args, '--data-dir', data], {encoding: 'utf8', windowsHide: true}));
  const input = (name, body) => { const file = join(dir, name + '.json'); fs.writeFileSync(file, JSON.stringify(body)); return file; };
  async function start(binary = exe) {
    daemon = spawn(binary, ['start', '--data-dir', data], {windowsHide: true, stdio: ['ignore', 'pipe', 'pipe']});
    await new Promise((res, rej) => {
      let stderr = ''; daemon.stderr.on('data', b => stderr += b);
      const timer = setTimeout(() => rej(Error('daemon readiness timeout: ' + stderr)), 10000);
      daemon.stdout.once('data', () => { clearTimeout(timer); res(); });
      daemon.once('error', e => { clearTimeout(timer); rej(e); });
      daemon.once('exit', code => { clearTimeout(timer); rej(Error('daemon exit ' + code + ': ' + stderr)); });
    });
  }
  async function stop() {
    if (!daemon || daemon.exitCode !== null) return;
    await new Promise(res => { daemon.once('exit', res); daemon.kill(); });
  }
  try {
    cli(['init'], legacy || exe);
    await start(legacy || exe);
    cli(['add', 'Continuity fixture', '--id', 'fixture', '--type', 'project', '--status', 'active'], legacy || exe);
    const originalTask = cli(['state', 'fixture'], legacy || exe);
    if (legacyContinuity) {
      assert(legacy, 'legacy continuity requires old executable');
      const contractInput = input('legacy-contract', {previous:'none',objective:'Legacy accepted work',constraints:[]});
      previousContract = cli(['contract','accept','fixture','--expected-task-revision',String(originalTask.revision),'--file',contractInput],legacy).id;
      const checkpointInput = input('legacy-checkpoint', {previous:'none',contract_id:previousContract,summary:'Legacy checkpoint',next_action:'Review after upgrade',blockers:[]});
      previousCheckpoint = cli(['checkpoint','create','fixture','--expected-task-revision',String(originalTask.revision),'--file',checkpointInput],legacy).id;
    }
    if (legacyGrants) cli(['grant','issue','fixture','--name','Legacy reader','--expires',new Date(Date.now()+3600000).toISOString(),'--output',legacyCredential],legacy);
    await stop();
    await start();
    assert.deepEqual(cli(['state', 'fixture']), originalTask);
    if (legacyGrants) {
      assert.equal(cli(['client','task','fixture','--credential',legacyCredential]).task.id,'fixture');
      const denied = spawnSync(exe,['client','checkpoint','fixture','--credential',legacyCredential,'--request-id',id(),'--expected-task-revision',String(originalTask.revision),'--file',input('legacy-denied',{previous:previousCheckpoint,contract_id:previousContract,summary:'Must remain read only',next_action:'Review',blockers:[]})],{encoding:'utf8',windowsHide:true});
      assert.notEqual(denied.status,0);assert.match(denied.stderr,/403/);
    }
    if (legacyContinuity) {
      assert.equal(cli(['checkpoint','show','fixture']).id,previousCheckpoint);
      const upgradedContext = cli(['context','fixture']);
      const oldContract = cli(['contract','show','fixture']);
      if (oldContract.version === 1) assert(upgradedContext.issues.some(i=>i.code==='contract_scope_changed'));
      else assert.equal(upgradedContext.resume_status,'ready');
    }
    if (legacy) assert.equal(fs.readdirSync(join(data, 'backups')).filter(n => n.startsWith('pre-schema-5-')).length, 1);
    const rev = String(originalTask.revision);
    const mutate = (verb, action, file, request = id()) => cli([verb, action, 'fixture', '--expected-task-revision', rev, '--file', file, '--request-id', request]);

    const work = join(dir, 'work'); fs.mkdirSync(work); fs.writeFileSync(join(work, 'result.txt'), 'first');
    const resource = mutate('resource', 'bind', input('resource', {kind: 'tree', root: work, path: '.'}));
    const contract = mutate('contract', 'accept', input('contract', {previous: previousContract, objective: 'Finish synthetic work', constraints: ['Keep user edits'], resource_ids: [resource.id]}));
    const request = id();
    const cpInput = input('checkpoint', {previous: previousCheckpoint, contract_id: contract.id, summary: 'Initial work saved', next_action: 'Verify result', blockers: []});
    const cp = mutate('checkpoint', 'create', cpInput, request);
    assert.deepEqual(mutate('checkpoint', 'create', cpInput, request), cp);
    assert.equal(cli(['context', 'fixture']).resume_status, 'ready');
    assert.equal(cli(['contract', 'show', 'fixture']).id, contract.id);
    assert.equal(cli(['checkpoint', 'show', 'fixture']).id, cp.id);
    const before = cli(['state']); cli(['replay']); assert.deepEqual(cli(['state']), before);
    await stop(); await start(); assert.deepEqual(cli(['state']), before);
    assert.equal(cli(['context', 'fixture']).resume_status, 'ready');
    fs.writeFileSync(join(work, 'result.txt'), 'second');
    assert(cli(['context', 'fixture']).issues.some(i => i.code === 'resource_changed'));
    const small = spawnSync(exe, ['context', 'fixture', '--budget', '1', '--data-dir', data], {encoding: 'utf8', windowsHide: true});
    assert.notEqual(small.status, 0); assert.match(small.stderr, /budget_too_small/);
    const cp2 = mutate('checkpoint', 'create', input('checkpoint2', {previous: cp.id, contract_id: contract.id, summary: 'Reviewed file edit', next_action: 'Review completion', blockers: []}));
    assert.notEqual(cp.id, cp2.id); assert.equal(cli(['context', 'fixture']).resume_status, 'ready');
    assert.equal(cli(['checkpoint', 'show', 'fixture', '--id', cp.id]).id, cp.id);
    cli(['add','Other project','--id','other-project','--type','project','--status','active']);
    const credential = join(dir,'reader-credential.json');
    const grant = cli(['grant','issue','fixture','--name','Smoke reader','--expires',new Date(Date.now()+3600000).toISOString(),'--resources',resource.id,'--output',credential]);
    const client = (action,target='fixture',extra=[]) => cli(['client',action,target,'--credential',credential,...extra]);
    assert.equal(client('task').task.id,'fixture');
    assert.equal(client('context').resume_status,'ready');
    assert.equal(client('history','fixture',['--limit','1']).items.length,1);
    const denied = () => spawnSync(exe,['client','task','other-project','--credential',credential],{encoding:'utf8',windowsHide:true});
    assert.notEqual(denied().status,0);
    const secret = JSON.parse(fs.readFileSync(credential,'utf8')).token;
    assert(!JSON.stringify(cli(['events'])).includes(secret));
    assert(!JSON.parse(fs.readFileSync(join(data,'client-endpoint.json'),'utf8')).token);
    await stop(); await start(); assert.equal(client('context').resume_status,'ready');
    cli(['grant','activate','--credential',credential]); // exact retry of issuance
    cli(['grant','revoke',grant.id]);
    cli(['grant','activate','--credential',credential]); // retry cannot reactivate a revoked grant
    const revoked = spawnSync(exe,['client','task','fixture','--credential',credential],{encoding:'utf8',windowsHide:true});
    assert.notEqual(revoked.status,0); assert.match(revoked.stderr,/403/);
    const saved = cli(['state']);
    const backup = join(dir, 'snapshot.db'); cli(['backup', '--output', backup]); assert(fs.statSync(backup).size > 0);
    const duplicate = spawnSync(exe, ['backup', '--output', backup, '--data-dir', data], {encoding: 'utf8', windowsHide: true});
    assert.notEqual(duplicate.status, 0);
    await stop();
    if (legacy) {
      const refused = spawnSync(legacy, ['init', '--data-dir', data], {encoding: 'utf8', windowsHide: true});
      assert.notEqual(refused.status, 0); assert.match(refused.stderr, /unsupported database schema version 5/);
    }
    const source = data;
    data = join(dir, 'restored'); fs.mkdirSync(data);
    fs.copyFileSync(backup, join(data, 'heimdall.db'));
    fs.copyFileSync(join(source, 'types.yaml'), join(data, 'types.yaml'));
    await start(); assert.deepEqual(cli(['state']), saved);
    cli(['replay']); assert.deepEqual(cli(['state']), saved); await stop();
    if (legacy) {
      data = join(dir, 'legacy-restored'); fs.mkdirSync(data);
      const pre = fs.readdirSync(join(source, 'backups')).find(n => n.startsWith('pre-schema-5-'));
      fs.copyFileSync(join(source, 'backups', pre), join(data, 'heimdall.db'));
      fs.copyFileSync(join(source, 'types.yaml'), join(data, 'types.yaml'));
      await start(legacy); assert.deepEqual(cli(['state', 'fixture'], legacy), originalTask);
      if (legacyContinuity) assert.equal(cli(['checkpoint','show','fixture'],legacy).id,previousCheckpoint);
      await stop();
    }
    console.log(JSON.stringify({status: 'passed', legacy_upgrade: !!legacy, legacy_continuity: legacyContinuity, checks: ['real CLI', 'reviewed contract scope', 'checkpoint retry', 'replay equality', 'forced-stop/restart', 'drift and new checkpoint', 'budget refusal', 'scoped credential issue and activation', 'cross-project denial', 'public port rediscovery', 'revocation and no reactivation on retry', 'credential absent from events', 'exclusive live database backup', 'restore into fresh directory', 'legacy rollback from pre-upgrade backup'], data: dir}, null, 2));
  } finally { await stop(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
