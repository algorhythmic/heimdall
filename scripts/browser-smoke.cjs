// Isolated real Chromium API test. Does not register a native host in any user browser.
const {chromium}=require('playwright');
const {mkdtempSync,readFileSync,mkdirSync}=require('node:fs');
const {join,resolve}=require('node:path');
const http=require('node:http');
const assert=require('node:assert/strict');
(async()=>{
 const root=resolve(__dirname,'..');mkdirSync(join(root,'.tools'),{recursive:true});
 const profile=mkdtempSync(join(root,'.tools','browser-test-'));
 const server=http.createServer((req,res)=>{res.setHeader('Content-Type','text/html');res.end('<title>Heimdall browser fixture</title><p>Local test page</p>');});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const url=`http://127.0.0.1:${server.address().port}/`;
 let context;
 try{
  context=await chromium.launchPersistentContext(profile,{channel:'chromium',headless:true,args:[`--disable-extensions-except=${join(root,'extension')}`,`--load-extension=${join(root,'extension')}`]});
  const worker=context.serviceWorkers()[0]??await context.waitForEvent('serviceworker');
  const expected=readFileSync(join(root,'extension','extension-id.txt'),'utf8').trim();assert.equal(new URL(worker.url()).host,expected);
  const popup=await context.newPage();await popup.goto(`chrome-extension://${expected}/popup.html`);
  const result=await popup.evaluate(async url=>{
   const {Actions,id,inventory}=await import('./controller.js');const {Outbox}=await import('./outbox.js');
   const epoch=id();const actions=new Actions(chrome,epoch);const open={id:id(),profile:id(),epoch,action:'open',url,expires_at:new Date(Date.now()+30000).toISOString()};
   const created=await actions.execute(open);if(created.status!=='succeeded')throw Error(JSON.stringify(created));
   // Wait on the browser's actual navigation completion.
   for(let i=0;i<50;i++){if((await chrome.tabs.get(created.tab_id)).status==='complete')break;await new Promise(r=>setTimeout(r,100));}
   const retry=await actions.execute(open);
   const snap=inventory(await chrome.tabs.query({}),created.window_id);
   const focus=await actions.execute({...open,id:id(),action:'focus',tab_id:created.tab_id,owner_id:open.id,expected_url:url});
   const navigated=await actions.execute({...open,id:id(),action:'navigate',url:url+'next',tab_id:created.tab_id,owner_id:open.id,expected_url:url});
   for(let i=0;i<50;i++){const t=await chrome.tabs.get(created.tab_id);if(t.url===url+'next'&&t.status==='complete')break;await new Promise(r=>setTimeout(r,100));}
   const destination=await chrome.windows.create({url:chrome.runtime.getURL('launch.html')});
   const moved=await actions.execute({...open,id:id(),action:'move',tab_id:created.tab_id,window_id:destination.id,owner_id:open.id,expected_url:url+'next'});
   const movedTo=(await chrome.tabs.get(created.tab_id)).windowId;
   const stale=await actions.execute({...open,id:id(),action:'close',tab_id:created.tab_id,owner_id:open.id,expected_url:url+'wrong'});
   const q=new Outbox();await q.put({id:'smoke',created:Date.now(),epoch,body:{type:'poll'}});const persisted=(await new Outbox().all()).some(r=>r.id==='smoke');await q.remove('smoke');
   const closed=await actions.execute({...open,id:id(),action:'close',tab_id:created.tab_id,owner_id:open.id,expected_url:url+'next'});
   return {created,retry,focus,navigated,moved,movedCorrectly:movedTo===destination.id,stale,closed,persisted,observed:snap.tabs.some(t=>t.id===created.tab_id&&t.title==='Heimdall browser fixture')};
  },url);
  assert.deepEqual(result.retry,result.created);assert.equal(result.focus.status,'succeeded');assert.equal(result.stale.status,'refused');assert.equal(result.closed.status,'succeeded');assert.equal(result.persisted,true);assert.equal(result.observed,true);
  assert.equal(result.navigated.status,'succeeded');assert.equal(result.moved.status,'succeeded');assert.equal(result.movedCorrectly,true);
  await popup.locator('#profile').filter({hasText:/[a-f0-9]{32}/}).waitFor();
  console.log(JSON.stringify({status:'passed',checks:['real MV3 worker and stable extension ID','real browser open, inventory, navigate, focus, move, close','stale URL refusal','idempotent retry','IndexedDB persistence','popup render'],native_registration:'not exercised'},null,2));
 }finally{await context?.close();await new Promise(r=>server.close(r));}
})().catch(e=>{console.error(e);process.exitCode=1;});
