// P03 manual preservation: only synthetic originals, clones and a local bare remote.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs');const {join}=require('node:path');const {randomBytes}=require('node:crypto');const assert=require('node:assert/strict');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
if(process.platform!=='linux'){console.log('P03 observation requires Linux; replay remains portable');process.exit(0);}
const {exe,dir}=smokePaths('preservation');let data=join(dir,'data'),daemon;
const id=()=>randomBytes(16).toString('hex');
const raw=(...args)=>execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
const cli=(...args)=>JSON.parse(raw(...args));
const input=(name,v)=>{const p=join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(v));return p;};
const git=(...args)=>execFileSync('git',args,{encoding:'utf8',stdio:['ignore','pipe','pipe'],env:{...process.env,GIT_CONFIG_GLOBAL:'/dev/null',GIT_CONFIG_SYSTEM:'/dev/null'}}).trim();
async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await new Promise((resolve,reject)=>{let errors='';const timer=setTimeout(()=>reject(Error(errors)),10000);daemon.stderr.on('data',b=>errors+=b);daemon.stdout.once('data',()=>{clearTimeout(timer);resolve();});daemon.once('error',e=>{clearTimeout(timer);reject(e);});daemon.once('exit',c=>{clearTimeout(timer);reject(Error('daemon exit '+c+errors));});});}
(async()=>{try{
  cli('init');await start();cli('add','Preserve planning','--id','alpha','--status','active');
  const work=join(dir,'work');fs.mkdirSync(work);const original=join(work,'notes.md');fs.writeFileSync(original,'Design without a code commit');
  const resource=cli('resource','bind','alpha','--expected-task-revision','1','--file',input('resource',{kind:'tree',root:work,path:'.'}));
  const c=cli('contract','accept','alpha','--expected-task-revision','1','--file',input('contract',{previous:'none',objective:'Preserve exact planning work',resource_ids:[resource.id]}));
  const a=cli('artifact','record','alpha','--expected-task-revision','1','--file',input('artifact',{artifact_id:'new',previous:'none',name:'Notes',environment:'local',resource_id:resource.id,path:'notes.md',git:false}));
  const cp=cli('checkpoint','create','alpha','--expected-task-revision','1','--file',input('checkpoint',{previous:'none',contract_id:c.id,summary:'Saved before remote work',next_action:'Review later',blockers:[],artifacts:[{artifact_id:a.artifact.id,version_id:a.record.id}]}));
  const clone=join(dir,'.private'),remote=join(dir,'remote.git');git('init','-b','main',clone);git('init','--bare',remote);git('-C',clone,'remote','add','origin',remote);
  fs.writeFileSync(join(clone,'README.md'),'Synthetic private clone');git('-C',clone,'add','README.md');const commit=()=>git('-C',clone,'-c','user.name=Fixture','-c','user.email=fixture@example.invalid','commit','-m','Synthetic preservation');commit();
  const mirrorPath='alpha/ingested/local/notes.md';const request={version:1,id:id(),target:'alpha',expected_task_revision:1,plan:{checkpoint_id:cp.id,private_root:clone,remote:'origin',ref:'refs/heads/main',selections:[{artifact_id:a.artifact.id,version_id:a.record.id,mirror_path:mirrorPath}]}};
  const before=cli('events');const preview=cli('preservation','preview','alpha','--file',input('preview',request));assert.equal(preview.snapshot.stage,'not_preserved');assert.deepEqual(cli('events'),before);
  request.preview_digest=preview.digest;const requestPath=input('request',request);const plan=cli('preservation','request','alpha','--file',requestPath);
  assert.deepEqual(cli('checkpoint','show','alpha'),cp);assert.equal(cli('preservation','list','alpha')[0].plan.id,plan.id);
  const mirror=join(clone,mirrorPath);fs.mkdirSync(join(clone,'alpha/ingested/local'),{recursive:true});fs.copyFileSync(original,mirror);
  let previous='none';let lastPath,last;
  const observe=(outcome,note,check_remote)=>{const req={version:1,id:id(),target:'alpha',expected_task_revision:1,observe:{plan_id:plan.id,previous,reported_outcome:outcome,note,check_remote}};lastPath=input(req.id,req);last=cli('preservation','observe','alpha','--file',lastPath);previous=last.id;return last;};
  assert.equal(observe('not_reported','',false).snapshot.stage,'mirrored');
  git('-C',clone,'add',mirrorPath);commit();assert.equal(observe('push_failed','Manual push failed',true).snapshot.stage,'committed');
  git('-C',clone,'push','origin','main');assert.equal(observe('succeeded','Manual push completed',true).snapshot.stage,'published');
  const portable=cli('preservation','export','alpha','--checkpoint',cp.id);assert.equal(portable.artifacts[0].digest,a.record.observation.digest);assert(!JSON.stringify(portable).includes(clone));
  fs.rmSync(original);assert.equal(observe('missing_original','Original removed',false).snapshot.items[0].source.status,'missing');
  fs.renameSync(remote,remote+'-offline');const offline=observe('offline','Remote unavailable',true);assert.equal(offline.snapshot.remote_status,'unavailable');assert.equal(offline.snapshot.stage,'committed');
  const state=cli('state');fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');assert.deepEqual(cli('checkpoint','show','alpha'),cp);assert.equal(cli('state','alpha').task.status,'active');
  const rejected=spawnSync(exe,['preservation','show','beta','--id',plan.id,'--data-dir',data],{encoding:'utf8'});assert.notEqual(rejected.status,0);
  await stopProcess(daemon,'SIGKILL');fs.rmSync(clone,{recursive:true});await start();assert.deepEqual(cli('preservation','request','alpha','--file',requestPath),plan);assert.deepEqual(cli('preservation','observe','alpha','--file',lastPath),last);
  cli('replay');assert.deepEqual(cli('state'),state);const snapshot=join(dir,'snapshot.db');cli('backup','--output',snapshot);await stopProcess(daemon);
  const source=data;data=join(dir,'restored');fs.mkdirSync(data);fs.copyFileSync(snapshot,join(data,'heimdall.db'));fs.copyFileSync(join(source,'types.yaml'),join(data,'types.yaml'));await start();assert.deepEqual(cli('state'),state);cli('replay');assert.deepEqual(cli('state'),state);assert.deepEqual(cli('preservation','observe','alpha','--file',lastPath),last);
  console.log(JSON.stringify({status:'passed',checks:['read-only preview','checkpoint-linked exact versions','mirror/commit/remote facts','remote failure and missing source','portable allowlist export','target isolation','SIGKILL/restart exact receipts','replay and backup restore without originals'],data:dir},null,2));
}finally{await stopProcess(daemon);}})().catch(e=>{console.error(e);process.exitCode=1;});
