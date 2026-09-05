// Whole worker/daemon test with real Chromium APIs and compiled native framing.
// Only Chrome's OS registry discovery/launch is replaced by an in-memory native-port shim.
const {chromium}=require('playwright');
const {spawn,execFileSync}=require('node:child_process');
const {mkdtempSync,mkdirSync,readFileSync}=require('node:fs');
const {join,resolve}=require('node:path');
const {randomBytes}=require('node:crypto');
const http=require('node:http');
const assert=require('node:assert/strict');
const wait=ms=>new Promise(r=>setTimeout(r,ms));
async function until(fn){for(let i=0;i<100;i++){const r=await fn();if(r)return r;await wait(200);}throw Error('Condition timed out');}
(async()=>{
 const root=resolve(__dirname,'..'),exe=join(root,'bin','heimdall.exe');mkdirSync(join(root,'.tools'),{recursive:true});const dir=mkdtempSync(join(root,'.tools','worker-test-')),data=join(dir,'data');
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8',windowsHide:true}));
 const extension=readFileSync(join(root,'extension','extension-id.txt'),'utf8').trim();cli('init');cli('browser','setup','--extension-id',extension,'--output',join(dir,'host'));
 let daemon,host,context;const replies=new Map();let buffer=Buffer.alloc(0);
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{windowsHide:true,stdio:['ignore','pipe','pipe']});await new Promise((r,j)=>{daemon.stdout.once('data',r);daemon.once('error',j);daemon.once('exit',c=>j(Error('daemon exit '+c)));});}
 async function stop(p){if(p&&p.exitCode===null)await new Promise(r=>{p.once('exit',r);p.kill();});}
 const secret=randomBytes(20).toString('hex');
 const server=http.createServer(async(req,res)=>{
  res.setHeader('Access-Control-Allow-Origin',`chrome-extension://${extension}`);res.setHeader('Access-Control-Allow-Headers','content-type');
  if(req.method==='OPTIONS'){res.end();return;}
  if(req.url==='/relay/'+secret&&req.method==='POST'){
   let raw='';for await(const chunk of req)raw+=chunk;if(raw.length>262144){res.writeHead(413).end();return;}
   const msg=JSON.parse(raw),body=Buffer.from(raw),header=Buffer.alloc(4);header.writeUInt32LE(body.length);
   const timer=setTimeout(()=>{replies.delete(msg.id);res.writeHead(504).end();},6000);
   replies.set(msg.id,reply=>{clearTimeout(timer);res.setHeader('Content-Type','application/json');res.end(JSON.stringify(reply));});host.stdin.write(Buffer.concat([header,body]));
  }else{res.setHeader('Content-Type','text/html');res.end('<title>Worker fixture</title><p>Local synthetic page</p>');}
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const url=`http://127.0.0.1:${server.address().port}/`;
 try{
  await start();host=spawn(join(dir,'host','heimdall-browser-host.exe'),[`chrome-extension://${extension}/`],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  host.stdout.on('data',chunk=>{buffer=Buffer.concat([buffer,chunk]);while(buffer.length>=4&&buffer.length>=buffer.readUInt32LE(0)+4){const n=buffer.readUInt32LE(0),reply=JSON.parse(buffer.subarray(4,n+4));buffer=buffer.subarray(n+4);replies.get(reply.id)?.(reply);replies.delete(reply.id);}});
  context=await chromium.launchPersistentContext(join(dir,'profile'),{channel:'chromium',headless:true,args:[`--disable-extensions-except=${join(root,'extension')}`,`--load-extension=${join(root,'extension')}`]});
  const worker=context.serviceWorkers()[0]??await context.waitForEvent('serviceworker');
  await worker.evaluate(relay=>{
   chrome.runtime.connectNative=()=>{
    const listeners=new Set(),disconnects=new Set();let closed=false;
    return {onMessage:{addListener:f=>listeners.add(f)},onDisconnect:{addListener:f=>disconnects.add(f)},disconnect(){if(!closed){closed=true;for(const f of disconnects)f();}},postMessage(message){fetch(relay,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(message)}).then(r=>r.json()).then(reply=>{if(!closed)for(const f of listeners)f(reply);}).catch(()=>{if(!closed){closed=true;for(const f of disconnects)f();}});}};
   };
  },url+'relay/'+secret);
  const page=await context.newPage();await page.goto(url);
  const profile=await until(()=>Object.values(cli('browser','status').profiles)[0]);assert.equal(profile.paired,false);assert.equal(profile.tabs.length,0);
  cli('browser','pair',profile.id);
  await until(()=>cli('browser','status').profiles[profile.id].tabs.some(t=>t.url===url&&t.title==='Worker fixture'));
  const operate=async(action,extra=[])=>{const p=cli('browser','status').profiles[profile.id];const op=cli('browser',action,'--profile',p.id,'--epoch',p.epoch,...extra);return until(()=>{const o=cli('browser','status').operations[op.id];return o.status!=='pending'&&o;});};
  const opened=await operate('open',['--url',url+'managed']);assert.equal(opened.status,'succeeded');
  await until(()=>cli('browser','status').profiles[profile.id].tabs.some(t=>t.id===opened.tab_id&&t.owner_id===opened.id));
  const focus=await operate('focus',['--tab',String(opened.tab_id),'--expected-url',url+'managed']);assert.equal(focus.status,'succeeded');
  const popup=await context.newPage();await popup.goto(`chrome-extension://${extension}/popup.html`);await popup.locator('#paused').check();await wait(4000);
  const before=cli('browser','status').profiles[profile.id].last_sequence;await page.goto(url+'paused');await wait(4000);assert.equal(cli('browser','status').profiles[profile.id].last_sequence,before);
  await popup.locator('#paused').uncheck();await until(()=>cli('browser','status').profiles[profile.id].tabs.some(t=>t.url===url+'paused'));
  await stop(daemon);await page.goto(url+'offline');await wait(6000);
  assert.ok(await popup.evaluate(async()=>{const {Outbox}=await import('./outbox.js');return (await new Outbox().all()).some(r=>r.body.tabs?.some(t=>t.url.endsWith('/offline')));}));
  await start();await page.goto(url+'reconnected');await until(()=>cli('browser','status').profiles[profile.id].tabs.some(t=>t.url===url+'reconnected'));
  const closed=await operate('close',['--tab',String(opened.tab_id),'--expected-url',url+'managed']);assert.equal(closed.status,'succeeded');
  console.log(JSON.stringify({status:'passed',checks:['worker handshake without auto-pairing','paired automatic inventory','CLI open/focus/close through worker and native framing','pause and resume','offline IndexedDB buffering','daemon restart and worker reconnect'],native_discovery:'test shim; OS registry registration not exercised'},null,2));
 }finally{await context?.close();await stop(host);await stop(daemon);await new Promise(r=>server.close(r));}
})().catch(e=>{console.error(e);process.exitCode=1;});
