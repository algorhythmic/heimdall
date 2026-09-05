import {Outbox} from './outbox.js';
import {Actions,id,inventory} from './controller.js';
const queue=new Outbox(); let port,profile,epoch,connection,paired=false,authorized=false,busy=false,dirty=true,actions,failures=0,nextAttempt=0;
const pending=new Map();
async function status(patch){await chrome.storage.local.set({status:{profile,epoch,paired,...patch}});}
async function init(){
 const local=await chrome.storage.local.get(['profile']);profile=local.profile??id();await chrome.storage.local.set({profile});
 const session=await chrome.storage.session.get(['epoch','authorized']);epoch=session.epoch??id();authorized=!!session.authorized;await chrome.storage.session.set({epoch});actions=new Actions(chrome,epoch);
}
const ready=init();
function rpc(body){
 const message={v:1,id:id(),profile,epoch,connection,...body};
 return new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>{pending.delete(message.id);reject(Error('Native host response timed out'));},6000);
  pending.set(message.id,{resolve:reply=>{clearTimeout(timer);reply.type==='error'?reject(Error(reply.error)):resolve(reply);},reject:e=>{clearTimeout(timer);reject(e);}});
  try{port.postMessage(message);}catch(e){clearTimeout(timer);pending.delete(message.id);reject(e);}
 });
}
async function connect(){
 connection=id();port=chrome.runtime.connectNative('dev.heimdall.browser');
 const current=port;
 port.onMessage.addListener(reply=>{const p=pending.get(reply.id);if(p){pending.delete(reply.id);p.resolve(reply);}});
 port.onDisconnect.addListener(()=>{const reason=chrome.runtime.lastError?.message??'Native host disconnected';if(port===current)port=undefined;paired=false;for(const p of pending.values())p.reject(Error(reason));pending.clear();status({connected:false,detail:reason});});
 const reply=await rpc({type:'hello',label:'Browser profile',extension_version:chrome.runtime.getManifest().version});paired=reply.paired;
 const s=await chrome.storage.session.get(['sequence']);await chrome.storage.session.set({sequence:Math.max(s.sequence??0,reply.last_sequence??0)});
 dirty=true;
}
async function enqueue(body){const {outboxOrder=0}=await chrome.storage.session.get('outboxOrder');await chrome.storage.session.set({outboxOrder:outboxOrder+1});const dropped=await queue.put({id:id(),epoch,created:Date.now(),order:outboxOrder+1,body});if(dropped)await chrome.storage.local.set({gap:'Outbox reached retention limit; some observations were dropped'});}
async function collect(paused){
 if(!authorized||paused||!dirty)return;dirty=false;
 try{const tabs=await chrome.tabs.query({});let focus=-1;try{const w=await chrome.windows.getLastFocused();if(w.focused&&!w.incognito)focus=w.id;}catch{}
  const {sequence=0}=await chrome.storage.session.get(['sequence']);await chrome.storage.session.set({sequence:sequence+1});
  await enqueue({...inventory(tabs,focus),seq:sequence+1});
 }catch(e){dirty=true;throw e;}
}
async function cycle(){
 if(busy)return;busy=true;
 try{
  await ready;const {paused=false}=await chrome.storage.local.get(['paused']);if(paused)await queue.clear();await collect(paused);
  if(Date.now()<nextAttempt)return;
  if(!port)await connect();
  const poll=await rpc({type:'poll'});const wasPaired=paired;paired=poll.paired;authorized=paired;await chrome.storage.session.set({authorized});if(paired&&!wasPaired)dirty=true;
  failures=0;nextAttempt=0;
  await status({connected:true,paused,detail:paired?'Connected':'Pair this profile using the local CLI'});
  if(!paired){await queue.clear();return;}
  if(paused){await queue.clear();for(const op of poll.commands??[])await rpc({type:'command_result',result:await actions.execute(op,true)});return;}
  for(const row of (await queue.all()).sort((a,b)=>(a.order??a.created)-(b.order??b.created))){
    if(row.epoch!==epoch||Date.now()-row.created>86400000){await queue.remove(row.id);await chrome.storage.local.set({gap:'Older browser-session observations discarded'});continue;}
    try{await rpc(row.body);await queue.remove(row.id);}catch(e){if(/stale_sequence|already finalized/.test(e.message)){await queue.remove(row.id);}else throw e;}
  }
  for(const op of poll.commands??[]){const result=await actions.execute(op);await enqueue({type:'command_result',result});dirty=true;}
  await collect(paused);
 }catch(e){failures++;nextAttempt=Date.now()+Math.min(30000,1000*2**Math.min(failures,5))*(0.8+Math.random()*0.2);await status({connected:false,detail:String(e.message)});if(port){const old=port;port=undefined;old.disconnect();}}
 finally{busy=false;}
}
for(const event of [chrome.tabs.onCreated,chrome.tabs.onUpdated,chrome.tabs.onRemoved,chrome.tabs.onActivated,chrome.tabs.onAttached,chrome.tabs.onDetached,chrome.windows.onFocusChanged])event.addListener(()=>{dirty=true;});
chrome.tabs.onRemoved.addListener(async tabId=>{await ready;const {owners={}}=await chrome.storage.session.get('owners');delete owners[tabId];await chrome.storage.session.set({owners});});
chrome.storage.onChanged.addListener((changes,area)=>{if(area==='local'&&changes.paused){dirty=true;cycle();}});
chrome.alarms.onAlarm.addListener(()=>cycle());
chrome.alarms.create('reconnect',{periodInMinutes:0.5});
setInterval(cycle,2000);cycle();
