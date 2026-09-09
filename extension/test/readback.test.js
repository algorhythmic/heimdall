import test from 'node:test';
import assert from 'node:assert/strict';
import {readback,retainedResults} from '../readback.js';
import {readFileSync} from 'node:fs';
const ref=JSON.parse(readFileSync(new URL('../../testdata/actions/wire-v1.json',import.meta.url))).reference;
function fixture(){
 const state={owners:{7:ref.id},journal:{[ref.id]:{action_ref:ref,result:{operation_id:ref.id,action_ref:ref,status:'succeeded',tab_id:7,window_id:3,url:'https://example.test/'}}}};
 const tabs=[{id:7,windowId:3,url:'https://example.test/',title:'Same',active:true,status:'complete'},{id:8,windowId:3,url:'chrome://settings',title:'Excluded UI',active:false},{id:9,windowId:9,url:'https://private.test/',incognito:true}];
 const api={tabs:{query:async()=>structuredClone(tabs)},windows:{getLastFocused:async()=>({id:3,focused:true})},storage:{session:{get:async()=>structuredClone(state)}}};return {state,tabs,api};
}
test('challenged census distinguishes filtered URLs from closed tabs and verifies exact ownership references',async()=>{
 const f=fixture(),r=await readback(f.api,{id:'a'.repeat(32)});assert.equal(r.type,'readback');assert.equal(r.stable,true);assert.equal(r.complete,true);assert.deepEqual(r.present_tabs,[7,8]);assert.equal(r.tabs.length,1);assert.equal(r.tabs[0].load_status,'complete');assert.deepEqual(r.instances,[{tab_id:7,action_ref:ref}]);assert(!JSON.stringify(r).includes('private.test'));
 const results=await retainedResults(f.api,[ref]);assert.equal(results.length,1);assert.deepEqual(await retainedResults(f.api,[{...ref,attempt_id:'f'.repeat(32)}]),[]);
});
test('concurrent focus, tab metadata and event-generation changes lose stable coverage',async()=>{
 const f=fixture();let count=0;f.api.tabs.query=async()=>{count++;return f.tabs.map(t=>({...t,active:count===1}));};assert.equal((await readback(f.api,{id:'a'.repeat(32)})).stable,false);
 let generation=0;const g=fixture();g.api.tabs.query=async()=>{generation++;return g.tabs;};assert.equal((await readback(g.api,{id:'a'.repeat(32)},()=>generation)).stable,false);
});
test('large censuses are partial and old journal rows confer no scoped ownership',async()=>{
 const f=fixture();delete f.state.journal[ref.id].action_ref;assert.deepEqual((await readback(f.api,{id:'a'.repeat(32)})).instances,[]);
 f.api.tabs.query=async()=>Array.from({length:2050},(_,i)=>({id:i+1,windowId:3,url:'chrome://settings'}));const r=await readback(f.api,{id:'a'.repeat(32)});assert.equal(r.complete,false);assert.equal(r.present_tabs.length,2048);
});
