// Real synthetic daemon data for the reference TUI. No user database is touched.
const fs=require('node:fs');
const {join,resolve}=require('node:path');
const {spawn,execFileSync}=require('node:child_process');
const {randomBytes}=require('node:crypto');
const {stopProcess}=require('./smoke-paths.cjs');
const id=()=>randomBytes(16).toString('hex');
async function seed(exe,dir){
 const data=join(dir,'data');fs.mkdirSync(dir,{recursive:true});
 const raw=(...args)=>execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'});
 const cli=(...args)=>JSON.parse(raw(...args));
 const input=(name,value)=>{const p=join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(value));return p;};
 cli('init');const catalogPath=join(data,'types.yaml');fs.writeFileSync(catalogPath,fs.readFileSync(catalogPath,'utf8').replace('[backlog, active, blocked, review, done, dropped]','[backlog, active, waiting, parked, blocked, review, done, dropped]'));const daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});
 await new Promise((res,rej)=>{let errors='';const timer=setTimeout(()=>rej(Error('daemon readiness: '+errors)),10000);daemon.stderr.on('data',b=>errors+=b);daemon.stdout.once('data',()=>{clearTimeout(timer);res()});daemon.once('exit',c=>{clearTimeout(timer);rej(Error('daemon exit '+c+' '+errors))});daemon.once('error',e=>{clearTimeout(timer);rej(e)})});
 try{
  const doc={version:1,revision:0,tasks:[
   {id:'jobapp-sensemesh',title:'Sensemesh follow-up',type:'project',status:'waiting',resume_by:'2026-09-04',next_action:'Send the follow-up email',done:{text:'Application follow-up sent'},subtasks:[{id:'prepare',title:'Prepare the application',status:'done',done:{text:'Application ready'}},{id:'follow-up',title:'Send follow-up',status:'open',done:{text:'Email sent'}}]},
   {id:'video-series',title:'Ship a 90-second intro',type:'project',status:'active',next_action:'Cut the intro to 90 seconds',done:{text:'The intro is reviewed and ready'},subtasks:[{id:'storyboard',title:'Storyboard the cut',status:'open',done:{text:'The storyboard is reviewed',checks:[{id:'files',kind:'artifact.exists'}]}},{id:'cut-intro',title:'Cut the intro to 90 seconds',status:'open',after:['storyboard'],estimate_minutes:30,done:{text:'The cut is reviewed'}},{id:'upload',title:'Upload and schedule',status:'open',after:['cut-intro'],done:{text:'Upload independently observed'}}]},
   {id:'fuse-f1',title:'Bench the new tokenizer',type:'project',status:'active',next_action:'Bench the new tokenizer',done:{text:'Benchmark reviewed'}},
   {id:'huginn-migration',title:'Move agents to the scheduler',type:'project',status:'parked',next_action:'Move agents to the scheduler',done:{text:'Migration verified'}},
   {id:'reelforge-docs',title:'Rewrite the install page',type:'project',status:'parked',next_action:'Rewrite the install page',done:{text:'Install steps tested'}}
  ]};
  // The default project catalog determines legal lifecycle names.
  const catalog=fs.readFileSync(join(data,'types.yaml'),'utf8');
  if(!catalog.includes('parked')){doc.tasks.forEach(t=>{if(t.status==='parked')t.status='waiting'})}
  const path=join(dir,'tasks.yaml');fs.writeFileSync(path,JSON.stringify(doc));cli('import-tasks',path);
  const work=join(dir,'work');fs.mkdirSync(work);fs.writeFileSync(join(work,'intro.edl'),'A 90-second intro\n');fs.writeFileSync(join(work,'timeline.py'),'# timeline trim to 2:10\n');
  const st=cli('state');const revision=target=>String(st.tasks[target].revision);
  const resource=cli('resource','bind','video-series','--expected-task-revision',revision('video-series'),'--file',input('resource',{kind:'tree',root:work,path:'.'}));
  const contract=cli('contract','accept','video-series','--expected-task-revision',revision('video-series'),'--file',input('contract',{previous:'none',objective:'Ship a 90-second intro; keep the storyboard as the reference cut.',resource_ids:[resource.id]}));
  cli('checkpoint','create','video-series','--expected-task-revision',revision('video-series'),'--file',input('checkpoint',{previous:'none',contract_id:contract.id,summary:'Storyboard approved; timeline trimmed to 2:10, music not yet cut.',next_action:'Cut the intro to 90 seconds and commit the timeline.',current_step:'cut-intro',blockers:[]}));
  cli('progress','propose','video-series','--expected-task-revision',revision('video-series'),'--file',input('decision',{kind:'decision',text:'Drop the music bed; keep the narration clear.',contract_id:contract.id}));
  cli('capture','unassigned/reference: pricing page for follow-up','--pointer','https://example.test/pricing');
  const surface=id();cli('workspace','accept','video-series','--expected-task-revision',revision('video-series'),'--file',input('workspace',{previous:'none',name:'Video production',surfaces:[{id:surface,kind:'terminal',label:'video-prod · edit timeline',required:true,restore_policy:'manual'}]}));
  fs.appendFileSync(join(work,'timeline.py'),'# changed after checkpoint\n');
  const stepContract=cli('contract','accept','video-series#storyboard','--expected-task-revision',revision('video-series'),'--file',input('step-contract',{previous:'none',objective:'Review the storyboard files',resource_ids:[resource.id]}));
  const check=cli('evidence','configure','video-series#storyboard','--expected-task-revision',revision('video-series'),'--file',input('check',{check_id:'files',contract_id:stepContract.id,previous:'none',spec:{kind:'artifact.exists',resource_id:resource.id}}));
  const attempt=cli('evidence','evaluate','video-series#storyboard','--expected-task-revision',revision('video-series'),'--evaluator',check.id);
  for(let tries=0;tries<100;tries++){if(cli('evidence','list','video-series#storyboard').evidence.find(e=>e.id===attempt.id)?.status==='finished')break;await new Promise(r=>setTimeout(r,50));}
  cli('tick');
  return {data,daemon,raw,cli,dir};
 }catch(e){await stopProcess(daemon);throw e}
}
module.exports={seed};
if(require.main===module){(async()=>{
 const root=resolve(__dirname,'..'),exe=join(root,'bin',process.platform==='win32'?'heimdall.exe':'heimdall');
 const dir=fs.mkdtempSync(join(root,'.tools','tui-demo-'));let fixture;
 try{fixture=await seed(exe,dir);if(process.argv.includes('--seed-only')){console.log(JSON.stringify({data:fixture.data,dir},null,2));return}
  await new Promise((res,rej)=>{const p=spawn(exe,['tui','--data-dir',fixture.data],{stdio:'inherit'});p.once('exit',res);p.once('error',rej)});
 }finally{if(fixture)await stopProcess(fixture.daemon)}
})().catch(e=>{console.error(e);process.exitCode=1})}
