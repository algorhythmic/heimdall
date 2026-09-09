// Explicit declarations through the compiled CLI. No real sessions or desktop
// state are inspected or controlled; source identities here are synthetic.
const {spawn, execFileSync, spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const {randomBytes} = require('node:crypto');
const assert = require('node:assert/strict');
const {smokePaths, stopProcess} = require('./smoke-paths.cjs');
const {exe, dir} = smokePaths('workspace');
const id = () => randomBytes(16).toString('hex');
let data = join(dir, 'data'), daemon;
const cli = (...args) => JSON.parse(execFileSync(exe, [...args, '--data-dir', data], {encoding:'utf8', windowsHide:true}));
const refused = (...args) => spawnSync(exe, [...args, '--data-dir', data], {encoding:'utf8', windowsHide:true});
const input = (name, value) => { const path = join(dir, name+'.json'); fs.writeFileSync(path, JSON.stringify(value)); return path; };
async function start() {
  daemon = spawn(exe, ['start','--data-dir',data], {windowsHide:true, stdio:['ignore','pipe','pipe']});
  await new Promise((resolve, reject) => {
    let errors = '';
    daemon.stderr.on('data', b => errors += b);
    const timer = setTimeout(() => reject(Error('daemon readiness timeout: '+errors)), 10000);
    daemon.stdout.once('data', () => { clearTimeout(timer); resolve(); });
    daemon.once('error', error => { clearTimeout(timer); reject(error); });
    daemon.once('exit', code => { clearTimeout(timer); reject(Error('daemon exit '+code+': '+errors)); });
  });
}

(async () => {
  try {
    cli('init'); await start();
    const records = {};
    for (const target of ['alpha','beta']) {
      cli('add', 'Planning '+target, '--id', target, '--status', 'active');
      const revision = String(cli('state',target).revision);
      const surface = id();
      const desired = {previous:'none',name:'Planning '+target,surfaces:[{id:surface,kind:'terminal',label:'Shell',required:true,restore_policy:'manual'}]};
      const manifest = cli('workspace','accept',target,'--expected-task-revision',revision,'--file',input(target+'-manifest',desired));
      const session = {previous:'none',manifest_id:manifest.id,surface_id:surface,locator:{adapter:'generic',environment:'test',host:'synthetic-host',source_epoch:'epoch-1',session_id:'planning',workspace_id:'workspace-1',pane_id:target,platform:'linux',cwd:'/synthetic/shared-worktree'}};
      const path = input(target+'-binding',session), request = id();
      const args = ['session','bind',target,'--expected-task-revision',revision,'--request-id',request,'--file',path];
      const binding = cli(...args);
      records[target] = {manifest,desired,session,binding,args,revision,path};
      assert.equal(cli('workspace','show',target).bindings[0].status,'unverified');
    }
    const {alpha:a,beta:b} = records;
    assert.notEqual(refused('session','show','beta','--surface',a.session.surface_id,'--id',a.binding.id).status,0);
    assert.notEqual(refused('workspace','show','beta','--id',a.manifest.id).status,0);
    const collision = structuredClone(b.session);
    collision.previous = b.binding.id;
    collision.locator.pane_id = 'alpha'; collision.locator.workspace_id = 'moved';
    assert.notEqual(refused('session','bind','beta','--expected-task-revision',b.revision,'--file',input('collision',collision)).status,0);
    assert.match(refused('session','bind','alpha','--expected-task-revision',a.revision,'--file',a.path).stderr,/409/);
    const events = cli('events');
    cli('workspace','show','alpha'); cli('workspace','show','beta');
    assert.deepEqual(cli('events'),events);
    const state = cli('state');
    await stopProcess(daemon,'SIGKILL'); await start();
    assert.deepEqual(cli('state'),state);
    assert.deepEqual(cli(...a.args),a.binding);
    cli('replay'); assert.deepEqual(cli('state'),state);
    const unbind = {previous:a.binding.id,manifest_id:a.manifest.id,surface_id:a.session.surface_id};
    const detached = cli('session','unbind','alpha','--expected-task-revision',a.revision,'--file',input('unbind',unbind));
    assert.equal(cli('workspace','show','alpha').bindings[0].status,'unbound');
    assert.equal(cli('session','show','alpha','--surface',a.session.surface_id).active,false);
    const replacement = structuredClone(a.session);
    replacement.previous = detached.id; replacement.locator.source_epoch = 'epoch-2';
    const replaced = cli('session','bind','alpha','--expected-task-revision',a.revision,'--file',input('replacement',replacement));
    assert.equal(replaced.surface_id,a.binding.surface_id);
    assert.equal(cli('session','show','alpha','--surface',a.session.surface_id,'--id',a.binding.id).locator.source_epoch,'epoch-1');
    cli('update','alpha','--title','Revised task');
    const stale = cli('workspace','show','alpha');
    assert(stale.issues.includes('manifest_task_changed'));
    assert(stale.bindings[0].issues.includes('binding_task_changed'));
    assert.equal(stale.bindings[0].status,'unverified');
    const saved = cli('state'), snapshot = join(dir,'snapshot.db');
    cli('backup','--output',snapshot); await stopProcess(daemon);
    const source = data; data = join(dir,'restored'); fs.mkdirSync(data);
    fs.copyFileSync(snapshot,join(data,'heimdall.db'));
    fs.copyFileSync(join(source,'types.yaml'),join(data,'types.yaml'));
    await start(); assert.deepEqual(cli('state'),saved);
    cli('replay'); assert.deepEqual(cli('state'),saved);
    assert.deepEqual(cli(...a.args),a.binding,'historical retry changed after replacement and restore');
    console.log(JSON.stringify({status:'passed',checks:['explicit same-cwd task ownership','cross-task lookup denial','pane collision after declared move','head conflicts','unverified/read-only views','SIGKILL/restart and exact retry','pure replay','explicit unbind and epoch replacement','stable surface and immutable binding history','task revision staleness','fresh-directory backup restore'],data:dir},null,2));
  } finally { await stopProcess(daemon); }
})().catch(error => { console.error(error); process.exitCode=1; });
