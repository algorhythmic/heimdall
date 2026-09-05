// Isolated Chromium acceptance of the compiled daemon's scoped task GUI.
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs'),{join,resolve}=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require(require.resolve('playwright',{paths:[resolve(__dirname,'../web')]}));
(async()=>{
 const root=resolve(__dirname,'..'),exe=join(root,'bin','heimdall.exe');fs.mkdirSync(join(root,'.tools'),{recursive:true});
 const dir=fs.mkdtempSync(join(root,'.tools','gui-test-')),data=join(dir,'data');let daemon,browser;
 const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8',windowsHide:true}));
 const input=(name,value)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(value));return path;};
 async function start(){daemon=spawn(exe,['start','--data-dir',data],{windowsHide:true,stdio:['ignore','pipe','pipe']});await new Promise((res,rej)=>{let errors='';daemon.stderr.on('data',b=>errors+=b);const timer=setTimeout(()=>rej(Error('startup timeout '+errors)),10000);daemon.stdout.once('data',()=>{clearTimeout(timer);res()});daemon.once('error',e=>{clearTimeout(timer);rej(e)});daemon.once('exit',c=>{clearTimeout(timer);rej(Error('daemon exit '+c+' '+errors))});});}
 async function stop(){if(daemon&&daemon.exitCode===null)await new Promise(res=>{daemon.once('exit',res);daemon.kill()});}
 try{
  cli('init');await start();cli('add','Ship the evidence review workflow','--id','workspace','--status','active');cli('add','Verify input changes and recovery','--id','checks','--parent','workspace','--status','active');cli('add','Private workstream fixture','--id','private','--status','active');
  const document=cli('ls');document.tasks.find(t=>t.id==='workspace').done={text:'The implementation is tested and ready for review',mode:'all',checks:[{id:'children',kind:'children_done'},{id:'artifact',kind:'artifact.exists'}]};cli('import-tasks',input('tasks',document));
  const task=cli('state','workspace'),revision=String(task.revision);fs.writeFileSync(join(dir,'artifact.txt'),'Synthetic reviewed artifact');const resource=cli('resource','bind','workspace','--expected-task-revision',revision,'--file',input('resource',{kind:'file',root:dir,path:'artifact.txt'}));
  const contract=cli('contract','accept','workspace','--expected-task-revision',revision,'--file',input('contract',{previous:'none',objective:'Make evidence and saved progress easy to inspect before accepting completed work.',constraints:['Keep each session scoped to its workstream.','A recorded pass must be revalidated before completion.'],resource_ids:[resource.id]}));
  cli('decision','accept','workspace','--expected-task-revision',revision,'--file',input('decision',{text:'Keep the daemon as the source of truth. The browser shows scoped state and explicit review controls.'}));
  cli('checkpoint','create','workspace','--expected-task-revision',revision,'--file',input('checkpoint',{previous:'none',contract_id:contract.id,summary:'Evidence evaluation, replay and stale-input checks are implemented. The local task interface is ready for review.',next_action:'Inspect the task context and accept the verified completion proposal.',blockers:[]}));
  cli('complete','checks');const malicious='<img src=x onerror="window.guiInjection=true">';cli('add',malicious,'--id','untrusted','--parent','workspace','--status','dropped');
  const evaluator=cli('evidence','configure','workspace','--expected-task-revision',revision,'--file',input('evaluator',{check_id:'artifact',contract_id:contract.id,previous:'none',spec:{kind:'artifact.exists',resource_id:resource.id}}));const attempt=cli('evidence','evaluate','workspace','--evaluator',evaluator.id,'--expected-task-revision',revision);
  for(let i=0;i<50;i++){if(cli('evidence','list','workspace').evidence.find(e=>e.id===attempt.id)?.status==='finished')break;await new Promise(r=>setTimeout(r,100))}assert.equal(cli('evidence','list','workspace').evidence.find(e=>e.id===attempt.id).outcome,'matched');cli('tick');
  const boot=cli('ui','workspace');assert(!boot.url.includes(boot.code));browser=await chromium.launch({headless:true});const context=await browser.newContext({viewport:{width:1440,height:1080}});const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(boot.url);await page.locator('#login:not([hidden])').waitFor();await page.getByLabel('One-time code').fill(boot.code);await page.getByRole('button',{name:'Open workspace'}).click();await page.getByRole('heading',{name:'Last saved progress'}).waitFor();await page.getByText('Recorded pass',{exact:true}).waitFor();
  assert.equal(await page.locator('#tasks').getByText('Private workstream fixture').count(),0);assert.equal(await page.locator('#tasks').getByText(malicious,{exact:true}).count(),1);assert.equal(await page.evaluate(()=>!!window.guiInjection),false);assert.equal(await page.locator('#tasks img').count(),0);
  assert.equal(await page.evaluate(async()=> (await fetch('/ui/task?target=private')).status),403);
  assert.equal(await page.evaluate(async()=> (await fetch('/ui/logout',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).status),403);
  const cookies=await context.cookies();assert(cookies.some(c=>c.name==='heimdall_ui'&&c.httpOnly&&c.sameSite==='Strict'));
  await page.getByLabel('Find a task').focus();await page.keyboard.type('Verify');assert.equal(await page.locator('#tasks button').count(),1);await page.getByLabel('Find a task').fill('');
  // Use an ordinary title for the documentation image after the injection check.
  cli('update','untrusted','--title','Deferred: automatic continuation');await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByText('Deferred: automatic continuation',{exact:true}).waitFor();
  await page.screenshot({path:join(dir,'desktop.png'),fullPage:true});
  await page.setViewportSize({width:390,height:844});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth));await page.screenshot({path:join(dir,'mobile.png'),fullPage:true});
  await page.setViewportSize({width:1440,height:1080});await page.getByRole('button',{name:'Accept completion'}).click();await page.getByText('No completion proposal is waiting for review.',{exact:true}).waitFor();assert.equal(cli('state','workspace').task.status,'done');assert.equal(cli('state','private').task.status,'active');
  await page.getByRole('button',{name:'Sign out'}).click();await page.locator('#login:not([hidden])').waitFor();assert.equal(await page.locator('#workspace').isVisible(),false);
  assert.equal(await page.evaluate(async code=>(await fetch('/ui/bootstrap',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({code})})).status,boot.code),401);assert.deepEqual(errors,[]);
  console.log(JSON.stringify({status:'passed',checks:['compiled UI bootstrap','scoped task/context','one-time code and HttpOnly session','missing-CSRF and cross-task denial','literal untrusted text','keyboard task search','desktop/mobile layout','explicit scoped completion review','logout and code replay denial'],screenshots:[join(dir,'desktop.png'),join(dir,'mobile.png')],data:dir},null,2));
 }finally{if(browser)await browser.close();await stop()}
})().catch(e=>{console.error(e);process.exitCode=1});
