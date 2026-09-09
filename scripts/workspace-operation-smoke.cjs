// Compiled W05/W07 acceptance: synthetic Unix IPC in CI; explicitly opted-in real
// Hyprland uses one disposable GTK window and never targets another application.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs');const {join}=require('node:path');const os=require('node:os');
const {randomBytes}=require('node:crypto');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {exe,dir}=smokePaths('workspace-operation');const data=join(dir,'data');
const native=process.env.HEIMDALL_NATIVE_OPERATION==='1';let daemon,fake,app,socketDir;
const id=()=>randomBytes(16).toString('hex');
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data,'--json'],{encoding:'utf8'}));
const file=(name,v)=>{const p=join(dir,name+'.json');fs.writeFileSync(p+'.next',JSON.stringify(v));fs.renameSync(p+'.next',p);return p;};
const delay=ms=>new Promise(r=>setTimeout(r,ms));
async function until(fn, attempts=120){for(let n=0;n<attempts;n++){const v=await fn();if(v)return v;await delay(50);}throw Error('observation deadline');}
const ready=p=>new Promise((resolve,reject)=>{let errors='';const timer=setTimeout(()=>reject(Error(errors||'ready timeout')),10000);p.stderr.on('data',b=>errors+=b);p.stdout.once('data',()=>{clearTimeout(timer);resolve();});p.once('error',reject);p.once('exit',c=>{clearTimeout(timer);reject(Error('exit '+c+errors));});});
async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await ready(daemon);}
const window=(stableId,address)=>({stableId,address,pid:123,class:'synthetic-app',title:'Synthetic same title',mapped:true,monitor:0,workspace:{id:1,name:'planning'},at:[10,20],size:[800,600]});
const fixture={version:{version:'0.56.2',commit:'efb50993780079460b0cbed1363e2166a2de1d9f',dirty:false},status:{configProvider:'hyprlang'},activewindow:{},monitors:[{id:0,name:'SYNTHETIC-1',x:0,y:0,width:1920,height:1080,scale:1,transform:0,activeWorkspace:{id:1}}],workspaces:[{id:1,name:'planning',monitor:'SYNTHETIC-1',monitorID:0}],clients:[window('18000001','0x64'),window('18000999','0x65')],close_mode:'remain'};
(async()=>{try{
 cli('init');await start();cli('add','Native operation fixture','--id','alpha','--status','active');
 const ep=JSON.parse(fs.readFileSync(join(data,'endpoint.json'))),browser=JSON.parse(fs.readFileSync(join(data,'browser-endpoint.json')));
 for(const path of ['queue','list','show','cancel','reconcile','residents']){const response=await fetch(ep.url+'/workspace/operation/'+path+'?target=alpha',{headers:{authorization:'Bearer '+browser.token}});assert.equal(response.status,401);}
 const denied=await fetch(ep.url+'/workspace/verify',{method:'POST',headers:{authorization:'Bearer '+browser.token,'content-type':'application/json'},body:JSON.stringify({version:1,target:'alpha',placement_policy:'saved'})});assert.equal(denied.status,401);
 if(process.platform!=='linux'){console.log(JSON.stringify({status:'passed',checks:['native operation routes require local CLI authority; non-Linux adapter unavailable']}));return;}
 const surface=id(),manifest=cli('workspace','accept','alpha','--expected-task-revision','1','--file',file('manifest',{previous:'none',name:'Native fixture',surfaces:[{id:surface,kind:'native',label:'Synthetic view',required:true,restore_policy:'manual'}]}));
 const audit=join(dir,'native-ipc.txt');let stateFile;
 if(native){
  assert(process.env.HYPRLAND_INSTANCE_SIGNATURE&&process.env.XDG_RUNTIME_DIR,'explicit local Hyprland session required');socketDir=join(process.env.XDG_RUNTIME_DIR,'hypr',process.env.HYPRLAND_INSTANCE_SIGNATURE);
  app=spawn('python3',[join(__dirname,'native-workspace-fixture.py')],{stdio:['ignore','pipe','pipe'],env:{...process.env,GDK_BACKEND:'wayland'}});await ready(app);
 }else{
  socketDir=fs.mkdtempSync(join(os.tmpdir(),'heimdall-w05-'));stateFile=file('compositor',fixture);
  fake=spawn('python3',[join(__dirname,'hyprland-operation-fixture.py'),socketDir,stateFile,audit],{stdio:['pipe','pipe','pipe']});await ready(fake);
 }
 const probe=cli('viewport','probe','--socket-dir',socketDir);const source=cli('viewport','select','--file',file('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
 const owned=await until(()=>{const inventory=cli('viewport','inventory');return inventory.snapshot.windows.find(w=>native?w.pid===app.pid:w.identity.stable_id==='18000001');});
 const inventory=cli('viewport','inventory');cli('viewport','bind','alpha','--file',file('binding',{version:1,id:id(),op:'bind',target:'alpha',previous:'none',expected_task_revision:1,binding:{manifest_id:manifest.id,surface_id:surface,source_id:source.id,snapshot_id:inventory.snapshot.id,window:owned.identity}}));
 const status=cli('snapshot','status','alpha');cli('snapshot','capture','alpha','--file',file('capture',{version:1,id:id(),op:'capture',target:'alpha',previous:'none',expected_task_revision:1,manifest_id:manifest.id,source_id:source.id,input_digest:status.input_digest}));
 const queue=kind=>{const preview=cli('workspace','diff','alpha');return cli('workspace',kind,'alpha','--file',file('review-'+id(),preview),'--surfaces','all');};
 const settled=operation=>until(()=>{const op=cli('workspace','operation','alpha','--id',operation.intent.id);return op.status==='complete'?op:null;});
 const reportFile=join(dir,'recovery-present.json'),beforeVerify=cli('state');let report=cli('workspace','verify','alpha','--output',reportFile);assert.equal(report.full,true);assert.equal(report.surfaces[0].workspace_membership.status,'matched');assert.deepEqual(cli('state'),beforeVerify);
 // Report publication must not overwrite retained evidence.
 assert.throws(()=>cli('workspace','verify','alpha','--output',reportFile));
 let op=await settled(queue('focus'));assert.equal(op.outcome,'matched');assert.equal(cli('action','show','alpha','--id',op.action_ids[0]).verification,'matched');
 report=cli('workspace','verify','alpha','--operation',op.intent.id);assert.equal(report.full,true);
 if(native){
  if(process.env.HEIMDALL_NATIVE_REVIEW_FILE){fs.writeFileSync(process.env.HEIMDALL_NATIVE_REVIEW_FILE,JSON.stringify({pid:app.pid,dir}));await until(()=>fs.existsSync(process.env.HEIMDALL_NATIVE_REVIEW_FILE+'.continue'),1200);}
  op=await settled(queue('close'));assert.equal(op.outcome,'matched');await until(()=>app.exitCode!==null);
 }else{
  op=await settled(queue('close'));assert.equal(op.outcome,'partial');assert.equal(Object.keys(cli('state').workspace_slots).length,1);
  report=cli('workspace','verify','alpha','--operation',op.intent.id);assert.equal(report.full,false);assert.equal(report.surfaces[0].existence.status,'not_matched');file('recovery-refused-close',report);
  const current=JSON.parse(fs.readFileSync(stateFile));current.close_mode='pause_close';file('compositor',current);
  const pending=queue('close');await until(()=>fs.readFileSync(audit,'utf8').split('\n').filter(c=>c==='dispatch closewindow stableid:18000001').length===2);
  // The external effect occurred; kill the daemon before the held ACK returns.
  await stopProcess(daemon,'SIGKILL');await start();op=await settled(pending);assert.equal(op.outcome,'matched');
  const action=cli('action','show','alpha','--id',op.action_ids[0]);assert.equal(action.execution,'uncertain');assert.equal(action.report,undefined);assert.equal(action.verification,'matched');
  assert.deepEqual(JSON.parse(fs.readFileSync(stateFile)).clients.map(w=>w.stableId),['18000999']);
  assert.equal(fs.readFileSync(audit,'utf8').split('\n').filter(c=>c.startsWith('dispatch ')).length,3);
 }
 report=cli('workspace','verify','alpha','--operation',op.intent.id);assert.equal(report.full,true);assert.equal(report.surfaces[0].expected,'absent');file('recovery-closed',report);
 const state=cli('state');assert.equal(Object.keys(state.workspace_slots).length,0);assert.equal(state.tasks.alpha.task.status,'active');
 cli('backup','--output',join(dir,'schema19.db'));fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
 cli('replay');assert.deepEqual(cli('state'),state);await stopProcess(daemon);
 console.log(JSON.stringify({status:'passed',native,checks:['pre-dispatch shared journal and close snapshot','independent active-window focus','graceful close and observed residency','unowned windows left open','inert replay','fresh W07 recovery reports before focus and after observed closure','private report export without overwrite or state mutation',...(native?['actual isolated GTK view on selected Hyprland']:['application refusal retains capacity and cannot claim full recovery','daemon kill after native close before ACK','restart observes absence without repeated input'])],data:dir},null,2));
}finally{await stopProcess(daemon);await stopProcess(app);await stopProcess(fake);if(socketDir&&!native)fs.rmSync(socketDir,{recursive:true,force:true});}})().catch(e=>{console.error(e);process.exitCode=1;});
