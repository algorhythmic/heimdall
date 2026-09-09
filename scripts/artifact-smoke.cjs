// Local Linux artifacts through the compiled CLI. All files and databases are synthetic.
const {spawn, execFileSync, spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const {randomBytes, createHash} = require('node:crypto');
const assert = require('node:assert/strict');
const {smokePaths, stopProcess} = require('./smoke-paths.cjs');
const {exe, dir} = smokePaths('artifact');
const id = () => randomBytes(16).toString('hex');
let data = join(dir, 'data'), daemon;
const work = join(dir, 'work');
const raw = (...args) => execFileSync(exe, [...args, '--data-dir', data], {encoding:'utf8'});
const cli = (...args) => JSON.parse(raw(...args));
const refused = (...args) => spawnSync(exe, [...args, '--data-dir', data], {encoding:'utf8'});
const input = (name, value) => { const path = join(dir, name+'.json'); fs.writeFileSync(path, JSON.stringify(value)); return path; };
async function start() {
  daemon = spawn(exe, ['start','--data-dir',data], {stdio:['ignore','pipe','pipe']});
  await new Promise((resolve,reject) => {
    let errors = '';
    const timer = setTimeout(() => reject(Error('daemon readiness timeout: '+errors)),10000);
    daemon.stderr.on('data',b => errors += b);
    daemon.stdout.once('data',() => { clearTimeout(timer); resolve(); });
    daemon.once('error',error => { clearTimeout(timer); reject(error); });
    daemon.once('exit',code => { clearTimeout(timer); reject(Error('daemon exit '+code+': '+errors)); });
  });
}
(async () => {
  try {
    assert.equal(process.platform,'linux','artifact observation currently targets Linux');
    fs.mkdirSync(work);
    const bytes = 'Planning without a Git commit\n';
    fs.writeFileSync(join(work,'notes.md'),bytes,{mode:0o600});
    cli('init'); await start();
    const resources = {}, contracts = {};
    for (const target of ['alpha','beta']) {
      cli('add','Planning '+target,'--id',target,'--status','active');
      resources[target] = cli('resource','bind',target,'--expected-task-revision','1','--file',input(target+'-resource',{kind:'tree',root:work,path:'.'}));
      contracts[target] = cli('contract','accept',target,'--expected-task-revision','1','--file',input(target+'-contract',{previous:'none',objective:'Continue '+target,resource_ids:[resources[target].id]}));
    }
    const artifact = {artifact_id:'new',previous:'none',name:'Planning notes',environment:'local-test',resource_id:resources.alpha.id,path:'notes.md',git:false};
    const firstArgs = ['artifact','record','alpha','--expected-task-revision','1','--request-id',id(),'--file',input('first',artifact)];
    const first = cli(...firstArgs), aid = first.artifact.id, vid = first.record.id;
    assert.equal(aid,vid);
    assert.equal(first.record.observation.digest,createHash('sha256').update(bytes).digest('hex'));
    assert.equal(first.record.observation.git,undefined);
    assert.equal(cli('artifact','check','alpha','--id',aid).status,'matched');
    assert.notEqual(refused('artifact','show','beta','--id',aid).status,0);
    assert.notEqual(refused('artifact','record','beta','--expected-task-revision','1','--file',input('wrong-target',artifact)).status,0);
    const beta = cli('artifact','record','beta','--expected-task-revision','1','--file',input('beta-artifact',{...artifact,resource_id:resources.beta.id}));
    assert.notEqual(beta.artifact.id,aid);
    assert.equal(beta.record.observation.digest,first.record.observation.digest);
    const refs = [{artifact_id:aid,version_id:vid}];
    const cpArgs = ['checkpoint','create','alpha','--expected-task-revision','1','--request-id',id(),'--file',input('checkpoint',{previous:'none',contract_id:contracts.alpha.id,summary:'Reviewed exact notes',next_action:'Review the design decision',artifacts:refs})];
    const cp = cli(...cpArgs);
    assert.equal(cp.version,3); assert.deepEqual(cp.artifacts,refs);
    const before = cli('events');
    const resume = cli('resume','alpha','--json');
    assert.equal(resume.artifacts[0].status,'matched');
    assert.match(raw('resume','alpha'),/Pinned artifacts/);
    assert.equal(cli('artifact','list','alpha').length,1);
    assert.notEqual(refused('resume','alpha','--budget','1').status,0);
    assert.deepEqual(cli('events'),before,'reads wrote events');
    const draft = join(dir,'draft.json'); cli('checkpoint','draft','alpha','--output',draft);
    const request = JSON.parse(fs.readFileSync(draft,'utf8'));
    assert.equal(request.version,2); assert.deepEqual(request.checkpoint.artifacts,refs);
    request.checkpoint.summary = 'Pinned draft survives restart';
    request.checkpoint.next_action = 'Continue reviewing notes';
    fs.writeFileSync(draft,JSON.stringify(request,null,2));
    const saved = cli('checkpoint','submit','alpha','--file',draft);
    fs.writeFileSync(join(work,'notes.md'),'Changed bytes\n');
    assert.equal(cli('artifact','check','alpha','--id',aid).status,'content_changed');
    const stale = {...request,id:id(),checkpoint:{...request.checkpoint,previous:saved.id}};
    const stalePath = input('stale',stale), retained = fs.readFileSync(stalePath,'utf8');
    assert.match(refused('checkpoint','submit','alpha','--file',stalePath).stderr,/409/);
    assert.equal(fs.readFileSync(stalePath,'utf8'),retained);
    fs.writeFileSync(join(work,'notes.md'),bytes);
    fs.renameSync(join(work,'notes.md'),join(work,'moved.md'));
    assert.equal(cli('artifact','check','alpha','--id',aid).status,'missing');
    const movedInput = {artifact_id:aid,previous:vid,environment:artifact.environment,resource_id:artifact.resource_id,path:'moved.md',git:false};
    const moved = cli('artifact','record','alpha','--expected-task-revision','1','--file',input('moved',movedInput));
    assert.equal(moved.artifact.id,aid); assert.equal(moved.record.previous,vid);
    assert.equal(moved.record.observation.digest,first.record.observation.digest);
    assert.equal(cli('artifact','check','alpha','--id',aid).status,'matched');
    assert.equal(cli('artifact','check','alpha','--id',aid,'--version',vid).status,'relocated');
    assert.equal(cli('resume','alpha','--json').artifacts[0].status,'relocated');
    assert.deepEqual(cli('artifact','show','alpha','--id',aid,'--version',vid).record,first.record);
    // The saved draft must never silently adopt the replacement head.
    assert.deepEqual(JSON.parse(fs.readFileSync(draft,'utf8')).checkpoint.artifacts,refs);
    const state = cli('state');
    fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
    fs.rmSync(work,{recursive:true});
    await stopProcess(daemon,'SIGKILL'); await start();
    assert.deepEqual(cli(...firstArgs),first,'artifact retry probed missing original');
    assert.deepEqual(cli(...cpArgs),cp,'checkpoint retry changed historical reference');
    assert.deepEqual(cli('checkpoint','submit','alpha','--file',draft),saved);
    cli('replay'); assert.deepEqual(cli('state'),state);
    const snapshot = join(dir,'snapshot.db'); cli('backup','--output',snapshot); await stopProcess(daemon);
    const source = data; data = join(dir,'restored'); fs.mkdirSync(data);
    fs.copyFileSync(snapshot,join(data,'heimdall.db')); fs.copyFileSync(join(source,'types.yaml'),join(data,'types.yaml'));
    await start(); assert.deepEqual(cli('state'),state);
    cli('replay'); assert.deepEqual(cli('state'),state);
    assert.deepEqual(cli(...firstArgs),first);
    assert.equal(cli('state','alpha').task.status,'active');
    console.log(JSON.stringify({status:'passed',checks:['non-Git exact file identity','same-file cross-task scope','CLI v3 checkpoint and v2 draft pins','bounded read-only resume','changed/missing/explicit relocation','stale-version conflict preserves draft','SIGKILL/restart exact receipts with original removed','inert replay and fresh-directory restore','task completion unchanged'],data:dir},null,2));
  } finally { await stopProcess(daemon); }
})().catch(error => { console.error(error); process.exitCode=1; });
