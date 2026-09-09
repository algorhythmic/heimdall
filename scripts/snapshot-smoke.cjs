// W03 compiled snapshot/policy workflow against synthetic compositor IPC.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs');const {join}=require('node:path');const os=require('node:os');
const {randomBytes}=require('node:crypto');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {exe,dir}=smokePaths('snapshot');let data=join(dir,'data'),daemon,fake;
const socketDir=fs.mkdtempSync(join(os.tmpdir(),'heimdall-hypr-'));
const id=()=>randomBytes(16).toString('hex');
const raw=(...args)=>execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const cli=(...args)=>JSON.parse(raw(...args));
const refused=(...args)=>spawnSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const input=(name,v)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(v));return path;};
const ready=p=>new Promise((resolve,reject)=>{let errors='';const timer=setTimeout(()=>reject(Error(errors||'ready timeout')),10000);p.stderr.on('data',b=>errors+=b);p.stdout.once('data',()=>{clearTimeout(timer);resolve();});p.once('error',e=>{clearTimeout(timer);reject(e);});p.once('exit',c=>{clearTimeout(timer);reject(Error('exit '+c+errors));});});
async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await ready(daemon);}
async function until(fn){const deadline=Date.now()+10000;while(Date.now()<deadline){const result=fn();if(result)return result;await new Promise(r=>setTimeout(r,200));}throw Error('snapshot convergence timeout');}
const fixture={version:{version:'0.56.2',dirty:false},monitors:[{id:0,name:'SYNTHETIC-1',x:0,y:0,width:1920,height:1080,scale:1,transform:0,activeWorkspace:{id:1}}],workspaces:[{id:1,name:'planning',monitor:'SYNTHETIC-1',monitorID:0}],clients:[{stableId:'18000001',address:'0x64',pid:123,class:'same-app',title:'Owned window title',mapped:true,monitor:0,workspace:{id:1,name:'planning'},at:[10,20],size:[800,600]}]};
(async()=>{try{
 cli('init');await start();cli('add','Snapshot task','--id','alpha','--status','active');cli('add','Other task','--id','beta','--status','active');
 assert.equal(cli('snapshot','status','alpha').head_available,false);
 if(process.platform!=='linux'){assert.deepEqual(cli('snapshot','list','alpha'),[]);console.log(JSON.stringify({status:'passed',platform:process.platform,checks:['snapshot metadata commands work without native compositor']}));return;}
 const stateFile=input('compositor',fixture);fake=spawn('python3',[join(__dirname,'hyprland-fixture.py'),socketDir,stateFile,join(dir,'ipc-reads.txt')],{stdio:['pipe','pipe','pipe']});await ready(fake);
 const surface=id();const manifest=cli('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Planning',surfaces:[{id:surface,kind:'terminal',label:'Shell',required:true,restore_policy:'manual'}]}));
 const probe=cli('viewport','probe','--socket-dir',socketDir);
 const source=cli('viewport','select','--file',input('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
 const inventory=cli('viewport','inventory');cli('viewport','bind','alpha','--file',input('binding',{version:1,id:id(),op:'bind',target:'alpha',previous:'none',expected_task_revision:1,binding:{manifest_id:manifest.id,surface_id:surface,source_id:source.id,snapshot_id:inventory.snapshot.id,window:inventory.snapshot.windows[0].identity}}));
 const request=()=>{const s=cli('snapshot','status','alpha');return {version:1,id:id(),op:'capture',target:'alpha',previous:s.head?.id||'none',expected_task_revision:s.task_revision,manifest_id:s.manifest_id,source_id:s.source_id,input_digest:s.input_digest};};
 const initial=request(),file=input('capture',initial);const point=cli('snapshot','capture','alpha','--file',file);
 assert(point.published);assert.equal(cli('snapshot','show','alpha','--id',point.id).payload.surfaces[0].window.title,'Owned window title');assert.equal(cli('snapshot','status','alpha').pins.length,1);
 assert(!JSON.stringify(cli('state')).includes('Owned window title'));
 assert.notEqual(refused('snapshot','show','beta','--id',point.id).status,0);
 const stale={...initial,id:id()};assert.match(refused('snapshot','capture','alpha','--file',input('stale',stale)).stderr,/409/);
 const policy={version:1,id:id(),op:'policy',target:'alpha',previous:'none',expected_task_revision:1,manifest_id:manifest.id,source_id:source.id,policy:{enabled:true,debounce_seconds:1,max_dirty_seconds:5,retain_count:2}};
 cli('snapshot','policy','alpha','--file',input('policy',policy));
 let head=point.id,firstAuto;
 for(const x of [20,30,40]){fixture.clients[0].at=[x,20];input('compositor',fixture);fake.stdin.write('movewindowv2>>64,1,planning\n');const next=await until(()=>{const status=cli('snapshot','status','alpha');return status.head?.id!==head&&status.head;});head=next.id;firstAuto??=next.id;}
 assert.equal(cli('snapshot','show','alpha','--id',firstAuto).pruned,true);
 assert.equal(cli('snapshot','show','alpha','--id',point.id).pruned,false);
 const pruneHead={version:1,id:id(),op:'prune',target:'alpha',previous:'none',snapshot_ids:[head],reason:'Cannot prune current head'};assert.notEqual(refused('snapshot','prune','alpha','--file',input('prune-head',pruneHead)).status,0);
 cli('snapshot','unpin','alpha','--file',input('unpin',{version:1,id:id(),op:'unpin',target:'alpha',previous:point.id,snapshot_id:point.id,reason:'Release manual point'}));
 cli('snapshot','prune','alpha','--file',input('prune',{version:1,id:id(),op:'prune',target:'alpha',previous:'none',snapshot_ids:[point.id],reason:'Explicit payload retention'}));
 assert.equal(cli('snapshot','show','alpha','--id',point.id).pruned,true);assert.deepEqual(cli('snapshot','capture','alpha','--file',file),point);
 const page=cli('snapshot','list','alpha','--limit','2');assert.equal(page.length,2);assert(cli('snapshot','list','alpha','--limit','2','--before',String(page[1].event_id)).length>0);
 const credential=join(dir,'reader.credential.json');cli('grant','issue','alpha','--name','No snapshot authority','--expires',new Date(Date.now()+3600000).toISOString(),'--output',credential);
 const ep=JSON.parse(fs.readFileSync(join(data,'endpoint.json'))),reader=JSON.parse(fs.readFileSync(credential)),browser=JSON.parse(fs.readFileSync(join(data,'browser-endpoint.json')));assert(reader.token);
 for(const token of [reader.token,browser.token]){const response=await fetch(ep.url+'/workspace/snapshot/show?target=alpha&id='+head,{headers:{authorization:'Bearer '+token}});assert.equal(response.status,401);}
 fixture.clients=[];input('compositor',fixture);fake.stdin.write('closewindow>>64\n');assert(cli('viewport','inventory').fresh);
 const partial=cli('snapshot','capture','alpha','--file',input('partial',request()));assert.equal(partial.coverage,'partial');assert.equal(partial.published,false);assert.equal(cli('snapshot','status','alpha').head.id,head);
 const state=cli('state');fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');await stopProcess(daemon,'SIGKILL');await start();assert.equal(cli('snapshot','status','alpha').head.id,head);assert.equal(cli('snapshot','status','alpha').head_available,true);assert.deepEqual(cli('state'),state);
 cli('replay');assert.deepEqual(cli('state'),state);assert.equal(cli('snapshot','show','alpha','--id',firstAuto).pruned,true);
 const snapshot=join(dir,'snapshot.db');cli('backup','--output',snapshot);await stopProcess(daemon);const original=data;data=join(dir,'restored');fs.mkdirSync(data);fs.copyFileSync(snapshot,join(data,'heimdall.db'));fs.copyFileSync(join(original,'types.yaml'),join(data,'types.yaml'));await start();assert.deepEqual(cli('state'),state);assert.equal(cli('snapshot','show','alpha','--id',head).payload.surfaces.length,1);assert.deepEqual(cli('snapshot','capture','alpha','--file',file),point);
 console.log(JSON.stringify({status:'passed',checks:['manual capture and durable pins','explicit automatic capture policy','payload retention and protected head','pruned history and inert exact retry','bounded history pagination and scope denial','partial capture preserves complete point','SIGKILL/restart with empty source','replay and backup restore include retained payloads'],data:dir},null,2));
}finally{await stopProcess(daemon);await stopProcess(fake);fs.rmSync(socketDir,{recursive:true,force:true});}})().catch(e=>{console.error(e);process.exitCode=1;});
