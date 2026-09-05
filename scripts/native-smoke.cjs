// Compiled daemon + compiled native host, with a framed synthetic browser client.
const {spawn,execFileSync}=require('node:child_process');
const {mkdtempSync,mkdirSync,readFileSync,existsSync}=require('node:fs');
const {join,resolve}=require('node:path');
const {randomBytes}=require('node:crypto');
const assert=require('node:assert/strict');
const id=()=>randomBytes(16).toString('hex');
(async()=>{
 const root=resolve(__dirname,'..'),exe=resolve(process.argv[2]??join(root,'bin','heimdall.exe'));mkdirSync(join(root,'.tools'),{recursive:true});const dir=mkdtempSync(join(root,'.tools','native-test-'));const data=join(dir,'data');
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8',windowsHide:true}));
 const extension=readFileSync(join(root,'extension','extension-id.txt'),'utf8').trim();cli('init');const setup=cli('browser','setup','--extension-id',extension,'--output',join(dir,'host'));assert.equal(setup.status,'prepared_not_registered');
 let daemon,host;
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{windowsHide:true,stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{daemon.stdout.once('data',res);daemon.once('error',rej);daemon.once('exit',code=>rej(Error('daemon exit '+code)));});}
 async function stop(p){if(!p||p.exitCode!==null)return;await new Promise(res=>{p.once('exit',res);p.kill();});}
 try{
  await start();const a=JSON.parse(readFileSync(join(data,'endpoint.json'))),b=JSON.parse(readFileSync(join(data,'browser-endpoint.json')));assert.notEqual(a.token,b.token);
  host=spawn(join(dir,'host','heimdall-browser-host'+(process.platform==='win32'?'.exe':'')),[`chrome-extension://${extension}/`],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  let buffer=Buffer.alloc(0);const responses=[];host.stdout.on('data',chunk=>{buffer=Buffer.concat([buffer,chunk]);while(buffer.length>=4&&buffer.length>=4+buffer.readUInt32LE(0)){const n=buffer.readUInt32LE(0);responses.shift()?.(JSON.parse(buffer.subarray(4,4+n)));buffer=buffer.subarray(4+n);}});
  const profile=id(),epoch=id(),connection=id();
  const send=body=>new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(Error('native reply timed out')),7000);responses.push(r=>{clearTimeout(timer);resolve(r);});const raw=Buffer.from(JSON.stringify({v:1,id:id(),profile,epoch,connection,...body}));const header=Buffer.alloc(4);header.writeUInt32LE(raw.length);host.stdin.write(Buffer.concat([header,raw]));});
  assert.equal((await send({type:'hello',extension_version:'0.2.0'})).type,'pairing_required');cli('browser','pair',profile);
  const inventory={type:'inventory',seq:1,observed_at:new Date().toISOString(),tabs:[{id:1,window_id:1,url:'https://example.test/',title:'Native fixture',active:true}],focused_window:1,complete:true};assert.equal((await send(inventory)).type,'ack');
  const op=cli('browser','open','--profile',profile,'--epoch',epoch,'--url','https://example.test/');assert.equal((await send({type:'poll'})).commands[0].id,op.id);
  assert.equal((await send({type:'command_result',result:{operation_id:op.id,status:'succeeded',tab_id:2,window_id:2,url:'https://example.test/'}})).type,'ack');
  assert.equal(cli('browser','status').operations[op.id].status,'succeeded');
  await stop(daemon);await start();assert.equal((await send({...inventory,seq:2})).type,'ack');assert.equal(cli('browser','status').profiles[profile].last_sequence,2);
  const before=cli('state');cli('replay');assert.deepEqual(cli('state'),before);
  console.log(JSON.stringify({status:'passed',checks:['compiled native host framing','explicit pairing','durable inventory','CLI reverse command and result','separate role credentials','daemon restart and rotated credential discovery','replay equality'],data:dir},null,2));
 }finally{await stop(host);await stop(daemon);}
})().catch(e=>{console.error(e);process.exitCode=1;});
