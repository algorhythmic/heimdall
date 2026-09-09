import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {Actions} from '../controller.js';
import {retainedPairings,pairURL} from '../pairing.js';
import {readback,retainedResults} from '../readback.js';
const ref=JSON.parse(readFileSync(new URL('../../testdata/actions/wire-v1.json',import.meta.url))).reference;
function fixture(){
 const state={},calls=[],tabs=[];
 const api={runtime:{getURL:p=>'chrome-extension://'+'a'.repeat(32)+'/'+p},storage:{session:{get:async()=>structuredClone(state),set:async v=>Object.assign(state,structuredClone(v))}},
 tabs:{query:async()=>structuredClone(tabs),get:async id=>{const t=tabs.find(t=>t.id===id);if(!t)throw Error('missing');return structuredClone(t);},create:async p=>{calls.push(['createTab',p]);const t={id:7,windowId:p.windowId,url:p.url,active:true};tabs.push(t);return t;},update:async(id,p)=>{calls.push(['update',id,p]);Object.assign(tabs.find(t=>t.id===id),p);},remove:async id=>{calls.push(['remove',id]);tabs.splice(tabs.findIndex(t=>t.id===id),1);}},
 windows:{create:async p=>{calls.push(['createWindow',p]);const t={id:7,windowId:3,url:p.url,active:true};tabs.push(t);return {id:3,tabs:[t]};},getLastFocused:async()=>({id:3,focused:true})}};
 const op={id:ref.id,action_ref:ref,profile:'c'.repeat(32),epoch:'d'.repeat(32),action:'open',url:'https://example.test/',expires_at:new Date(Date.now()+30000).toISOString(),pairing:{version:1,source_id:'e'.repeat(32),source_epoch:'f'.repeat(64),previous_viewport:'none'}};
 const actions=new Actions(api,op.epoch);
 const continuation=()=>({version:1,id:'e'.repeat(32),action_ref:ref,profile:op.profile,epoch:op.epoch,marker_tab_id:7,window_id:3,action:op.action,url:op.url,expires_at:op.expires_at,...(op.action==='associate'?{original_tab_id:op.tab_id}:{})});
 return {state,calls,tabs,api,op,actions,continuation};
}
test('nonce creation and continuation each journal before their one-time side effect',async()=>{
 const f=fixture(),ready=await f.actions.execute(f.op);assert.equal(ready.pair_ready.marker_tab_id,7);assert.equal(f.calls.length,1);assert.equal(f.tabs[0].url,pairURL(f.api,ref.id));
 assert.deepEqual(await f.actions.execute(f.op),ready);assert.equal(f.calls.length,1);
 const read=await readback(f.api,{id:'a'.repeat(32)},()=>9);assert.equal(read.markers.length,1);assert.equal(read.event_generation,9);assert.equal(read.tabs.length,0);
 const c=f.continuation(),result=await f.actions.pairing.continue(c);assert.equal(result.status,'succeeded');assert.equal(result.continuation_id,c.id);assert.equal(f.tabs[0].url,f.op.url);
 assert.deepEqual(await f.actions.pairing.continue(c),result);assert.equal(f.calls.length,2);assert.deepEqual(await retainedResults(f.api,[ref]),[result]);
 assert.equal((await f.actions.pairing.continue({...c,id:'f'.repeat(32)})).status,'uncertain');assert.equal(f.calls.length,2);
});
test('changed marker URL, window, activation, cancellation or authority prevents continuation',async()=>{
 for(const patch of [{url:'https://user-data.test/'},{windowId:8},{active:false},{pendingUrl:'https://next.test/'},{incognito:true}]){
  const f=fixture();await f.actions.execute(f.op);Object.assign(f.tabs[0],patch);assert.equal((await f.actions.pairing.continue(f.continuation())).status,'refused');assert.equal(f.calls.length,1);
 }
 const f=fixture();await f.actions.execute(f.op);assert.equal((await f.actions.pairing.continue(f.continuation(),true)).status,'refused');assert.equal((await f.actions.pairing.continue({...f.continuation(),url:'https://other.test/'})).status,'refused');assert.equal(f.calls.length,1);
});
test('failed journal persistence cannot repeat either phase and exact nonce recovery is read-only',async()=>{
 const f=fixture(),set=f.api.storage.session.set;let writes=0;f.api.storage.session.set=async v=>{if(++writes>=2)throw Error('worker stopped');return set(v);};
 await assert.rejects(f.actions.execute(f.op));assert.equal(f.calls.length,1);f.api.storage.session.set=set;
 const recovered=await retainedPairings(f.api,[ref]);assert.equal(recovered[0].marker_tab_id,7);assert.equal(f.calls.length,1);
 writes=0;f.api.storage.session.set=async v=>{if(++writes>=2)throw Error('worker stopped');return set(v);};
 await assert.rejects(f.actions.pairing.continue(f.continuation()));assert.equal(f.calls.length,2);f.api.storage.session.set=set;
 assert.equal((await f.actions.pairing.continue(f.continuation())).status,'uncertain');assert.equal(f.calls.length,2);
});
test('association uses the owned selected window and removes only its temporary tab',async()=>{
 const f=fixture();Object.assign(f.op,{action:'associate',url:undefined,tab_id:5,window_id:3,owner_id:'b'.repeat(32),expected_url:'https://original.test/'});f.state.owners={5:f.op.owner_id};f.tabs.push({id:5,windowId:3,url:f.op.expected_url});
 await f.actions.execute(f.op);const result=await f.actions.pairing.continue(f.continuation());assert.equal(result.status,'succeeded');assert.deepEqual(f.calls.map(c=>c[0]),['createTab','remove']);assert.deepEqual(f.tabs.map(t=>t.id),[5]);
});
test('ambiguous recovered nonce tabs cannot establish pairing',async()=>{
 const f=fixture();await f.actions.execute(f.op);delete f.state.journal[ref.id].pairing.ready;f.tabs.push({...f.tabs[0],id:8,windowId:4});assert.deepEqual(await retainedPairings(f.api,[ref]),[]);assert.equal(f.calls.length,1);
});
