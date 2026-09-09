// Compiled P02 acceptance. Decision review is portable; artifact checks are Linux-only.
const {spawn, execFileSync, spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const {randomBytes} = require('node:crypto');
const assert = require('node:assert/strict');
const {smokePaths, stopProcess} = require('./smoke-paths.cjs');
const {exe, dir} = smokePaths('progress');
const id = () => randomBytes(16).toString('hex');
let data = join(dir, 'data'), daemon;
const raw = (...args) => execFileSync(exe, [...args, '--data-dir', data], {encoding:'utf8'});
const cli = (...args) => JSON.parse(raw(...args));
const refused = (...args) => spawnSync(exe, [...args, '--data-dir', data], {encoding:'utf8'});
const input = (name, value) => { const path = join(dir, name+'.json'); fs.writeFileSync(path, JSON.stringify(value)); return path; };
const write = (action, value, request = id()) => ['progress',action,'alpha','--expected-task-revision','1','--request-id',request,'--file',input(request,value)];
const review = (p, status, previous = 'none') => ({proposal_id:p.id,digest:p.digest,previous,status,note:'Explicit review of frozen content'});
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
    cli('init'); await start();
    for (const target of ['alpha','beta']) cli('add','Planning '+target,'--id',target,'--status','active');
    const contract = cli('contract','accept','alpha','--expected-task-revision','1','--file',input('contract',{previous:'none',objective:'Review planning progress'}));
    const proposalArgs = write('propose',{kind:'decision',text:'Use explicit ownership\nPreserve user edits.',contract_id:contract.id});
    const p = cli(...proposalArgs);
    assert.equal(cli('context','alpha').decisions.length,0);
    assert.match(raw('resume','alpha'),/Progress review/);
    assert.equal(cli('progress','show','alpha','--id',p.id).status,'draft');
    assert.notEqual(refused('progress','show','beta','--id',p.id).status,0);
    assert.equal(cli('progress','list','alpha','--limit','1')[0].proposal.id,p.id);
    assert.equal(cli('progress','list','alpha','--after',p.id).length,0);
    const r = cli(...write('review',review(p,'reviewed')));
    assert.equal(cli('context','alpha').decisions.length,0);
    assert.equal(cli('state','alpha').task.status,'active');
    assert.match(refused(...write('review',{...review(p,'accepted',r.id),digest:'a'.repeat(64)})).stderr,/409/);
    const acceptanceArgs = write('review',review(p,'accepted',r.id));
    const accepted = cli(...acceptanceArgs);
    assert.equal(cli('context','alpha').decisions[0].id,accepted.id);
    assert.equal(cli('progress','show','alpha','--id',p.id).freshness,'recorded_current');
    const events = cli('events');
    raw('resume','alpha'); cli('progress','list','alpha'); cli('progress','show','alpha','--id',p.id);
    assert.deepEqual(cli('events'),events,'reads wrote events');
    assert.match(refused('resume','alpha','--budget','1').stderr,/budget_too_small/);
    const pending = cli(...write('propose',{kind:'decision',text:'Unresolved planning choice',contract_id:contract.id}));
    const replacement = cli(...write('propose',{kind:'decision',text:'Use task and environment ownership',contract_id:contract.id,supersedes:accepted.id}));
    cli(...write('review',review(replacement,'accepted')));
    assert.match(refused(...write('review',review(pending,'accepted'))).stderr,/409/);
    cli(...write('review',review(pending,'rejected')));
    assert.equal(cli('progress','show','alpha','--id',p.id).status,'superseded');
    let artifactAcceptanceArgs, artifactAccepted;
    if (process.platform === 'linux') {
      const work = join(dir,'work'); fs.mkdirSync(work);
      const file = join(work,'notes.md'); fs.writeFileSync(file,'A design without a Git commit',{mode:0o600});
      const resource = cli('resource','bind','alpha','--expected-task-revision','1','--file',input('resource',{kind:'tree',root:work,path:'.'}));
      const c = cli('contract','accept','alpha','--expected-task-revision','1','--file',input('artifact-contract',{previous:contract.id,objective:'Review exact planning artifact',resource_ids:[resource.id]}));
      const selectors = {artifact_id:'new',previous:'none',name:'Design notes',environment:'local-test',resource_id:resource.id,path:'notes.md',git:false};
      const a = cli('artifact','record','alpha','--expected-task-revision','1','--file',input('artifact',selectors));
      const p = cli(...write('propose',{kind:'artifact',text:'Review design notes',contract_id:c.id,previous:'none',artifacts:[{artifact_id:a.artifact.id,version_id:a.record.id}]}));
      const r = cli(...write('review',review(p,'reviewed')));
      assert.equal(cli('artifact','show','alpha','--id',a.artifact.id).lifecycle,'reviewed');
      fs.writeFileSync(file,'Changed after review');
      artifactAcceptanceArgs = write('review',review(p,'accepted',r.id));
      assert.match(refused(...artifactAcceptanceArgs).stderr,/409/);
      assert.equal(cli('progress','show','alpha','--id',p.id).freshness,'stale');
      fs.writeFileSync(file,'A design without a Git commit');
      artifactAccepted = cli(...artifactAcceptanceArgs);
      assert.equal(cli('artifact','list','alpha')[0].lifecycle,'accepted');
      const updated = cli('artifact','record','alpha','--expected-task-revision','1','--file',input('new-version',{...selectors,name:undefined,artifact_id:a.artifact.id,previous:a.record.id}));
      assert.equal(updated.lifecycle,'draft');
      assert.equal(cli('progress','show','alpha','--id',p.id).status,'superseded');
      fs.rmSync(work,{recursive:true});
    }
    assert.equal(cli('state','alpha').task.status,'active');
    const state = cli('state');
    fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
    await stopProcess(daemon,'SIGKILL'); await start();
    assert.deepEqual(cli(...proposalArgs),p);
    assert.deepEqual(cli(...acceptanceArgs),accepted);
    if (artifactAcceptanceArgs) assert.deepEqual(cli(...artifactAcceptanceArgs),artifactAccepted);
    cli('replay'); assert.deepEqual(cli('state'),state);
    const snapshot = join(dir,'snapshot.db'); cli('backup','--output',snapshot); await stopProcess(daemon);
    const source = data; data = join(dir,'restored'); fs.mkdirSync(data);
    fs.copyFileSync(snapshot,join(data,'heimdall.db')); fs.copyFileSync(join(source,'types.yaml'),join(data,'types.yaml'));
    await start(); assert.deepEqual(cli('state'),state);
    cli('replay'); assert.deepEqual(cli('state'),state);
    assert.deepEqual(cli(...acceptanceArgs),accepted);
    console.log(JSON.stringify({status:'passed',checks:['decision proposal/review/accept/reject/supersede','exact reviewed digest and prior head','stale accepted direction refusal','read-only scoped selection and budget','task completion unchanged','SIGKILL/restart exact receipts','inert replay and backup restore'],linux_artifacts:process.platform==='linux',data:dir},null,2));
  } finally { await stopProcess(daemon); }
})().catch(error => { console.error(error); process.exitCode=1; });
