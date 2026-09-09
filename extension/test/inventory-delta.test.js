import test from 'node:test';
import assert from 'node:assert/strict';
import {inventory,inventoryDelta} from '../controller.js';
test('container deltas retain unchanged tabs, report removals and omit private tabs',()=>{
 const tab={id:1,windowId:1,url:'https://example.test/',title:'first'};
 const before=inventory([tab,{...tab,id:2}],1);
 const after=inventory([{...tab,title:'changed'},{...tab,id:3},{...tab,id:4,incognito:true}],1);
 const d=inventoryDelta(before,after,7);
 assert.equal(d.base_seq,7);assert.equal(d.delta,true);assert.deepEqual(d.removed,[2]);assert.deepEqual(d.tabs.map(t=>t.id),[1,3]);
 const same=inventoryDelta(after,after,8);assert.deepEqual(same.tabs,[]);assert.deepEqual(same.removed,[]);
 const partial=inventoryDelta(before,{...after,complete:false},7);assert.deepEqual(partial.removed,[]);
});
