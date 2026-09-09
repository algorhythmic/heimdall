// W04 compiled CLI/daemon acceptance. Linux uses synthetic read-only IPC;
// other platforms verify explicit unavailable coverage and local authority.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs');const {join}=require('node:path');const os=require('node:os');
const {randomBytes}=require('node:crypto');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {exe,dir}=smokePaths('preview');const data=join(dir,'data');let daemon,fake,socketDir;
const id=()=>randomBytes(16).toString('hex');
const raw=(...args)=>execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const cli=(...args)=>JSON.parse(raw(...args,'--json'));
const refused=(...args)=>spawnSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const input=(name,v)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(v));return path;};
const ready=p=>new Promise((resolve,reject)=>{let errors='';const timer=setTimeout(()=>reject(Error(errors||'ready timeout')),10000);p.stderr.on('data',b=>errors+=b);p.stdout.once('data',()=>{clearTimeout(timer);resolve();});p.once('error',e=>{clearTimeout(timer);reject(e);});p.once('exit',c=>{clearTimeout(timer);reject(Error('exit '+c+errors));});});
async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await ready(daemon);}
const window=(stableId,address,workspace=1)=>({stableId,address,pid:123,class:'same-app',title:'Duplicate title',mapped:true,monitor:0,workspace:{id:workspace,name:workspace===1?'planning':'review'},at:[10,20],size:[800,600]});
const fixture={version:{version:'0.56.2',dirty:false},monitors:[{id:0,name:'SYNTHETIC-1',x:0,y:0,width:1920,height:1080,scale:1,transform:0,activeWorkspace:{id:1}}],workspaces:[{id:1,name:'planning',monitor:'SYNTHETIC-1',monitorID:0},{id:2,name:'review',monitor:'SYNTHETIC-1',monitorID:0}],clients:[window('18000001','0x64'),window('18000002','0x65')]};
(async()=>{try{
 cli('init');await start();cli('add','Preview task','--id','alpha','--status','active');cli('add','Other task','--id','beta','--status','active');
 const surface=id();const manifest=cli('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Planning',surfaces:[{id:surface,kind:'terminal',label:'Shell',required:true,restore_policy:'manual'}]}));
 let diff=cli('workspace','diff','alpha');assert(diff.issues.includes('snapshot_missing'));assert.equal(diff.fresh,false);
 const credential=join(dir,'reader.credential.json');cli('grant','issue','alpha','--name','Read only context','--expires',new Date(Date.now()+3600000).toISOString(),'--output',credential);
 const ep=JSON.parse(fs.readFileSync(join(data,'endpoint.json'))),reader=JSON.parse(fs.readFileSync(credential)),browser=JSON.parse(fs.readFileSync(join(data,'browser-endpoint.json')));
 for(const token of [reader.token,browser.token])for(const path of ['list','diff','preview','validate']){
  const response=await fetch(ep.url+'/workspace/'+path+'?target=alpha',{headers:{authorization:'Bearer '+token}});assert.equal(response.status,401);
 }
 if(process.platform!=='linux'){assert.equal(cli('workspace','list','alpha').surfaces[0].status,'unavailable');console.log(JSON.stringify({status:'passed',platform:process.platform,checks:['scoped metadata with unavailable compositor','preview routes require local CLI authority']}));return;}
 socketDir=fs.mkdtempSync(join(os.tmpdir(),'heimdall-hypr-'));const stateFile=input('compositor',fixture),audit=join(dir,'ipc-reads.txt');
 fake=spawn('python3',[join(__dirname,'hyprland-fixture.py'),socketDir,stateFile,audit],{stdio:['pipe','pipe','pipe']});await ready(fake);
 const probe=cli('viewport','probe','--socket-dir',socketDir);
 const source=cli('viewport','select','--file',input('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
 const inventory=cli('viewport','inventory');cli('viewport','bind','alpha','--file',input('binding',{version:1,id:id(),op:'bind',target:'alpha',previous:'none',expected_task_revision:1,binding:{manifest_id:manifest.id,surface_id:surface,source_id:source.id,snapshot_id:inventory.snapshot.id,window:inventory.snapshot.windows[0].identity}}));
 const status=cli('snapshot','status','alpha');const point=cli('snapshot','capture','alpha','--file',input('capture',{version:1,id:id(),op:'capture',target:'alpha',previous:'none',expected_task_revision:1,manifest_id:manifest.id,source_id:source.id,input_digest:status.input_digest}));
 const before=cli('state'),events=cli('events');const planFile=join(dir,'preview.json');
 const preview=cli('workspace','preview','alpha','--manifest',manifest.id,'--snapshot',point.id,'--output',planFile);
 assert.equal(preview.fresh,true);assert.equal(preview.surfaces[0].disposition,'leave-open');assert.equal(preview.unowned_left_open,1);assert(!JSON.stringify(preview).includes('Duplicate title'));
 assert.equal(fs.statSync(planFile).mode&0o777,0o600);assert.deepEqual(JSON.parse(fs.readFileSync(planFile)),preview);
 assert.equal(cli('workspace','validate','alpha','--file',planFile).current,true);
 assert.match(raw('workspace','preview','alpha','--manifest',manifest.id,'--snapshot',point.id),/leave-open/);
 assert.equal(cli('workspace','list','alpha').surfaces[0].status,'observed');
 assert.notEqual(refused('workspace','preview','beta','--manifest',manifest.id,'--snapshot',point.id).status,0);
 assert.notEqual(refused('workspace','preview','alpha','--manifest',manifest.id,'--snapshot',point.id,'--output',planFile).status,0);
 const forged=structuredClone(preview);forged.surfaces[0].disposition='launch';assert.equal(cli('workspace','validate','alpha','--file',input('forged',forged)).issue,'preview_not_issued_by_this_daemon');
 fixture.clients[0]=window('18000001','0x64',2);input('compositor',fixture);
 diff=cli('workspace','diff','alpha');assert.equal(diff.surfaces[0].disposition,'move');assert.equal(diff.surfaces[0].desired.workspace,'planning');assert.equal(diff.surfaces[0].observed.workspace,'review');
 assert.equal(cli('workspace','validate','alpha','--file',planFile).current,false);
 fixture.clients=[window('18000009','0x64')];input('compositor',fixture);assert.equal(cli('workspace','diff','alpha').surfaces[0].disposition,'launch');
 fixture.monitors[0].scale=1.25;input('compositor',fixture);diff=cli('workspace','diff','alpha');assert.equal(diff.surfaces[0].disposition,'review-required');assert(diff.surfaces[0].issues.includes('display_geometry_changed'));
 assert.deepEqual(cli('state'),before);assert.deepEqual(cli('events'),events);
 fs.writeFileSync(join(dir,'review.json'),JSON.stringify(diff,null,2)+'\n');
 await stopProcess(daemon);await start();assert.equal(cli('workspace','validate','alpha','--file',planFile).issue,'preview_not_issued_by_this_daemon');assert.deepEqual(cli('state'),before);
 for(const command of fs.readFileSync(audit,'utf8').trim().split('\n'))assert(['j/version','j/clients','j/workspaces','j/monitors'].includes(command));
 console.log(JSON.stringify({status:'passed',checks:['scoped live list and deterministic preview','private explicit plan file','stale and forged preview refusal','stable identity beats duplicate title and address reuse','changed display requires review','no durable mutation or compositor action','daemon restart invalidates preview','local CLI authority'],data:dir},null,2));
}finally{await stopProcess(daemon);await stopProcess(fake);if(socketDir)fs.rmSync(socketDir,{recursive:true,force:true});}})().catch(e=>{console.error(e);process.exitCode=1;});
