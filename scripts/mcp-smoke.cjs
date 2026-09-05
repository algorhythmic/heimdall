// Compiled stdio server: real legacy MCP handshake, scoped writes and reconnect.
const {spawn,execFileSync,spawnSync}=require('node:child_process');
const fs=require('node:fs'),{join,resolve}=require('node:path'),{randomBytes}=require('node:crypto'),assert=require('node:assert/strict');
const id=()=>randomBytes(16).toString('hex');
(async()=>{
 const root=resolve(__dirname,'..'),exe=join(root,'bin','heimdall.exe');fs.mkdirSync(join(root,'.tools'),{recursive:true});
 const dir=fs.mkdtempSync(join(root,'.tools','mcp-test-')),data=join(dir,'data');let daemon;const adapters=[];
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8',windowsHide:true}));
 const input=(name,body)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(body));return path;};
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{windowsHide:true,stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{const t=setTimeout(()=>rej(Error('daemon startup timeout')),10000);daemon.stdout.once('data',()=>{clearTimeout(t);res()});daemon.once('error',e=>{clearTimeout(t);rej(e)});daemon.once('exit',c=>{clearTimeout(t);rej(Error('daemon exit '+c))});});}
 async function stop(p){if(!p||p.exitCode!==null)return;await new Promise(res=>{p.once('exit',res);p.kill();});}
 function adapter(credential){
  const p=spawn(exe,['mcp','--credential',credential],{windowsHide:true,stdio:['pipe','pipe','pipe']});adapters.push(p);
  let sequence=0,buffer='',stderr='';const pending=new Map();p.stderr.on('data',b=>stderr+=b);
  p.stdout.on('data',b=>{buffer+=b;while(buffer.includes('\n')){const at=buffer.indexOf('\n'),line=buffer.slice(0,at);buffer=buffer.slice(at+1);try{const r=JSON.parse(line);pending.get(r.id)?.(r);pending.delete(r.id);}catch(e){for(const resolve of pending.values())resolve({error:{message:'non-protocol stdout: '+e.message}});pending.clear();}}});
  p.on('exit',c=>{for(const resolve of pending.values())resolve({error:{message:'adapter exit '+c+' '+stderr}});pending.clear();});
  const request=(method,params)=>new Promise((resolve,reject)=>{const n=++sequence,t=setTimeout(()=>{pending.delete(n);reject(Error('MCP timeout '+method+' '+stderr))},10000);pending.set(n,r=>{clearTimeout(t);resolve(r)});p.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');});
  const notify=(method,params={})=>p.stdin.write(JSON.stringify({jsonrpc:'2.0',method,params})+'\n');
  return {p,request,notify,stderr:()=>stderr};
 }
 async function handshake(a){const r=await a.request('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'heimdall-stdio-fixture',version:'1'}});assert(!r.error,JSON.stringify(r));assert.equal(r.result.protocolVersion,'2025-11-25');a.notify('notifications/initialized');assert.equal((await a.request('tools/list',{})).result.tools.length,4);}
 async function tool(a,name,args,wantError=false){const r=await a.request('tools/call',{name,arguments:args});assert(!r.error,JSON.stringify(r));assert.equal(!!r.result.isError,wantError,JSON.stringify(r));return r.result;}
 try{
  cli('init');await start();cli('add','MCP fixture','--id','mcp-fixture','--type','project','--status','active');const task=cli('state','mcp-fixture');
  const contract=cli('contract','accept','mcp-fixture','--expected-task-revision',String(task.revision),'--file',input('contract',{previous:'none',objective:'Verify MCP continuity',constraints:[]}));
  const expires=new Date(Date.now()+3600000).toISOString(),writerFile=join(dir,'writer.credential.json'),readerFile=join(dir,'reader.credential.json');
  const grant=cli('grant','issue','mcp-fixture','--name','MCP writer','--expires',expires,'--checkpoint-write','--output',writerFile);
  cli('grant','issue','mcp-fixture','--name','MCP reader','--expires',expires,'--output',readerFile);
  const writer=adapter(writerFile),reader=adapter(readerFile);await handshake(writer);await handshake(reader);
  assert.equal((await tool(writer,'heimdall_task',{target:'mcp-fixture'})).structuredContent.task.id,'mcp-fixture');
  const args={target:'mcp-fixture',request_id:id(),expected_task_revision:task.revision,previous:'none',contract_id:contract.id,summary:'Progress through stdio',next_action:'Review evidence',blockers:[]};
  await tool(reader,'heimdall_checkpoint',args,true);
  const recorded=(await tool(writer,'heimdall_checkpoint',args)).structuredContent;assert.equal(recorded.actor,'client:'+grant.id);assert.equal(recorded.grant_id,grant.id);
  assert.deepEqual((await tool(writer,'heimdall_checkpoint',args)).structuredContent,recorded);
  await tool(writer,'heimdall_checkpoint',{...args,request_id:id()},true); // stale head
  await tool(writer,'heimdall_task',{target:'other-project'},true);
  const secondWriter=spawnSync(exe,['init','--data-dir',data],{encoding:'utf8',windowsHide:true});assert.notEqual(secondWriter.status,0);assert.match(secondWriter.stderr,/owns this data directory/);
  await stop(daemon);const offline=await tool(writer,'heimdall_task',{target:'mcp-fixture'},true);assert.equal(offline.structuredContent.code,'daemon_unavailable');
  await start();assert.deepEqual((await tool(writer,'heimdall_checkpoint',args)).structuredContent,recorded);
  const before=cli('state');cli('replay');assert.deepEqual(cli('state'),before);assert.equal(cli('state','mcp-fixture').task.status,'active');
  cli('grant','revoke',grant.id);const denied=await tool(writer,'heimdall_checkpoint',args,true);assert.equal(denied.structuredContent.code,'access_denied');
  await stop(daemon);await start();await tool(writer,'heimdall_checkpoint',args,true);
  // The ordinary scoped CLI uses the same endpoint and authority checks.
  const deniedCLI=spawnSync(exe,['client','checkpoint','mcp-fixture','--credential',writerFile,'--request-id',args.request_id,'--expected-task-revision',String(task.revision),'--file',input('cp',{previous:'none',contract_id:contract.id,summary:args.summary,next_action:args.next_action,blockers:[]})],{encoding:'utf8',windowsHide:true});assert.notEqual(deniedCLI.status,0);assert.match(deniedCLI.stderr,/403/);
  for(const file of [writerFile,readerFile])assert(!JSON.stringify(cli('events')).includes(JSON.parse(fs.readFileSync(file,'utf8')).token));
  console.log(JSON.stringify({status:'passed',protocol:'2025-11-25',checks:['compiled stdio discovery and tool calls','read/write grant separation','grant-bound provenance','exact retries and stale-head conflict','cross-project denial','sole database writer','daemon offline and restart rediscovery','replay equality','revoked retries denied after restart','task completion unchanged','tokens absent from events'],data:dir},null,2));
 }finally{for(const p of adapters)await stop(p);await stop(daemon);}
})().catch(e=>{console.error(e);process.exitCode=1;});
