import {inventory} from './controller.js';
import {validActionRef} from './action-protocol.js';
import {pairURL} from './pairing.js';
const equivalent=(a,b)=>['version','id','attempt_id','intent_digest','target','manifest_id','surface_id'].every(k=>a?.[k]===b?.[k]);
export async function retainedResults(api,refs){
 const {journal={}}=await api.storage.session.get('journal');
 return refs.flatMap(ref=>{const row=journal[ref.id],result=row?.pairing?.continuation?.result??row?.result;return validActionRef(ref)&&equivalent(row?.action_ref,ref)&&result?[result]:[];});
}
// This is direct challenged readback, never an offline outbox observation.
export async function readback(api,challenge,generation=()=>0){
 const sample=async()=>{
  const tabs=await api.tabs.query({});let focused=-1;
  try{const w=await api.windows.getLastFocused();if(w.focused&&!w.incognito)focused=w.id;}catch{}
  const body=inventory(tabs,focused);
  body.tabs.sort((a,b)=>a.id-b.id);
  const publicTabs=tabs.filter(t=>!t.incognito&&t.id>0&&t.windowId>0);
  body.present_tabs=publicTabs.map(t=>t.id).sort((a,b)=>a-b).slice(0,2048);
  if(publicTabs.length>2048)body.complete=false;
  const present=new Set(body.present_tabs);body.tabs=body.tabs.filter(t=>present.has(t.id));
  const {owners={},journal={}}=await api.storage.session.get(['owners','journal']);
  body.instances=publicTabs.filter(t=>present.has(t.id)).flatMap(t=>{
   const owner=owners[t.id],row=journal[owner];
   return owner&&validActionRef(row?.action_ref)&&row.action_ref.id===owner?[{tab_id:t.id,action_ref:row.action_ref}]:[];
  }).sort((a,b)=>a.tab_id-b.tab_id);
  if(body.instances.length>128){body.instances=body.instances.slice(0,128);body.complete=false;}
  body.markers=publicTabs.flatMap(t=>Object.values(journal).flatMap(row=>{
   const r=row.pairing?.ready;
   return r&&r.marker_tab_id===t.id&&r.window_id===t.windowId&&t.url===pairURL(api,row.action_ref.id)&&!t.pendingUrl?[{action_ref:row.action_ref,tab_id:t.id,window_id:t.windowId,active:!!t.active}]:[];
  })).sort((a,b)=>a.tab_id-b.tab_id);
  if(body.markers.length>128){body.markers=body.markers.slice(0,128);body.complete=false;}
  return body;
 };
 const before=generation(),first=await sample(),second=await sample();
 const semantic=b=>JSON.stringify({...b,observed_at:undefined});
 second.stable=before===generation()&&semantic(first)===semantic(second);
 second.event_generation=generation();
 second.type='readback';second.challenge_id=challenge.id;
 // Native frame bounds apply to the complete census and ownership references too.
 while(new TextEncoder().encode(JSON.stringify(second)).length>240*1024&&second.tabs.length){second.tabs.pop();second.complete=false;}
 return second;
}
