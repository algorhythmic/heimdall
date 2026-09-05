// Compiled daemon/CLI acceptance with synthetic resources and an explicit Node test.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs'),{join,resolve}=require('node:path'),{randomBytes}=require('node:crypto'),assert=require('node:assert/strict');
const id=()=>randomBytes(16).toString('hex');
(async()=>{
 const root=resolve(__dirname,'..'),exe=join(root,'bin','heimdall.exe');fs.mkdirSync(join(root,'.tools'),{recursive:true});
 const dir=fs.mkdtempSync(join(root,'.tools','evidence-test-')),data=join(dir,'data'),work=join(dir,'work');fs.mkdirSync(work);fs.writeFileSync(join(work,'input.txt'),'accepted input');let daemon;
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8',windowsHide:true}));
 const input=(name,value)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(value));return path;};
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{windowsHide:true,stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{let errors='';daemon.stderr.on('data',b=>errors+=b);const timer=setTimeout(()=>rej(Error('startup timeout '+errors)),10000);daemon.stdout.once('data',()=>{clearTimeout(timer);res()});daemon.once('error',e=>{clearTimeout(timer);rej(e)});daemon.once('exit',c=>{clearTimeout(timer);rej(Error('daemon exited '+c+' '+errors))});});}
 async function stop(){if(daemon&&daemon.exitCode===null)await new Promise(res=>{daemon.once('exit',res);daemon.kill()});}
 const wait=ms=>new Promise(r=>setTimeout(r,ms));
 async function finished(target,evidence){for(let i=0;i<100;i++){const result=cli('evidence','list',target).evidence.find(e=>e.id===evidence);if(result?.status==='finished')return result;await wait(100)}throw Error('evaluation did not finish');}
 try{
  cli('init');await start();cli('add','Evidence fixture','--id','proof','--status','active');
  const doc=cli('ls');doc.tasks.find(t=>t.id==='proof').done={text:'Configured test passes',checks:[{id:'test',kind:'test.exit'}]};cli('import-tasks',input('tasks',doc));
  const task=cli('state','proof'),rev=String(task.revision);
  const resource=cli('resource','bind','proof','--expected-task-revision',rev,'--file',input('resource',{kind:'tree',root:work,path:'.'}));
  const contract=cli('contract','accept','proof','--expected-task-revision',rev,'--file',input('contract',{previous:'none',objective:'Verify a real test invocation',resource_ids:[resource.id]}));
  const counter=join(dir,'executions.txt');const spec={kind:'test.exit',resource_id:resource.id,argv:[process.execPath,'-e',`require('node:fs').appendFileSync(${JSON.stringify(counter)},'x');process.stdout.write('synthetic test passed')`],timeout_seconds:10};
  const definition=cli('evidence','configure','proof','--expected-task-revision',rev,'--file',input('definition',{check_id:'test',contract_id:contract.id,previous:'none',spec}));
  const request=id();const run=()=>cli('evidence','evaluate','proof','--evaluator',definition.id,'--expected-task-revision',rev,'--request-id',request);
  const started=run();const result=await finished('proof',started.id);assert.equal(result.outcome,'matched');assert.equal(result.exit_code,0);assert.equal(result.output_digest.length,64);assert.equal(result.executable_digest.length,64);
  assert.deepEqual(run(),started);assert.equal(fs.readFileSync(counter,'utf8'),'x');
  cli('tick');let state=cli('state');const proposal=Object.values(state.proposals).find(p=>p.target==='proof'&&p.status==='pending');assert(proposal);assert.equal(state.tasks.proof.task.status,'active');
  fs.writeFileSync(join(work,'input.txt'),'changed after test');const refused=spawnSync(exe,['ratify',proposal.id,'--accept','--data-dir',data],{encoding:'utf8',windowsHide:true});assert.notEqual(refused.status,0);assert.match(refused.stderr,/evidence changed/);
  cli('evidence','refresh','proof');cli('tick');state=cli('state');assert.equal(state.proposals[proposal.id].status,'superseded');assert(state.evidence_invalidations[started.id]);
  const saved=cli('state');cli('replay');assert.deepEqual(cli('state'),saved);await stop();await start();assert.deepEqual(cli('state'),saved);assert.deepEqual(run(),started);assert.equal(fs.readFileSync(counter,'utf8'),'x');
  // A fresh explicit identity can evaluate changed inputs; it still requires review.
  const again=cli('evidence','evaluate','proof','--evaluator',definition.id,'--expected-task-revision',rev,'--request-id',id());assert.equal((await finished('proof',again.id)).outcome,'matched');cli('tick');state=cli('state');const current=Object.values(state.proposals).find(p=>p.target==='proof'&&p.status==='pending');assert(current);cli('ratify',current.id,'--accept');assert.equal(cli('state','proof').task.status,'done');
  console.log(JSON.stringify({status:'passed',checks:['compiled configure/evaluate/list','observed test provenance','retry without reexecution','live stale-input denial','invalidation and supersession','replay and restart','explicit rerun and ratified completion'],data:dir},null,2));
 }finally{await stop()}
})().catch(e=>{console.error(e);process.exitCode=1});
