import test from 'node:test';
import assert from 'node:assert/strict';
import {FocusTracker} from '../focus.js';
const snapshot=(seconds,id=1,patch={})=>({observed_at:new Date(100000+seconds*1000).toISOString(),complete:true,focused_window:id?1:-1,tabs:id?[{id,window_id:1,active:true,url:`https://example.test/${id}`}]:[],...patch});
test('focus spans close on sampled blur with a two-second minimum',()=>{
 const f=new FocusTracker();
 assert.deepEqual(f.observe(snapshot(0)),[]);
 assert.deepEqual(f.observe(snapshot(1)),[]);
 const spans=f.observe(snapshot(5,2));
 assert.equal(spans.length,1);assert.equal(spans[0].duration_s,5);assert.equal(spans[0].tab_id,1);
 assert.deepEqual(f.observe(snapshot(6,1)),[]);
 assert.equal(f.observe(snapshot(10,0))[0].duration_s,4);
 assert.deepEqual(f.observe(snapshot(20,0)),[]);
});
test('navigation and window focus change split intervals',()=>{
 const f=new FocusTracker();f.observe(snapshot(0));
 assert.equal(f.observe(snapshot(3,1,{tabs:[{id:1,window_id:1,active:true,url:'https://example.test/new'}]}))[0].pointer,'https://example.test/1');
 assert.equal(f.observe(snapshot(6,0))[0].pointer,'https://example.test/new');
});
test('coverage loss, pause/reconnect reset, backwards clocks and unknown content discard intervals',()=>{
 for(const gap of [snapshot(5,1,{complete:false}),snapshot(-1),snapshot(5,1,{tabs:[]}),snapshot(5,1,{tabs:[{id:1,window_id:1,active:true,url:'https://example.test/?x=%zz'}]}),snapshot(5,1,{tabs:[{id:1,window_id:1,active:true,url:'https://example.test/',navigation_pending:true}]})]){
  const f=new FocusTracker();f.observe(snapshot(0));assert.deepEqual(f.observe(gap),[]);assert.deepEqual(f.observe(snapshot(10,0)),[]);
 }
 const f=new FocusTracker();f.observe(snapshot(0));f.reset();assert.deepEqual(f.observe(snapshot(10,0)),[]);
});
