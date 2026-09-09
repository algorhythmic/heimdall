// Installed Linux editor gate: real CLI/daemon and Neovim, isolated configuration.
// A synthetic Herdr protocol socket supplies a binding, then disappears. Actual
// Herdr rendering/restart acceptance remains in herdr-smoke.cjs and WCU evidence.
const {spawn,execFileSync,execFile} = require('node:child_process');
const fs = require('node:fs');
const {join} = require('node:path');
const net = require('node:net');
const assert = require('node:assert/strict');
const {promisify} = require('node:util');
const {smokePaths,stopProcess} = require('./smoke-paths.cjs');
const {root,exe,dir} = smokePaths('neovim');
const data=join(dir,'data'),work=join(dir,'work'),drafts=join(dir,'drafts');
const runtime=fs.mkdtempSync('/tmp/hnvim-');
const nvim=process.env.HEIMDALL_NVIM || 'nvim';
const cli=(...args)=>JSON.parse(execFileSync(exe,[...args,'--data-dir',data],{encoding:'utf8'}));
const input=(name,value)=>{const path=join(dir,name+'.json');fs.writeFileSync(path,JSON.stringify(value));return path;};
let daemon,server;
async function start() {
  daemon=spawn(exe,['start','--data-dir',data],{stdio:['ignore','pipe','pipe']});
  await new Promise((resolve,reject)=>{
    let out='',err='';const timer=setTimeout(()=>reject(Error('daemon readiness timeout: '+err)),10000);
    daemon.stdout.on('data',b=>{out+=b;if(out.includes('"status":"running"')){clearTimeout(timer);resolve();}});
    daemon.stderr.on('data',b=>err+=b);
    daemon.once('error',e=>{clearTimeout(timer);reject(e);});
    daemon.once('exit',code=>{clearTimeout(timer);reject(Error('daemon exited '+code+': '+err));});
  });
}
async function editor(fixture) {
  const path=input('editor-fixture',fixture);
  const result=await promisify(execFile)(nvim,['--headless','-u','NONE','-i','NONE','-n','-l',join(root,'scripts/neovim-smoke.lua')],{
    cwd:work,timeout:60000,maxBuffer:1024*1024,
    env:{...process.env,HEIMDALL_NVIM_FIXTURE:path,NVIM_LOG_FILE:join(runtime,'nvim.log'),XDG_RUNTIME_DIR:runtime,XDG_CONFIG_HOME:join(runtime,'config'),XDG_STATE_HOME:join(runtime,'state'),XDG_CACHE_HOME:join(runtime,'cache'),XDG_DATA_HOME:join(runtime,'data')},
  });
  assert.match(result.stdout+result.stderr,/HEIMDALL_NVIM_PASSED/);
}
(async()=>{
  try {
    assert.equal(process.platform,'linux','this installed editor gate currently targets Linux');
    fs.mkdirSync(work);fs.mkdirSync(drafts,{mode:0o700});
    execFileSync('git',['init',work],{stdio:'ignore'});
    const filename='notes | let g:heimdall_injected=1.md';
    fs.writeFileSync(join(work,filename),'Planning artifact\nvim: set modeline:\n');
    fs.writeFileSync(join(dir,'outside.md'),'Outside resource');
    cli('init');await start();
    const resources={};
    for(const target of ['alpha','beta']) {
      cli('add',target==='alpha'?'Alpha $(touch SHOULD_NOT_EXIST)\u202e':'Beta planning','--id',target,'--status','active','--next-action','Review '+target+' direction');
      const resource=cli('resource','bind',target,'--expected-task-revision','1','--file',input(target+'-resource',{kind:'tree',root:work,path:'.'}));resources[target]=resource.id;
      const contract=cli('contract','accept',target,'--expected-task-revision','1','--file',input(target+'-contract',{previous:'none',objective:'Continue '+target,resource_ids:[resource.id]}));
      if(target==='beta') {
        const artifact=cli('artifact','record',target,'--expected-task-revision','1','--file',input('beta-artifact',{artifact_id:'new',previous:'none',name:'Beta artifact',environment:'editor-test',resource_id:resource.id,path:filename,git:true}));
        cli('checkpoint','create',target,'--expected-task-revision','1','--file',input('beta-checkpoint',{previous:'none',contract_id:contract.id,summary:'Pinned beta notes',next_action:'Review beta notes',artifacts:[{artifact_id:artifact.artifact.id,version_id:artifact.record.id}]}));
      }
    }
    cli('add','Child task','--id','child-task','--parent','alpha','--status','active');
    const surface='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';
    const manifest=cli('workspace','accept','alpha','--expected-task-revision','1','--file',input('manifest',{previous:'none',name:'Editor fixture',surfaces:[{id:surface,kind:'terminal',label:'Synthetic shell',required:true,restore_policy:'manual'}]}));
    const socket=join(runtime,'herdr.sock');
    // Expose this owned process's actual cwd/PID so the adapter can independently
    // verify /proc and canonical Git identity. No user's Herdr pane is involved.
    process.chdir(work);
    const pane={pane_id:'w1:p1',terminal_id:'nvim-fixture-terminal',workspace_id:'w1',tab_id:'w1:t1',foreground_cwd:work};
    server=net.createServer(conn=>{
      let text='';conn.on('data',chunk=>{text+=chunk;if(!text.includes('\n'))return;const req=JSON.parse(text);
        let result;
        if(req.method==='ping')result={type:'pong',version:'0.8.2',protocol:20};
        else if(req.method==='pane.get')result={type:'pane_info',pane};
        else if(req.method==='pane.process_info')result={type:'pane_process_info',process_info:{pane_id:pane.pane_id,shell_pid:process.pid}};
        else throw Error('Unexpected fixture RPC '+req.method);
        conn.end(JSON.stringify({id:req.id,result})+'\n');
      });
    });
    await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(socket,resolve);});fs.chmodSync(socket,0o600);
    await promisify(execFile)(exe,['session','bind-herdr','alpha','--surface',surface,'--manifest',manifest.id,'--previous','none','--expected-task-revision','1','--socket',socket,'--pane',pane.pane_id,'--data-dir',data]);
    await new Promise(resolve=>server.close(resolve));server=null;
    assert.equal(cli('session','refresh','alpha','--surface',surface).status,'disconnected');
    const fixture={root,exe,data,work,drafts,filename,resource:resources.alpha,surface,daemon_pid:daemon.pid,mode:'main',result:join(dir,'editor-result.json')};
    await editor(fixture);
    const saved=JSON.parse(fs.readFileSync(fixture.result,'utf8'));
    assert(!fs.existsSync(join(work,'SHOULD_NOT_EXIST')));
    await stopProcess(daemon);await start();
    await editor({...fixture,mode:'retry',...saved,daemon_pid:daemon.pid});
    if(process.env.HEIMDALL_LAZY_PATH)await editor({...fixture,mode:'lazy',lazy_path:process.env.HEIMDALL_LAZY_PATH,daemon_pid:daemon.pid});
    assert.equal(cli('checkpoint','list','alpha').length,1);
    assert.equal(cli('state','alpha').task.status,'active');
    const state=cli('state');cli('replay');assert.deepEqual(cli('state'),state);
    console.log(JSON.stringify({status:'passed',neovim:execFileSync(nvim,['--version'],{encoding:'utf8'}).split('\n')[0],checks:['explicit task picker and same-repository switching','late response discarded after task switch','read-only resume and missing daemon display','editable private drafts, conflicts and original-target checks','retained draft and exact retry after daemon/editor restart','literal artifact path, traversal/symlink refusal and disabled modelines','disconnected Herdr status from stopped synthetic protocol socket','root-scoped GUI handoff with no credential in buffers/history','bounded subprocess output, invalid JSON and timeout','pure replay and task completion unchanged'],data:dir},null,2));
  } finally {
    await stopProcess(daemon);
    if(server)await new Promise(resolve=>server.close(resolve));
    fs.rmSync(runtime,{recursive:true,force:true});
  }
})().catch(e=>{console.error(e);process.exitCode=1;});
