// S2a native acceptance uses a disposable GTK window; WCU input is supplied by
// the operator/MCP host while this script waits. It never sends desktop input.
const {spawn,execFileSync}=require('child_process');
const fs=require('fs'),path=require('path'),os=require('os'),assert=require('assert/strict');
const {randomBytes}=require('crypto');
const exe=path.resolve(process.argv[2]),dir=path.resolve(process.argv[3]),data=path.join(dir,'data');
fs.mkdirSync(dir,{recursive:true});
const id=()=>randomBytes(16).toString('hex');
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data,'--json'],{encoding:'utf8'}));
const file=(name,value)=>{let p=path.join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(value));return p};
const delay=ms=>new Promise(r=>setTimeout(r,ms));
const ready=p=>new Promise((resolve,reject)=>{p.stdout.once('data',resolve);p.once('error',reject);p.once('exit',()=>reject(Error('fixture exited')))});
let daemon,app;
(async()=>{try{
 cli('init');if(process.env.HEIMDALL_WCU_OBSERVER_CONFIG)fs.copyFileSync(process.env.HEIMDALL_WCU_OBSERVER_CONFIG,path.join(data,'wcu-observer.json'));daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','inherit']});await ready(daemon);
 cli('add','External input fixture','--id','external-fixture','--status','active');
 const target='external-fixture',surface=id();
 const manifest=cli('workspace','accept',target,'--expected-task-revision','1','--file',file('manifest',{previous:'none',name:'Disposable WCU acceptance',surfaces:[{id:surface,kind:'native',label:'Synthetic GTK view',required:true,restore_policy:'manual'}]}));
 app=spawn('python3',[path.join(__dirname,'native-workspace-fixture.py')],{stdio:['ignore','pipe','inherit'],env:{...process.env,GDK_BACKEND:'wayland'}});await ready(app);
 const socketDir=path.join(process.env.XDG_RUNTIME_DIR,'hypr',process.env.HYPRLAND_INSTANCE_SIGNATURE);
 const probe=cli('viewport','probe','--socket-dir',socketDir);
 const source=cli('viewport','select','--file',file('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
 let inventory,owned;for(let i=0;i<80;i++){inventory=cli('viewport','inventory');owned=inventory.snapshot.windows.find(w=>w.pid===app.pid);if(owned)break;await delay(50)};assert(owned,'disposable fixture window required');
 cli('viewport','bind',target,'--file',file('binding',{version:1,id:id(),op:'bind',target,previous:'none',expected_task_revision:1,binding:{manifest_id:manifest.id,surface_id:surface,source_id:source.id,snapshot_id:inventory.snapshot.id,window:owned.identity}}));
 const credential=path.join(dir,'action.credential.json');cli('grant','issue','--action',target,'--name','WCU acceptance','--expires',new Date(Date.now()+3600000).toISOString(),'--output',credential);cli('grant','activate','--credential',credential);
 const token=JSON.parse(fs.readFileSync(credential)).token,endpoint=JSON.parse(fs.readFileSync(path.join(data,'client-endpoint.json')));
 const call=async(route,body)=>{let r=await fetch(endpoint.url+route,{method:'POST',headers:{authorization:'Bearer '+token,'content-type':'application/json'},body:JSON.stringify(body)});let result=await r.json();assert.equal(r.status,200,JSON.stringify(result));return result};
 const intent=await call('/client/intent',{version:1,id:id(),target,purpose:'Two guarded Tab inputs in the disposable fixture; verify focus only',surface_id:surface,steps:2,expected:{kind:'window_focused'}});
 file('ready',{dir,pid:app.pid,intent});console.log(JSON.stringify({dir,intent}));
 // WCU must be called separately with the returned exact target. No input replay.
 const reports=path.join(dir,'wcu-reports.json');for(let n=0;!fs.existsSync(reports)&&n<2000;n++)await delay(50);assert(fs.existsSync(reports),'WCU reports were not supplied before deadline');
 const supplied=JSON.parse(fs.readFileSync(reports));assert.equal(supplied.length,2);
 for(let i=0;i<2;i++)await call('/client/report',{version:1,id:id(),target,intent_id:intent.action.intent.id,step:i+1,outcome:supplied[i].outcome,wcu_request_id:supplied[i].wcu_request_id,metrics_digest:supplied[i].metrics_digest});
 let action;for(let i=0;i<100;i++){action=cli('action','show',target,'--id',intent.action.intent.id);if(action.verification!=='pending')break;await delay(50)}
 assert.equal(action.reports.length,2);assert.equal(action.execution,'api_reported');assert(action.observation.native);const read=action.observation.native;assert(read.complete&&read.focus_known);const same=read.window&&read.focused_window&&JSON.stringify(read.focused_window)===JSON.stringify(action.intent.external.owned.window);assert.equal(action.verification,same?'matched':'not_matched');if(process.env.HEIMDALL_WCU_OBSERVER_CONFIG)assert(action.observation.external,'configured WCU corroboration required');file('verified',action);
 cli('tick');const before=cli('state');assert.deepEqual(cli('replay'),before);console.log(JSON.stringify({status:'passed',dir,action_id:action.intent.id,verification:action.verification}));
 }finally{if(app&&app.exitCode===null)app.kill('SIGTERM');if(daemon&&daemon.exitCode===null)daemon.kill('SIGTERM')}})().catch(e=>{console.error(e);process.exitCode=1});
