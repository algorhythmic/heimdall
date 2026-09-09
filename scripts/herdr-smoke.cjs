// Installed Linux acceptance. Only a newly created, isolated Herdr server and
// synthetic Heimdall data are controlled. Herdr is deliberately not installed by CI.
const {spawn,execFileSync,spawnSync} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const {tmpdir} = require('node:os');
const net = require('node:net');
const {randomBytes} = require('node:crypto');
const assert = require('node:assert/strict');
const {smokePaths,stopProcess} = require('./smoke-paths.cjs');
const {exe,dir} = smokePaths('herdr');
const id=()=>randomBytes(16).toString('hex');
const live=fs.mkdtempSync(join(tmpdir(),'ht02-'));
const data=join(dir,'data'),repo=join(live,'work');
let daemon,server,socket;
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
const refused=(...args)=>spawnSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const input=(name,value)=>{const file=join(dir,name+'.json');fs.writeFileSync(file,JSON.stringify(value));return file;};
const herdrEnv={...process.env,HERDR_CONFIG_PATH:join(live,'config.toml'),XDG_CONFIG_HOME:join(live,'config'),XDG_STATE_HOME:join(live,'state'),XDG_RUNTIME_DIR:join(live,'runtime'),HERDR_STARTUP_CWD:repo};
for (const key of ['HERDR_SOCKET_PATH','HERDR_CLIENT_SOCKET_PATH','HERDR_PANE_ID','HERDR_TAB_ID','HERDR_WORKSPACE_ID']) delete herdrEnv[key];
async function ready(child,pattern) {
  return new Promise((resolve,reject)=>{
    let out='',errors='';const timer=setTimeout(()=>reject(Error('readiness timeout: '+errors)),10000);
    child.stdout.on('data',b=>{out+=b;const m=out.match(pattern);if(m){clearTimeout(timer);resolve(m);}});
    child.stderr.on('data',b=>{errors+=b;const m=errors.match(pattern);if(m){clearTimeout(timer);resolve(m);}});
    child.once('error',e=>{clearTimeout(timer);reject(e);});
    child.once('exit',code=>{clearTimeout(timer);reject(Error('early exit '+code+': '+errors));});
  });
}
async function startServer() {
  server=spawn('herdr',['--session','heimdall-test','server'],{env:herdrEnv,cwd:repo,stdio:['ignore','pipe','pipe']});
  const m=await ready(server,/api socket: ([^\r\n]+)/);socket=m[1];assert(socket.startsWith(live+'/'));
}
async function api(method,params={}) {
  return new Promise((resolve,reject)=>{
    let data='';const request=id();const conn=net.createConnection(socket);
    conn.setTimeout(3000,()=>conn.destroy(Error('Herdr API timeout')));
    conn.once('connect',()=>conn.write(JSON.stringify({id:request,method,params})+'\n'));
    conn.on('data',b=>{data+=b;if(data.length>1<<20)conn.destroy(Error('oversized response'));});
    conn.on('error',reject);conn.on('end',()=>{try{const r=JSON.parse(data);assert.equal(r.id,request);if(r.error)throw Error(JSON.stringify(r.error));resolve(r.result);}catch(e){reject(e);}});
  });
}
async function stopServer() {
  if(!server || server.exitCode!==null || server.signalCode!==null)return;
  if(!socket){await stopProcess(server);return;}
  await api('server.stop');
  await new Promise((resolve,reject)=>{if(server.exitCode!==null)return resolve();const timer=setTimeout(()=>reject(Error('test Herdr did not stop')),5000);server.once('exit',()=>{clearTimeout(timer);resolve();});});
}
function bind(target,manifest,surface,pane,previous='none',request=id()) {
  return cli('session','bind-herdr',target,'--surface',surface,'--manifest',manifest.id,'--previous',previous,'--socket',socket,'--pane',pane,'--expected-task-revision','1','--request-id',request);
}
const check=(target,r)=>cli('session','refresh',target,'--surface',r.surface);

(async()=>{
  try {
    assert.equal(process.platform,'linux','installed Herdr acceptance requires Linux');
    assert.match(execFileSync('herdr',['--version'],{encoding:'utf8'}),/^herdr 0\.8\.2\s*$/);
    for(const d of ['config','state','runtime','work'])fs.mkdirSync(join(live,d),{mode:0o700});
    const shell=join(live,'shell');fs.writeFileSync(shell,'#!/bin/sh\nexec /bin/bash --noprofile --norc\n',{mode:0o700});
    fs.writeFileSync(join(live,'config.toml'),`onboarding = false\n[terminal]\ndefault_shell = ${JSON.stringify(shell)}\nshell_mode = "non_login"\nnew_cwd = "current"\n[update]\nversion_check = false\nmanifest_check = false\n[ui.sound]\nenabled = false\n`);
    execFileSync('git',['init',repo],{stdio:'ignore'});
    cli('init');daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await ready(daemon,/"status":"running"/);
    await startServer();const snapshot=(await api('session.snapshot')).snapshot;
    const first=snapshot.panes[0];assert(first);
    const second=(await api('workspace.create',{cwd:repo,label:'Beta test',focus:false})).root_pane;
    const records={};
    for(const [target,pane] of [['alpha',first],['beta',second]]) {
      cli('add','Heimdall '+target,'--id',target,'--status','active','--next-action','Review '+target+' plan');
      const surface=id();const manifest=cli('workspace','accept',target,'--expected-task-revision','1','--file',input(target,{previous:'none',name:target,surfaces:[{id:surface,kind:'terminal',label:'Shell',required:true,restore_policy:'manual'}]}));
      const request=id(),binding=bind(target,manifest,surface,pane.pane_id,'none',request);
      assert.equal(binding.version,2);assert.equal(binding.locator.repository,join(repo,'.git'));assert.equal(binding.locator.worktree,repo);
      assert.equal(binding.herdr.terminal_id,pane.terminal_id);
      records[target]={surface,manifest,binding,request,pane};assert.equal(check(target,records[target]).status,'current');
      const publish=id();const result=cli('session','publish',target,'--surface',surface,'--binding',binding.id,'--request-id',publish,'--workspace-summary');assert.equal(result.status,'published');
      const observed=(await api('pane.get',{pane_id:pane.pane_id})).pane;
      assert.equal(observed.tokens.heimdall_task,target);assert.equal(observed.tokens.heimdall_next,'Review '+target+' plan');
      assert.equal((await api('workspace.get',{workspace_id:pane.workspace_id})).workspace.tokens.heimdall_summary,pane.pane_id+' | Heimdall '+target);
      records[target].publish=publish;records[target].published=result;
    }
    const {alpha:a,beta:b}=records;const expires=Date.now()+33000;
    assert.notEqual(refused('session','refresh','beta','--surface',a.surface).status,0);
    const state=cli('state');cli('session','refresh','alpha','--surface',a.surface);assert.deepEqual(cli('state'),state);
    const move=(await api('pane.move',{pane_id:a.pane.pane_id,destination:{type:'new_workspace',label:'Moved alpha'},focus:false})).move_result;
    assert.notEqual(move.pane.pane_id,a.pane.pane_id);assert.equal(move.pane.terminal_id,a.pane.terminal_id);
    assert.notEqual(check('alpha',a).status,'current');
    assert.match(refused('session','publish','alpha','--surface',a.surface,'--binding',a.binding.id).stderr,/409/);
    assert.notEqual(refused('session','bind-herdr','beta','--surface',b.surface,'--manifest',b.manifest.id,'--previous',b.binding.id,'--socket',socket,'--pane',move.pane.pane_id,'--expected-task-revision','1').status,0);
    const original=a.binding;
    a.binding=bind('alpha',a.manifest,a.surface,move.pane.pane_id,a.binding.id);assert.equal(a.binding.surface_id,original.surface_id);assert.equal(check('alpha',a).status,'current');
    const before=cli('state');cli('replay');assert.deepEqual(cli('state'),before);
    // Bounded metadata disappears instead of remaining apparently current after
    // Heimdall stops refreshing. Poll a specific property, not desktop animation.
    let beta,betaWorkspace;
    do {
      beta=(await api('pane.get',{pane_id:b.pane.pane_id})).pane;
      betaWorkspace=(await api('workspace.get',{workspace_id:b.pane.workspace_id})).workspace;
      if(!beta.tokens?.heimdall_task && !beta.title?.includes('Heimdall beta') && !betaWorkspace.tokens?.heimdall_summary)break;
      await new Promise(r=>setTimeout(r,250));
    }while(Date.now()<expires);
    assert(!beta.tokens?.heimdall_task,'metadata failed to expire');assert(!beta.title?.includes('Heimdall beta'));
    assert(!betaWorkspace.tokens?.heimdall_summary,'workspace summary failed to expire');
    await stopServer();assert.equal(check('alpha',a).status,'disconnected');
    await startServer();const restarted=(await api('session.snapshot')).snapshot;
    const stale=check('alpha',a);assert.equal(stale.status,'stale');assert(stale.issues.includes('source_changed'));
    assert.match(refused('session','publish','alpha','--surface',a.surface,'--binding',a.binding.id).stderr,/409/);
    // A committed old receipt may be read after restart; it never republishes.
    assert.deepEqual(cli('session','publish','beta','--surface',b.surface,'--binding',b.binding.id,'--request-id',b.publish,'--workspace-summary'),b.published);
    assert((await api('session.snapshot')).snapshot.panes.every(p=>!p.tokens?.heimdall_task));
    assert(restarted.panes.length>0);
    const current=bind('alpha',a.manifest,a.surface,restarted.panes[0].pane_id,a.binding.id);
    assert.notEqual(current.locator.source_epoch,a.binding.locator.source_epoch);
    assert.equal(cli('state','alpha').task.status,'active');
    const saved=cli('state');await stopProcess(daemon,'SIGKILL');daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await ready(daemon,/"status":"running"/);assert.deepEqual(cli('state'),saved);cli('replay');assert.deepEqual(cli('state'),saved);
    console.log(JSON.stringify({status:'passed',herdr:'0.8.2',protocol:20,checks:['same-repository task/pane separation','canonical live Git and cwd identity','metadata write/readback and 30-second expiry','cross-task and moved-pane refusal','explicit move reconciliation preserving logical surface','terminal identity cannot be adopted by another task','disconnection and server epoch change','no metadata re-emission on retry/replay','explicit new-server binding','Heimdall SIGKILL/restart and inert replay'],data:dir},null,2));
  } finally {
    await stopProcess(daemon);
    try {await stopServer();} catch(e) {await stopProcess(server);throw e;}
    fs.rmSync(live,{recursive:true,force:true});
  }
})().catch(e=>{console.error(e);process.exitCode=1;});
