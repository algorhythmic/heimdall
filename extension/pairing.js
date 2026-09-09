import {validActionRef,requestFingerprint,actionResult} from './action-protocol.js';
const sameRef=(a,b)=>['version','id','attempt_id','intent_digest','target','manifest_id','surface_id'].every(k=>a?.[k]===b?.[k]);
const opaque=s=>typeof s==='string'&&/^[a-f0-9]{32}$/.test(s);
export const pairURL=(api,id)=>api.runtime.getURL('pair.html')+'#'+id;
const continuationKey=c=>JSON.stringify([c.version,c.id,c.action_ref,c.profile,c.epoch,c.marker_tab_id,c.window_id,c.original_tab_id??0,c.action,c.url??'',c.expires_at]);
const detail=e=>String(e?.message??e).slice(0,400);

// Retained journals may recover the exact nonce tab, but never repeat creation.
export async function retainedPairings(api,refs){
 const {journal={},owners={}}=await api.storage.session.get(['journal','owners']);
 const tabs=await api.tabs.query({});const out=[];let changed=false;
 for(const ref of refs){
  const row=journal[ref.id],op=row?.pairing?.operation;
  if(!validActionRef(ref)||!sameRef(ref,row?.action_ref)||!op)continue;
  if(!row.pairing.ready){
   const matches=tabs.filter(t=>!t.incognito&&t.url===pairURL(api,ref.id)&&!t.pendingUrl);
   if(matches.length!==1)continue;
   const t=matches[0];
   if(op.action==='associate'&&t.windowId!==op.window_id)continue;
   row.pairing.ready={action_ref:ref,marker_tab_id:t.id,window_id:t.windowId,...(op.action==='associate'?{original_tab_id:op.tab_id}:{})};
   if(op.action==='open')owners[t.id]=op.id;
   // A recovered ready record supersedes the provisional first-phase error.
   delete row.result;changed=true;
  }
  out.push(row.pairing.ready);
 }
 if(changed)await api.storage.session.set({journal,owners});
 return out;
}
export class PairActions {
 constructor(api,epoch){this.api=api;this.epoch=epoch;}
 async execute(op,paused=false){
  const wrap=r=>actionResult(op,{operation_id:op.id,...r});
  const refuse=s=>wrap({status:'refused',detail:s});
  if(!validActionRef(op.action_ref)||op.action_ref.id!==op.id||!['open','associate'].includes(op.action)||op.pairing?.version!==1||!opaque(op.pairing.source_id)||!/^[a-f0-9]{64}$/.test(op.pairing.source_epoch)||!(op.pairing.previous_viewport==='none'||opaque(op.pairing.previous_viewport)))return refuse('Explicit pairing authority required');
  const {journal={},owners={}}=await this.api.storage.session.get(['journal','owners']);
  const old=journal[op.id];
  if(old){
   if(old.request!==requestFingerprint(op))return wrap({status:'uncertain',detail:'Pairing request identity changed; input was not repeated'});
   const ready=(await retainedPairings(this.api,[op.action_ref]))[0];
   return ready?{pair_ready:ready}:wrap(old.result??{status:'uncertain',detail:'Pairing creation was interrupted; input was not repeated'});
  }
  if(paused||op.epoch!==this.epoch||!Number.isFinite(Date.parse(op.expires_at))||Date.now()>=Date.parse(op.expires_at))return refuse('Paused, stale epoch, or expired');
  if(op.action==='open'){
   let u;try{u=new URL(op.url);}catch{return refuse('Invalid URL');}
   if(!['http:','https:'].includes(u.protocol)||u.username||u.password)return refuse('URL is not allowed');
  }else{
   let t;try{t=await this.api.tabs.get(op.tab_id);}catch{return refuse('Original tab missing');}
   if(t.incognito||owners[t.id]!==op.owner_id||!op.owner_id||t.url!==op.expected_url||t.pendingUrl||t.windowId!==op.window_id)return refuse('Selected owned tab/window changed');
  }
  journal[op.id]={started:Date.now(),request:requestFingerprint(op),action_ref:op.action_ref,pairing:{operation:op}};
  await this.api.storage.session.set({journal});
  try{
   let t;
   if(op.action==='open'){
    const w=await this.api.windows.create({url:pairURL(this.api,op.id),focused:true,incognito:false});t=w.tabs?.[0];
   }else t=await this.api.tabs.create({url:pairURL(this.api,op.id),windowId:op.window_id,active:true});
   if(!t?.id||!t.windowId)throw Error('Marker creation returned no exact instance');
   const ready={action_ref:op.action_ref,marker_tab_id:t.id,window_id:t.windowId,...(op.action==='associate'?{original_tab_id:op.tab_id}:{})};
   journal[op.id].pairing.ready=ready;
   if(op.action==='open')owners[t.id]=op.id;
   await this.api.storage.session.set({journal,owners});
   return {pair_ready:ready};
  }catch(e){
   const result=wrap({status:'uncertain',detail:detail(e)});journal[op.id].result=result;
   await this.api.storage.session.set({journal});return result;
  }
 }
 async continue(c,paused=false){
  const wrap=r=>({operation_id:c.action_ref?.id,action_ref:c.action_ref,continuation_id:c.id,...r});
  const refuse=s=>wrap({status:'refused',detail:s});
  const {journal={},owners={}}=await this.api.storage.session.get(['journal','owners']);
  const row=journal[c.action_ref?.id],p=row?.pairing,op=p?.operation,r=p?.ready;
  if(c.version!==1||!opaque(c.id)||!validActionRef(c.action_ref)||!sameRef(row?.action_ref,c.action_ref)||!r||c.profile!==op.profile||c.epoch!==op.epoch||c.action!==op.action||c.marker_tab_id!==r.marker_tab_id||c.window_id!==r.window_id||(c.original_tab_id??0)!==(r.original_tab_id??0)||(c.url??'')!==(op.url??'')||c.expires_at!==op.expires_at)return refuse('Continuation differs from retained pairing authority');
  if(p.continuation){
   if(p.continuation.request!==continuationKey(c))return wrap({status:'uncertain',detail:'Continuation identity changed; input was not repeated'});
   return p.continuation.result??wrap({status:'uncertain',detail:'Continuation interrupted; input was not repeated'});
  }
  if(paused||c.epoch!==this.epoch||!Number.isFinite(Date.parse(c.expires_at))||Date.now()>=Date.parse(c.expires_at))return refuse('Paused, stale epoch, or expired');
  let marker;try{marker=await this.api.tabs.get(c.marker_tab_id);}catch{return refuse('Marker no longer exists');}
  if(marker.incognito||marker.url!==pairURL(this.api,op.id)||marker.pendingUrl||marker.windowId!==c.window_id||!marker.active)return refuse('Marker URL, activation or window changed');
  if(op.action==='associate'){
   let t;try{t=await this.api.tabs.get(op.tab_id);}catch{return refuse('Original tab missing');}
   if(t.incognito||owners[t.id]!==op.owner_id||t.url!==op.expected_url||t.pendingUrl||t.windowId!==op.window_id)return refuse('Original owned tab changed');
  }else if(owners[marker.id]!==op.id)return refuse('Marker ownership changed');
  p.continuation={id:c.id,request:continuationKey(c),started:Date.now()};await this.api.storage.session.set({journal});
  let result;
  try{
   if(op.action==='open'){
    await this.api.tabs.update(marker.id,{url:op.url});result=wrap({status:'succeeded',tab_id:marker.id,window_id:marker.windowId,url:op.url});
   }else{await this.api.tabs.remove(marker.id);result=wrap({status:'succeeded'});}
  }catch(e){result=wrap({status:'uncertain',detail:detail(e)});}
  p.continuation.result=result;await this.api.storage.session.set({journal});return result;
 }
}
