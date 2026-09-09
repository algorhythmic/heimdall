import test from 'node:test';
import assert from 'node:assert/strict';
import {Actions,inventory,validURL} from '../controller.js';
import {readFileSync} from 'node:fs';
const actionRef=JSON.parse(readFileSync(new URL('../../testdata/actions/wire-v1.json',import.meta.url))).reference;
const epoch='a'.repeat(32), opID='b'.repeat(32);
function harness(){
 const state={},calls=[];
 const api={storage:{session:{get:async()=>structuredClone(state),set:async v=>Object.assign(state,structuredClone(v))}},
 runtime:{getURL:p=>'chrome-extension://id/'+p},
 tabs:{get:async id=>({id,url:'https://example.test/',windowId:1}),update:async(...a)=>calls.push(['update',...a]),move:async(...a)=>calls.push(['move',...a]),remove:async(...a)=>calls.push(['remove',...a])},
 windows:{create:async(...a)=>{calls.push(['create',...a]);return {id:9,tabs:[{id:10}]};},get:async()=>({incognito:false}),update:async(...a)=>calls.push(['focus',...a])}};
 return {api,state,calls,actions:new Actions(api,epoch)};
}
const operation=patch=>({id:opID,epoch,action:'open',url:'https://example.test/',expires_at:new Date(Date.now()+30000).toISOString(),...patch});
test('URL filtering excludes privileged schemes and credentials',()=>{for(const url of ['file:///x','javascript:alert(1)','https://u:p@example.test','chrome://settings','bad'])assert.equal(validURL(url),false);assert.equal(validURL('https://example.test'),true);});
test('inventory excludes private and privileged pages and respects byte limit',()=>{const tabs=[{id:1,windowId:1,url:'https://example.test',title:'normal'},{id:2,windowId:1,url:'https://private.test',incognito:true},{id:3,windowId:1,url:'chrome://settings'}];assert.equal(inventory(tabs,1).tabs.length,1);const large=inventory(Array.from({length:2049},(_,i)=>({id:i+1,windowId:1,url:'https://example.test/'+('x'.repeat(2000)),title:'😀'.repeat(1000)})),1);assert.equal(large.complete,false);assert.ok(new TextEncoder().encode(JSON.stringify(large)).length<=240*1024);});
test('open journal makes successful retries idempotent',async()=>{const h=harness();const a=await h.actions.execute(operation());assert.equal(a.status,'succeeded');assert.equal(h.state.owners[10],opID);assert.deepEqual(await h.actions.execute(operation()),a);assert.equal(h.calls.filter(c=>c[0]==='create').length,1);});
test('interrupted side effect is uncertain and never repeated',async()=>{const h=harness();h.state.journal={[opID]:{started:Date.now()}};assert.equal((await h.actions.execute(operation())).status,'uncertain');assert.equal(h.calls.length,0);});
test('stale epoch, expired, paused, and unowned tabs refuse with no action',async()=>{for(const patch of [{epoch:'c'.repeat(32)},{expires_at:'2000-01-01T00:00:00Z'},{action:'close',tab_id:1,expected_url:'https://example.test/'}]){const h=harness();assert.equal((await h.actions.execute(operation(patch))).status,'refused');assert.equal(h.calls.length,0);}const h=harness();assert.equal((await h.actions.execute(operation(),true)).status,'refused');});
test('owned tab requires current URL and supports explicit close',async()=>{const h=harness();h.state.owners={1:'owner'};const op=operation({action:'close',tab_id:1,owner_id:'owner',expected_url:'https://changed.test/'});assert.equal((await h.actions.execute(op)).status,'refused');op.expected_url='https://example.test/';assert.equal((await h.actions.execute(op)).status,'succeeded');assert.equal(h.state.owners[1],undefined);assert.equal(h.calls[0][0],'remove');});
test('partial API failure is retained as uncertain',async()=>{const h=harness();h.api.tabs.update=async()=>{throw Error('window closed');};const result=await h.actions.execute(operation());assert.equal(result.status,'uncertain');assert.deepEqual(await h.actions.execute(operation()),result);assert.equal(h.calls.length,1);});
test('shared task and attempt identity survives exact result retry',async()=>{
 const h=harness(),op=operation({id:actionRef.id,action_ref:actionRef});const result=await h.actions.execute(op);
 assert.deepEqual(result.action_ref,actionRef);assert.deepEqual(await h.actions.execute(op),result);assert.equal(h.calls.filter(c=>c[0]==='create').length,1);
 assert.deepEqual(await h.actions.execute({...op,action_ref:Object.fromEntries(Object.entries(actionRef).reverse())}),result);
 const changed={...op,url:'https://other.test/'};assert.equal((await h.actions.execute(changed)).status,'uncertain');assert.equal(h.calls.filter(c=>c[0]==='create').length,1);
});
test('shared action host disappears after side effect before result persistence',async()=>{
 const h=harness(),op=operation({id:actionRef.id,action_ref:actionRef});const set=h.api.storage.session.set;let writes=0;
 h.api.storage.session.set=async value=>{if(++writes===3)throw Error('worker stopped before durable result');return set(value);};
 await assert.rejects(h.actions.execute(op),/worker stopped/);const calls=h.calls.length;
 const retry=await h.actions.execute(op);assert.equal(retry.status,'uncertain');assert.deepEqual(retry.action_ref,actionRef);assert.equal(h.calls.length,calls);
});
test('shared action cannot inject input before intent persistence or with wrong scope shape',async()=>{
 const h=harness(),op=operation({id:actionRef.id,action_ref:actionRef});h.api.storage.session.set=async()=>{throw Error('storage unavailable');};
 await assert.rejects(h.actions.execute(op),/storage unavailable/);assert.equal(h.calls.length,0);
 const bad=harness();assert.equal((await bad.actions.execute({...op,action_ref:{...actionRef,actor:'cli'}})).status,'refused');assert.equal(bad.calls.length,0);
});
test('a pending navigation invalidates the old committed URL before input',async()=>{
 const h=harness();h.state.owners={1:opID};h.api.tabs.get=async()=>({id:1,windowId:1,url:'https://example.test/',pendingUrl:'https://example.test/next'});
 const r=await h.actions.execute({id:'c'.repeat(32),epoch,action:'close',tab_id:1,owner_id:opID,expected_url:'https://example.test/',expires_at:new Date(Date.now()+30000).toISOString()});assert.equal(r.status,'refused');assert.equal(h.calls.length,0);
});
