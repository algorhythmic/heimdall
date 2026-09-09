// Shared action journal acceptance using synthetic browser transport only.
// An optional old binary creates the pre-upgrade browser history on disk.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs');const {join,resolve}=require('node:path');
const {randomBytes}=require('node:crypto');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {exe,dir}=smokePaths('actions'),legacy=process.argv[2]&&resolve(process.argv[2]);let data=join(dir,'data'),daemon;
const id=()=>randomBytes(16).toString('hex');
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
const old=(...args)=>JSON.parse(execFileSync(legacy||exe,[...args,'--data-dir',data],{encoding:'utf8'}));
const refused=(...args)=>spawnSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const input=(name,value)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(value));return path;};
async function start(binary=exe){daemon=spawn(binary,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{let errors='';const timer=setTimeout(()=>rej(Error(errors||'daemon timeout')),10000);daemon.stderr.on('data',b=>errors+=b);daemon.stdout.once('data',()=>{clearTimeout(timer);res();});daemon.once('exit',code=>{clearTimeout(timer);rej(Error('daemon exit '+code+errors));});daemon.once('error',e=>{clearTimeout(timer);rej(e);});});}
let profile=id(),epoch=id(),connection=id(),sequence=0;
const message=body=>({v:1,id:id(),profile,epoch,connection,...body});
async function ingress(body,expect=200){const ep=JSON.parse(fs.readFileSync(join(data,'browser-endpoint.json')));const response=await fetch(ep.url+'/browser/message',{method:'POST',headers:{authorization:'Bearer '+ep.token,'content-type':'application/json'},body:JSON.stringify(body)});const text=await response.text();assert.equal(response.status,expect,text);return JSON.parse(text);}
async function inventory(tabs=[]){return ingress(message({type:'inventory',seq:++sequence,observed_at:new Date().toISOString(),tabs,focused_window:1,complete:true}));}
(async()=>{try{
 old('init');await start(legacy||exe);old('add','Action task','--id','alpha','--status','active');old('add','Other task','--id','beta','--status','active');
 const surface=id(),manifest=old('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Planning',surfaces:[{id:surface,kind:'browser',label:'Planning browser',required:true,restore_policy:'manual'}]}));
 await ingress(message({type:'hello',extension_version:'0.2.0',label:'Synthetic browser'}));old('browser','pair',profile);await inventory();
 const legacyOp=old('browser','open','--profile',profile,'--epoch',epoch,'--url','https://legacy.example.test/');await ingress(message({type:'poll'}));
 await ingress(message({type:'command_result',result:{operation_id:legacyOp.id,status:'succeeded',tab_id:10,window_id:10,url:'https://legacy.example.test/'}}));
 const legacyPoint=join(dir,'schema15.db');if(legacy)old('backup','--output',legacyPoint);
 await stopProcess(daemon);await start();
 assert.equal(cli('browser','status').legacy_actions[legacyOp.id].execution,'api_reported');assert.equal(cli('browser','status').legacy_actions[legacyOp.id].verification,'unsupported');
 const c=cli('action','context','alpha'),request={version:1,id:id(),target:'alpha',expected_task_revision:c.task_revision,manifest_id:c.manifest_id,surface_id:surface,context_digest:c.context_digest,browser:{profile,epoch,action:'open',url:'https://action.example.test/'}};
 assert.notEqual(refused('action','queue','alpha','--file',input('unsupported',request)).status,0);
 connection=id();await ingress(message({type:'hello',extension_version:'0.4.0',action_protocol:1,verification_protocol:1,label:'Synthetic browser'}));await inventory();
 const file=input('request',request),queued=cli('action','queue','alpha','--file',file);assert.equal(queued.execution,'queued');assert.equal(queued.verification,'pending');assert(queued.intent.attempt_id);
 assert.deepEqual(cli('action','queue','alpha','--file',file),queued);assert.equal(cli('action','show','alpha','--id',request.id).last_event_id,queued.last_event_id);
 assert.notEqual(refused('action','queue','alpha','--file',input('conflicting',{...request,id:id()})).status,0);
 const challenge=await ingress(message({type:'poll'}));assert(challenge.challenge);await ingress(message({type:'readback',challenge_id:challenge.challenge.id,seq:++sequence,observed_at:new Date().toISOString(),tabs:[],present_tabs:[],instances:[],stable:true,complete:true,focused_window:1}));
 const poll=message({type:'poll'}),delivery=await ingress(poll);assert.equal(delivery.commands.length,1);const ref=delivery.commands[0].action_ref;assert.equal(ref.id,request.id);assert.equal(ref.attempt_id,queued.intent.attempt_id);
 assert.equal(cli('action','show','alpha','--id',request.id).execution,'dispatching');assert.deepEqual(await ingress(poll),delivery);assert.equal((await ingress(message({type:'poll'}))).commands?.length||0,0);
 await stopProcess(daemon,'SIGKILL');await start();let action=cli('action','show','alpha','--id',request.id);assert.equal(action.execution,'uncertain');assert.equal(action.verification,'unknown');assert(action.uncertain_since);
 connection=id();await ingress(message({type:'hello',extension_version:'0.4.0',action_protocol:1,verification_protocol:1,label:'Reconnected synthetic browser'}));await inventory();assert.equal((await ingress(message({type:'poll'}))).commands?.length||0,0);
 const cancel={version:1,id:id(),target:'alpha',action_id:request.id,expected_revision:action.revision,reason:'Cancel without assuming dispatched input stopped'};
 action=cli('action','cancel','alpha','--file',input('cancel',cancel));assert.equal(action.execution,'uncertain');assert.equal(action.cancel_requested,true);
 assert.notEqual(refused('action','queue','alpha','--file',input('still-conflicting',{...request,id:id()})).status,0);
 const result={operation_id:request.id,action_ref:ref,status:'succeeded',tab_id:20,window_id:20,url:request.browser.url};await ingress(message({type:'command_result',result:{...result,action_ref:{...ref,attempt_id:id()}}}),400);await ingress(message({type:'command_result',result}));
 action=cli('action','show','alpha','--id',request.id);assert.equal(action.execution,'api_reported');assert.equal(action.verification,'pending');assert.equal(action.cancel_requested,true);assert(action.uncertain_since);
 await inventory([{id:20,window_id:20,url:request.browser.url,title:'Synthetic owned tab',active:true}]);
 assert.notEqual(refused('browser','close','--profile',profile,'--epoch',epoch,'--tab','20','--expected-url',request.browser.url).status,0);
 assert.notEqual(refused('action','show','beta','--id',request.id).status,0);assert.equal(cli('action','list','alpha').length,1);const history=cli('action','history','alpha','--id',request.id);assert.equal(history.length,5);
 const credential=join(dir,'reader.credential.json');cli('grant','issue','alpha','--name','No action authority','--expires',new Date(Date.now()+3600000).toISOString(),'--output',credential);
 const ep=JSON.parse(fs.readFileSync(join(data,'endpoint.json'))),reader=JSON.parse(fs.readFileSync(credential)),browser=JSON.parse(fs.readFileSync(join(data,'browser-endpoint.json')));
 for(const token of [reader.token,browser.token])for(const path of ['context','show','list','history','queue','cancel','reconcile']){const response=await fetch(ep.url+'/action/'+path+'?target=alpha&id='+request.id,{headers:{authorization:'Bearer '+token}});assert.equal(response.status,401);}
 const state=cli('state');cli('replay');assert.deepEqual(cli('state'),state);assert.equal(state.tasks.alpha.task.status,'active');
 fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');const backup=join(dir,'schema17.db');cli('backup','--output',backup);
 await stopProcess(daemon);const original=data;data=join(dir,'restored');fs.mkdirSync(data);fs.copyFileSync(backup,join(data,'heimdall.db'));fs.copyFileSync(join(original,'types.yaml'),join(data,'types.yaml'));await start();assert.deepEqual(cli('state'),state);assert.deepEqual(cli('action','queue','alpha','--file',file),queued);
 console.log(JSON.stringify({status:'passed',legacy:!!legacy,checks:['legacy browser success remains unverified','intent and attempt committed before delivery','surface conflict and exact retry','SIGKILL after delivery leaves uncertainty','cancel in flight and late API report remain distinct','wrong attempt and cross-task refusal','legacy control cannot bypass scoped ownership','history, replay and backup restore','CLI-only action authority'],data:dir},null,2));
}finally{await stopProcess(daemon);}})().catch(e=>{console.error(e);process.exitCode=1;});
