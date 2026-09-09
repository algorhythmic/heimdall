// P0 acceptance: edit only tasks.yaml, materialize, explicitly evaluate Go, replay/restart.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),{join}=require('node:path'),assert=require('node:assert/strict');
(async()=>{
 const {root,exe,dir}=require('./smoke-paths.cjs').smokePaths('p0');
 const data=join(dir,'data');let daemon;
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
 const wait=ms=>new Promise(r=>setTimeout(r,ms));
 const stop=()=>require('./smoke-paths.cjs').stopProcess(daemon);
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{let errors='';daemon.stderr.on('data',b=>errors+=b);const timer=setTimeout(()=>rej(Error(errors||'startup timeout')),10000);daemon.stdout.once('data',()=>{clearTimeout(timer);res()});daemon.once('error',rej);});}
 try{
  cli('init');await start();
  const go=process.env.HEIMDALL_GO||join(root,'.tools/go/bin/go');
  // JSON quoting is also valid for YAML scalars. No JSON definition files are written.
  fs.writeFileSync(join(data,'tasks.yaml'),`version: 1\nrevision: 0\ntasks:\n  - id: self-test\n    title: Heimdall self-test\n    type: project\n    status: active\n    next_action: Review check evidence\n    done:\n      text: Internal check tests pass\n      checks:\n        - id: go-tests\n          kind: test.exit\n          path: ${JSON.stringify(root)}\n          argv: [${JSON.stringify(go)}, test, ./internal/checks/...]\n          timeout_seconds: 120\n          exclude: [.tools, bin, heimdall, demo-data, node_modules]\n`);
  cli('sync');
  let state=cli('state');const definition=Object.values(state.evaluators).find(d=>d.target==='self-test');assert(definition);assert.equal(definition.materialized_from,'tasks.yaml@1');assert.equal(Object.keys(state.evidence).length,0);
  const result=cli('evidence','evaluate','self-test','--evaluator',definition.id,'--expected-task-revision',String(state.tasks['self-test'].revision));
  const deadline=Date.now()+140000;let evidence;
  do{state=cli('state');evidence=state.evidence[result.id];if(evidence?.status==='finished')break;await wait(200);}while(Date.now()<deadline);
  assert.equal(evidence?.outcome,'matched',JSON.stringify(evidence));assert.equal(evidence.exit_code,0);
  assert.equal(state.tasks['self-test'].task.status,'active');
  cli('tick');const saved=cli('state');cli('replay');assert.deepEqual(cli('state'),saved);await stop();await start();assert.deepEqual(cli('state'),saved);
  console.log(JSON.stringify({status:'passed',data:dir,evidence:result.id,checks:['tasks.yaml only declaration','resource/contract/evaluator provenance','no execution on save','Go test.exit matched','no automatic completion','replay/restart equality']},null,2));
 }finally{await stop()}
})().catch(e=>{console.error(e);process.exitCode=1});
