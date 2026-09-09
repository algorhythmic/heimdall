const {seed}=require('./tui-demo.cjs');
const {smokePaths,stopProcess}=require('./smoke-paths.cjs');
const {execFileSync}=require('node:child_process');
const fs=require('node:fs'),{join}=require('node:path'),assert=require('node:assert/strict');
(async()=>{
 const {exe,dir,root}=smokePaths('tui');let fixture;
 try{
  fixture=await seed(exe,dir);const {raw,cli,data}=fixture;
  const before=cli('events');
  const dashboard=raw('tui','--snapshot');fs.writeFileSync(join(dir,'dashboard.txt'),dashboard);
  for(const text of ['heimdall','needs you','workstreams','jobapp-sensemesh','video-series','completion','decision','link'])assert(dashboard.includes(text),text);
  const target=raw('ui','video-series','--snapshot');assert(target.includes('storyboard'));assert(!target.includes('jobapp-sensemesh'));
  const narrow=raw('tui','--snapshot','--width','60','--height','22');assert(narrow.includes('WORKSTREAMS'));assert(!narrow.includes('\x1b'));
  assert.deepEqual(cli('events'),before,'snapshot reads wrote events');
  const ep=JSON.parse(fs.readFileSync(join(data,'endpoint.json'),'utf8'));
  for(const path of ['/ui/','/ui/app.js','/ui/session','/ui-bootstrap']){const r=await fetch(ep.url+path,{headers:{Authorization:'Bearer '+ep.token}});assert.equal(r.status,404,'retired browser route '+path)}
  if(process.platform==='linux')execFileSync('python3',[join(root,'scripts/tui-pty.py'),exe,data,join(dir,'terminal.ansi')],{stdio:'inherit'});
  const state=cli('state');cli('replay');assert.deepEqual(cli('state'),state,'TUI mutations changed replay');
  console.log(JSON.stringify({status:'passed',checks:['compiled wide/compact snapshots','explicit task and step selection','read-only rendering','retired GUI routes','pure replay after TUI actions'],pty:process.platform==='linux',data:dir},null,2));
 }finally{if(fixture)await stopProcess(fixture.daemon)}
})().catch(e=>{console.error(e);process.exitCode=1});
