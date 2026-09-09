// Actual Linux Chromium native-host discovery in a dedicated user-data directory.
// Browser APIs and the native port are unmodified; only local fixture pages are used.
const {chromium}=require(require.resolve('playwright',{paths:[require('node:path').resolve(__dirname,'./browser-test')]}));
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),{join}=require('node:path'),http=require('node:http'),assert=require('node:assert/strict');
const {randomBytes}=require('node:crypto');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const wait=ms=>new Promise(r=>setTimeout(r,ms)),id=()=>randomBytes(16).toString('hex');
async function until(fn,label){for(let i=0;i<200;i++){const r=await fn();if(r)return r;await wait(200);}throw Error('Timed out: '+label);}
(async()=>{
 if(process.platform!=='linux'){console.log(JSON.stringify({status:'unavailable',reason:'Actual native discovery harness currently targets Linux Chromium'}));return;}
 const {root,exe,dir}=smokePaths('browser-verification'),data=join(dir,'data'),profileDir=join(dir,'chromium');
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
 const input=(name,value)=>{const p=join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(value));return p;};
 cli('init');const extension=fs.readFileSync(join(root,'extension','extension-id.txt'),'utf8').trim();
 cli('browser','setup','--extension-id',extension,'--output',join(dir,'host'));
 fs.mkdirSync(join(profileDir,'NativeMessagingHosts'),{recursive:true});fs.copyFileSync(join(dir,'host','dev.heimdall.browser.json'),join(profileDir,'NativeMessagingHosts','dev.heimdall.browser.json'));
 let daemon,context,crashOnLoad=false,crashed;
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{let errors='';daemon.stderr.on('data',b=>errors+=b);daemon.stdout.once('data',res);daemon.once('exit',c=>rej(Error('daemon '+c+' '+errors)));daemon.once('error',rej);});}
 const server=http.createServer(async(req,res)=>{
  if(req.url==='/redirect'){res.writeHead(302,{Location:'/landed'}).end();return;}
  if(req.url==='/kill'&&crashOnLoad){crashOnLoad=false;crashed=stopProcess(daemon,'SIGKILL');await crashed;}
  if(req.url==='/before-unload'){res.setHeader('Content-Type','text/html');res.end('<title>Unsaved fixture</title><button onclick="window.onbeforeunload=e=>{e.preventDefault();e.returnValue=String(1)}">Edit synthetic data</button>');return;}
  res.setHeader('Content-Type','text/html');res.end('<title>Duplicate fixture title</title><p>Local browser recovery fixture</p>');
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const url=`http://127.0.0.1:${server.address().port}`;
 try{
  await start();
  context=await chromium.launchPersistentContext(profileDir,{channel:'chromium',headless:true,args:[`--disable-extensions-except=${join(root,'extension')}`,`--load-extension=${join(root,'extension')}`]});
  const worker=context.serviceWorkers()[0]??await context.waitForEvent('serviceworker');assert.equal(new URL(worker.url()).host,extension);
  const popup=await context.newPage();await popup.goto(`chrome-extension://${extension}/popup.html`);
  const p=await until(()=>Object.values(cli('browser','status').profiles)[0],'actual native hello');assert.equal(p.paired,false);assert.equal(p.verification_protocol,1);cli('browser','pair',p.id);
  const page=await context.newPage();await page.goto(url+'/same');
  await until(()=>cli('browser','status').profiles[p.id].tabs.some(t=>t.url===url+'/same'),'native inventory');
  cli('add','Browser recovery','--id','alpha','--status','active');const surface=id();
  cli('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Browser recovery',surfaces:[{id:surface,kind:'browser',label:'Browser',required:true,restore_policy:'manual'}]}));
  const queue=(browser)=>{const c=cli('action','context','alpha');return cli('action','queue','alpha','--file',input('action-'+id(),{version:1,id:id(),target:'alpha',expected_task_revision:c.task_revision,manifest_id:c.manifest_id,surface_id:surface,context_digest:c.context_digest,browser:{profile:p.id,epoch:p.epoch,...browser}}));};
  const action=a=>cli('action','show','alpha','--id',a.intent.id);
  const verified=(a,status='matched')=>until(()=>{const got=action(a);return got.verification===status&&got;},'postcondition '+a.intent.browser.action+' '+status);
  const opened=queue({action:'open',url:url+'/same',load_condition:'complete'}),open=await verified(opened);
  assert.equal(open.execution,'api_reported');const tab=open.report.tab_id;assert.ok(open.observation);assert.ok(cli('browser','status').profiles[p.id].freshness);
  assert.equal(await popup.evaluate(async url=>(await chrome.tabs.query({})).filter(t=>t.url===url).length,url+'/same'),2,'duplicate URL did not cause adoption');
  const next=(kind,extra={})=>queue({action:kind,tab_id:tab,owner_id:open.intent.id,expected_url:extra.expected_url??url+'/same',...extra});
  const redirected=next('navigate',{url:url+'/redirect',load_condition:'complete'});await verified(redirected,'not_matched');
  const navigate=next('navigate',{expected_url:url+'/landed',url:url+'/next',load_condition:'complete'});await verified(navigate);
  const focus=next('focus',{expected_url:url+'/next'});await verified(focus);
  const destination=await popup.evaluate(async()=>await chrome.windows.create({url:chrome.runtime.getURL('launch.html')}));
  const move=next('move',{expected_url:url+'/next',window_id:destination.id});await verified(move);
  const close=next('close',{expected_url:url+'/next'});await verified(close);
  // Kill the real daemon when a new owned tab requests its page, after dispatch
  // and before the worker can flush its queued command result.
  crashOnLoad=true;const interrupted=queue({action:'open',url:url+'/kill',load_condition:'complete'});
  await until(()=>!!crashed,'page-triggered daemon kill');await crashed;await wait(1000);await start();
  const recovered=await verified(interrupted);assert.ok(recovered.uncertain_since);assert.equal(recovered.execution,'api_reported');
  assert.equal(await popup.evaluate(async url=>(await chrome.tabs.query({})).filter(t=>t.url===url).length,url+'/kill'),1,'recovery created a duplicate tab');
  const unsaved=queue({action:'navigate',tab_id:recovered.report.tab_id,owner_id:recovered.intent.id,expected_url:url+'/kill',url:url+'/before-unload',load_condition:'complete'});await verified(unsaved);
  const unsavedPage=await until(()=>context.pages().find(p=>p.url()===url+'/before-unload'),'unsaved fixture page');let unloadDialogs=0;unsavedPage.on('dialog',async d=>{unloadDialogs++;await d.dismiss();});await unsavedPage.locator('button').click();
  const closeUnsaved=queue({action:'close',tab_id:recovered.report.tab_id,owner_id:recovered.intent.id,expected_url:url+'/before-unload'});
  const closedUnsaved=await until(()=>{const a=action(closeUnsaved);return (['matched','not_matched'].includes(a.verification)||(a.execution==='uncertain'&&a.verification==='unknown'))&&a;},'before-unload close readback');
  const stillPresent=!unsavedPage.isClosed();assert.equal(closedUnsaved.verification==='matched',!stillPresent);if(stillPresent)assert.notEqual(closedUnsaved.verification,'matched');
  const state=cli('state');cli('replay');assert.deepEqual(cli('state'),state);assert.equal(state.tasks.alpha.task.status,'active');
  cli('backup','--output',join(dir,'schema17.db'));fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
  console.log(JSON.stringify({status:'passed',browser:context.browser().version(),native_discovery:'actual Chromium NativeMessagingHosts in isolated user-data directory',before_unload:{dialogs:unloadDialogs,verification:closedUnsaved.verification,data_saved:'not asserted'},checks:['explicit pairing','fresh nonce and stable readback','duplicate URL ownership exclusion','exact redirect mismatch','navigation completed load','focus','move','exact closure','real daemon kill after browser side effect before result','retained result recovery without duplicate tab','inert replay and unchanged task completion'],data:dir},null,2));
 }finally{await context?.close();await stopProcess(daemon);await new Promise(r=>server.close(r));}
})().catch(e=>{console.error(e);process.exitCode=1;});
