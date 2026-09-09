// Opt-in installed Linux acceptance. Every mutated window, task and Herdr
// server is created by this script; existing desktop applications are unowned.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs');const {join}=require('node:path');const os=require('node:os');
const {randomBytes,createHash}=require('node:crypto');const net=require('node:net');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {exe,dir}=smokePaths('application');const data=join(dir,'data');
const live=fs.mkdtempSync(join(os.tmpdir(),'heimdall-w06-'));const id=()=>randomBytes(16).toString('hex');
const input=(name,v)=>{const p=join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(v));return p;};
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data,'--json'],{encoding:'utf8'}));
const hash=p=>createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const delay=ms=>new Promise(r=>setTimeout(r,ms));
async function until(fn){for(let i=0;i<160;i++){const value=await fn();if(value)return value;await delay(75);}throw Error('application observation deadline');}
function ready(child,pattern=/./){return new Promise((resolve,reject)=>{let out='';const timer=setTimeout(()=>reject(Error('readiness deadline '+out)),10000);const read=b=>{out+=b;const m=out.match(pattern);if(m){clearTimeout(timer);resolve(m);}};child.stdout.on('data',read);child.stderr.on('data',read);child.once('exit',c=>{clearTimeout(timer);reject(Error('early exit '+c+' '+out));});child.once('error',reject);});}
let daemon,seed,server,socket;const launched=[];
const env={...process.env,XDG_CONFIG_HOME:join(live,'config'),XDG_STATE_HOME:join(live,'state')};
async function start(){daemon=spawn(exe,['start','--data-dir',data],{env,stdio:['ignore','pipe','pipe']});await ready(daemon,/"status":"running"/);}
async function api(method,params={}){return new Promise((resolve,reject)=>{let raw='';const conn=net.createConnection(socket);const request=id();conn.setTimeout(3000,()=>conn.destroy(Error('Herdr deadline')));conn.once('connect',()=>conn.write(JSON.stringify({id:request,method,params})+'\n'));conn.on('data',b=>raw+=b);conn.on('error',reject);conn.on('end',()=>{try{const r=JSON.parse(raw);if(r.error)throw Error(JSON.stringify(r.error));resolve(r.result);}catch(e){reject(e);}});});}
async function seedSurface(target,kind,source){
 cli('add','W06 '+target,'--id',target,'--status','active');
 const surface=id();const manifest=cli('workspace','accept',target,'--expected-task-revision','1','--file',input(id(),{previous:'none',name:target,surfaces:[{id:surface,kind,label:'Disposable '+kind,required:true,restore_policy:'manual'}]}));
 seed=spawn('python3',[join(__dirname,'native-workspace-fixture.py')],{env:{...env,GDK_BACKEND:'wayland'},stdio:['ignore','pipe','pipe']});await ready(seed);
 const w=await until(()=>cli('viewport','inventory').snapshot.windows.find(w=>w.pid===seed.pid));
 const inventory=cli('viewport','inventory');
 cli('viewport','bind',target,'--file',input(id(),{version:1,id:id(),op:'bind',target,previous:'none',expected_task_revision:1,binding:{manifest_id:manifest.id,surface_id:surface,source_id:source.id,snapshot_id:inventory.snapshot.id,window:w.identity}}));
 return {target,manifest,surface};
}
function recipe(r,spec){return cli('application','review',r.target,'--file',input(id(),{version:1,id:id(),target:r.target,expected_task_revision:1,manifest_id:r.manifest.id,surface_id:r.surface,previous:'none',spec}));}
function capture(r,source){const status=cli('snapshot','status',r.target);cli('snapshot','capture',r.target,'--file',input(id(),{version:1,id:id(),op:'capture',target:r.target,previous:'none',expected_task_revision:1,manifest_id:r.manifest.id,source_id:source.id,input_digest:status.input_digest}));}
function queue(r,kind,request=id()){const review=cli('workspace','diff',r.target);return cli('workspace',kind,r.target,'--file',input(id(),review),'--surfaces','all','--request-id',request);}
async function settle(r,op){return until(()=>{const current=cli('workspace','operation',r.target,'--id',op.intent.id);return current.status==='complete'?current:null;});}
async function open(r){await stopProcess(seed);seed=null;const op=queue(r,'open');assert.equal(op.unsupported.length,0);const done=await settle(r,op);assert.equal(done.outcome,'matched');const action=cli('action','show',r.target,'--id',op.action_ids[0]);assert.equal(action.verification,'matched');assert.equal(action.intent.version,4);launched.push(action.report.native.process);const state=cli('state');assert.equal(state.viewport_bindings[state.viewport_heads[r.surface]].application_action_id,action.intent.id);return action;}
(async()=>{try{
 assert.equal(process.env.HEIMDALL_NATIVE_APPLICATION,'1','set HEIMDALL_NATIVE_APPLICATION=1 for disposable real application acceptance');
 assert.equal(process.platform,'linux');assert(process.env.HYPRLAND_INSTANCE_SIGNATURE&&process.env.XDG_RUNTIME_DIR);
 fs.mkdirSync(env.XDG_CONFIG_HOME,{recursive:true});fs.mkdirSync(env.XDG_STATE_HOME,{recursive:true});
 cli('init');await start();
 const socketDir=join(process.env.XDG_RUNTIME_DIR,'hypr',process.env.HYPRLAND_INSTANCE_SIGNATURE);
 const probe=cli('viewport','probe','--socket-dir',socketDir);
 const source=cli('viewport','select','--file',input('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
 const generic=await seedSurface('terminal','terminal',source);
 recipe(generic,{adapter:'foot',executable:'/usr/bin/foot',executable_digest:hash('/usr/bin/foot'),command:'/usr/bin/bash',command_digest:hash('/usr/bin/bash'),argv:['--noprofile','--norc'],cwd:live,close_policy:'graceful_session_end'});
 capture(generic,source);await open(generic);
 const focus=await settle(generic,queue(generic,'open'));assert.equal(cli('action','show',generic.target,'--id',focus.action_ids[0]).intent.native.kind,'focus');
 await stopProcess(daemon,'SIGKILL');await start();
 assert.equal(Object.values(cli('state').actions).filter(a=>a.intent.native?.kind==='open').length,1);
 assert.equal((await settle(generic,queue(generic,'close'))).outcome,'matched');
 const config=join(live,'herdr.toml');fs.writeFileSync(config,'onboarding = false\n[terminal]\ndefault_shell = "/usr/bin/bash"\nshell_mode = "non_login"\n[update]\nversion_check = false\nmanifest_check = false\n[ui.sound]\nenabled = false\n');
 const serverEnv={...env,HERDR_CONFIG_PATH:config,HERDR_STARTUP_CWD:live};for(const k of ['HERDR_SOCKET_PATH','HERDR_CLIENT_SOCKET_PATH','HERDR_PANE_ID','HERDR_TAB_ID','HERDR_WORKSPACE_ID','HERDR_SESSION'])delete serverEnv[k];
 server=spawn('/usr/bin/herdr',['--session','heimdall-w06','server'],{cwd:live,env:serverEnv,stdio:['ignore','pipe','pipe']});socket=(await ready(server,/api socket: ([^\r\n]+)/))[1];assert(socket.startsWith(live+'/'));
 const pane=(await api('session.snapshot')).snapshot.panes[0];const terminal=await seedSurface('herdr','terminal',source);
 const binding=cli('session','bind-herdr',terminal.target,'--surface',terminal.surface,'--manifest',terminal.manifest.id,'--previous','none','--socket',socket,'--pane',pane.pane_id,'--expected-task-revision','1');
 recipe(terminal,{adapter:'foot-herdr',session_binding_id:binding.id,executable:'/usr/bin/foot',executable_digest:hash('/usr/bin/foot'),command:'/usr/bin/herdr',command_digest:hash('/usr/bin/herdr'),argv:[],cwd:live,close_policy:'detach'});
 capture(terminal,source);await open(terminal);
 assert.equal((await settle(terminal,queue(terminal,'close'))).outcome,'matched');
 const refreshed=cli('session','refresh',terminal.target,'--surface',terminal.surface);assert.equal(refreshed.status,'current');
 await api('server.stop');await until(()=>server.exitCode!==null);server=null;
 const expired=await settle(terminal,queue(terminal,'open'));assert.equal(expired.outcome,'partial');assert.equal(expired.action_ids.length,0);
 const editor=await seedSurface('editor','editor',source);const saved=join(live,'saved.txt');fs.writeFileSync(saved,'Saved editor state\nSecond line\n');
 recipe(editor,{adapter:'foot-nvim',editor:{files:[saved],active:0,line:2,column:1},executable:'/usr/bin/foot',executable_digest:hash('/usr/bin/foot'),command:'/usr/bin/nvim',command_digest:hash('/usr/bin/nvim'),argv:[],cwd:live,close_policy:'leave_open'});
 capture(editor,source);await open(editor);
 const preserved=await settle(editor,queue(editor,'close'));assert.equal(preserved.outcome,'partial');assert.equal(preserved.action_ids.length,0);
 const state=cli('state');cli('replay');assert.deepEqual(cli('state'),state);cli('backup','--output',join(dir,'schema20.db'));
 fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
 console.log(JSON.stringify({status:'passed',checks:['real foot launch and new runtime binding','repeat open focuses surviving view','daemon kill/restart does not duplicate launch','reviewed graceful terminal close','Herdr direct attach and surviving-session detach','expired Herdr session does not restart','structured Neovim saved-file launch','editor close preserves potentially unsaved state','inert replay'],data:dir},null,2));
}finally{
 await stopProcess(seed);await stopProcess(daemon);
 if(server){try{await api('server.stop');}catch{}await stopProcess(server);}
 // These are disposable child views created and recorded by this test only.
 for(const pin of launched){try{const stat=fs.readFileSync(`/proc/${pin.pid}/stat`,'utf8');if(stat.slice(stat.lastIndexOf(')')+1).trim().split(/\s+/)[19]===pin.start)process.kill(pin.pid,'SIGTERM');}catch(e){if(!['ESRCH','ENOENT'].includes(e.code))throw e;}}
 // Leave the fixture directory for inspecting acceptance output and editor state.
}})().catch(e=>{console.error(e);process.exitCode=1;});
