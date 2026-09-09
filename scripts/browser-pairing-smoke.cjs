// Real native messaging and browser APIs; the optional headful run observes
// the selected local Hyprland compositor. The default uses synthetic IPC only.
const {chromium}=require(require.resolve('playwright',{paths:[require('node:path').resolve(__dirname,'./browser-test')]}));
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),{join}=require('node:path'),http=require('node:http'),os=require('node:os'),assert=require('node:assert/strict');
const {randomBytes}=require('node:crypto');const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const wait=ms=>new Promise(r=>setTimeout(r,ms)),id=()=>randomBytes(16).toString('hex');
async function until(fn,label){for(let i=0;i<180;i++){const r=await fn();if(r)return r;await wait(200);}throw Error('Timed out: '+label);}
(async()=>{
 if(process.platform!=='linux')return;
 const live=process.env.HEIMDALL_PAIRING_HEADFUL==='1';
 const {root,exe,dir}=smokePaths('browser-pairing'),data=join(dir,'data'),profileDir=join(dir,'chromium');
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
 const input=(name,value)=>{const p=join(dir,name+'.json');fs.writeFileSync(p,JSON.stringify(value));return p;};
 cli('init');const extension=fs.readFileSync(join(root,'extension','extension-id.txt'),'utf8').trim();cli('browser','setup','--extension-id',extension,'--output',join(dir,'host'));
 fs.mkdirSync(join(profileDir,'NativeMessagingHosts'),{recursive:true});fs.copyFileSync(join(dir,'host','dev.heimdall.browser.json'),join(profileDir,'NativeMessagingHosts','dev.heimdall.browser.json'));
 let daemon,context,fake,socketDir,markerTimer;
 const fixture={version:{version:'0.56.2',dirty:false},monitors:[{id:0,name:'SYNTHETIC-1',x:0,y:0,width:1920,height:1080,scale:1,transform:0,activeWorkspace:{id:1}}],workspaces:[{id:1,name:'planning',monitor:'SYNTHETIC-1',monitorID:0}],clients:[]};
 const server=http.createServer((req,res)=>{res.setHeader('Content-Type','text/html');res.end('<title>Heimdall pairing acceptance</title><h1>Browser window paired</h1><p>Local synthetic content. No user profile or page data.</p>');});await new Promise(r=>server.listen(0,'127.0.0.1',r));const url=`http://127.0.0.1:${server.address().port}/paired`;
 try{
  daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{daemon.stdout.once('data',res);daemon.once('error',rej);});
  if(live){assert(process.env.HYPRLAND_INSTANCE_SIGNATURE);socketDir=join(process.env.XDG_RUNTIME_DIR,'hypr',process.env.HYPRLAND_INSTANCE_SIGNATURE);}
  else{
   socketDir=fs.mkdtempSync(join(os.tmpdir(),'heimdall-browser-pair-'));input('compositor',fixture);
   fake=spawn('python3',[join(root,'scripts','hyprland-fixture.py'),socketDir,join(dir,'compositor.json'),join(dir,'ipc-audit.txt')],{stdio:['pipe','pipe','inherit']});await new Promise((res,rej)=>{fake.stdout.once('data',res);fake.once('error',rej);});
  }
  const probe=cli('viewport','probe','--socket-dir',socketDir);assert(probe.fresh);
  const source=cli('viewport','select','--file',input('source',{version:1,id:id(),op:'select',previous:'none',source:{socket_dir:socketDir,host:probe.snapshot.host,epoch:probe.snapshot.source_epoch}}));
  context=await chromium.launchPersistentContext(profileDir,{channel:'chromium',headless:!live,...(live?{viewport:null}:{}),args:[...(live?['--ozone-platform=wayland']:[]),`--disable-extensions-except=${join(root,'extension')}`,`--load-extension=${join(root,'extension')}`]});
  const worker=context.serviceWorkers()[0]??await context.waitForEvent('serviceworker');assert.equal(new URL(worker.url()).host,extension);
  const popup=await context.newPage();await popup.goto(`chrome-extension://${extension}/popup.html`);
  const p=await until(()=>Object.values(cli('browser','status').profiles)[0],'native hello');assert.equal(p.pairing_protocol,1);cli('browser','pair',p.id);
  cli('add','Browser pairing','--id','alpha','--status','active');const surface=id();cli('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Browser pairing',surfaces:[{id:surface,kind:'browser',label:'Browser',required:true,restore_policy:'manual'}]}));
  const queue=browser=>{const c=cli('action','context','alpha'),state=cli('state');return cli('action','queue','alpha','--file',input('action-'+id(),{version:2,id:id(),target:'alpha',expected_task_revision:c.task_revision,manifest_id:c.manifest_id,surface_id:surface,context_digest:c.context_digest,browser:{profile:p.id,epoch:p.epoch,...browser,pairing:{version:1,source_id:source.id,source_epoch:source.epoch,previous_viewport:state.viewport_heads[surface]??'none'}}}));};
  let actionID,expectedTitle;
  if(!live){markerTimer=setInterval(()=>{if(!actionID)return;const title=context.pages().some(p=>p.url().endsWith('/pair.html#'+actionID))?'Heimdall pairing '+actionID+' - Chromium':'Heimdall pairing acceptance - Chromium';if(expectedTitle===title)return;expectedTitle=title;fixture.clients=[{address:'0x100',stableId:'18000001',pid:123,class:'chromium',title,workspace:{id:1,name:'planning'},monitor:0,at:[10,20],size:[800,600],mapped:true}];input('compositor',fixture);fake.stdin.write('windowtitle>>100\n');},50);}
  const opened=queue({action:'open',url,load_condition:'complete'});actionID=opened.intent.id;
  const action=a=>cli('action','show','alpha','--id',a.intent.id);
  const verified=async a=>until(()=>{const r=action(a);if(r.verification==='matched')return r;return false;},'paired '+a.intent.browser.action+' '+a.intent.id);
  const open=await verified(opened);assert(open.pairing.association_id);assert(open.pairing.continuation_delivery_id);assert.equal(open.pairing.probe_attempts,1);
  let state=cli('state');assert.equal(state.viewport_bindings[state.viewport_heads[surface]].browser_association_id,open.pairing.association_id);
  assert.equal((await popup.evaluate(async url=>(await chrome.tabs.query({})).filter(t=>t.url===url).length,url)),1);
  // Reassociate an exact already owned tab. Only a temporary tab is created/removed.
  const associated=queue({action:'associate',tab_id:open.report.tab_id,window_id:open.report.window_id,owner_id:open.intent.id,expected_url:url});actionID=associated.intent.id;
  const paired=await verified(associated);assert.notEqual(paired.pairing.association_id,open.pairing.association_id);
  assert.equal(await popup.evaluate(async()=>{const tabs=await chrome.tabs.query({});return tabs.filter(t=>t.url.includes('/pair.html#')).length;}),0);
  const view=cli('viewport','list','alpha');assert.equal(view.surfaces[0].status,'observed');assert.equal(view.surfaces[0].window.title,'');assert.equal(view.surfaces[0].window.pid,0);
  state=cli('state');cli('replay');assert.deepEqual(cli('state'),state);cli('backup','--output',join(dir,'schema19.db'));if(!live)fs.writeFileSync(join(dir,'events.json'),JSON.stringify(cli('events'),null,2)+'\n');
  console.log(JSON.stringify({status:'passed',browser:context.browser().version(),compositor:live?'actual Hyprland '+probe.snapshot.compositor_version:'synthetic read-only IPC',checks:['real native messaging','nonce open before navigation','ordered browser/native double observation','atomic association and viewport binding','one-time continuation','explicit owned-tab association','temporary-tab-only cleanup','scoped title redaction','inert replay'],data:dir},null,2));
  if(live&&process.env.HEIMDALL_PAIRING_VISUAL==='1'){
   await popup.evaluate(async nonce=>chrome.windows.create({url:chrome.runtime.getURL('pair.html')+'#'+nonce,focused:true}),id());
   console.log('Visual marker ready; release file: '+join(dir,'visual-done'));
   for(let n=0;!fs.existsSync(join(dir,'visual-done'));n++){if(n>1500)throw Error('Visual review timed out');await wait(200);}
  }
 }finally{clearInterval(markerTimer);await context?.close();await stopProcess(daemon);await stopProcess(fake);await new Promise(r=>server.close(r));if(!live&&socketDir)fs.rmSync(socketDir,{recursive:true,force:true});}
})().catch(e=>{console.error(e);process.exitCode=1;});
